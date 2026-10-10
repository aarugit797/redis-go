package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

const maxBatch = 1024

type batchRequest struct {
	commands [][]string
	replies  []string
}

type Server struct {
	cfg   *Config
	store *Store

	mu sync.Mutex
}

func NewServer(cfg *Config, store *Store) *Server {
	srv := &Server{cfg: cfg, store: store}
	go srv.cronLoop()
	return srv
}

func (srv *Server) cronLoop() {
	ticker := time.NewTicker(cronInterval)
	defer ticker.Stop()
	for range ticker.C {
		srv.mu.Lock()
		srv.store.cron()
		srv.mu.Unlock()
	}
}

func (srv *Server) runBatch(req *batchRequest) {
	req.replies = req.replies[:0]
	for _, args := range req.commands {
		req.replies = append(req.replies, safeExecute(srv.store, args))
	}
	if srv.store.aof != nil {
		if err := srv.store.aof.Flush(); err != nil {
			logf("AOF write failed: %v", err)
		}
	}
}

func (srv *Server) execute(req *batchRequest) {
	srv.mu.Lock()
	srv.runBatch(req)
	srv.mu.Unlock()
}

func safeExecute(s *Store, args []string) (reply string) {
	defer func() {
		if r := recover(); r != nil {
			logf("panic executing %v: %v\n%s", args, r, debug.Stack())
			reply = errorReply(fmt.Sprintf("ERR internal error: %v", r))
		}
	}()
	return executeCommand(s, args)
}

func (srv *Server) ListenAndServe() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", srv.cfg.Port))
	if err != nil {
		return err
	}
	defer listener.Close()
	logf("listening on :%d", srv.cfg.Port)
	return srv.Serve(listener)
}

func (srv *Server) Serve(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			logf("accept error: %v", err)
			continue
		}
		go srv.handleConnection(conn)
	}
}

func (srv *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReaderSize(conn, 16*1024)
	writer := bufio.NewWriterSize(conn, 16*1024)
	req := &batchRequest{}

	for {
		req.commands = req.commands[:0]
		args, err := parseRESP(reader)
		if err != nil {
			srv.protocolError(writer, err)
			return
		}
		req.commands = append(req.commands, args)
		for reader.Buffered() > 0 && len(req.commands) < maxBatch {
			args, err := parseRESP(reader)
			if err != nil {
				srv.protocolError(writer, err)
				return
			}
			req.commands = append(req.commands, args)
		}

		srv.execute(req)

		for _, r := range req.replies {
			writer.WriteString(r)
		}
		if err := writer.Flush(); err != nil {
			return
		}
	}
}

func (srv *Server) protocolError(w *bufio.Writer, err error) {
	if err == io.EOF || err == io.ErrUnexpectedEOF || errors.Is(err, net.ErrClosed) ||
		strings.Contains(err.Error(), "connection reset") {
		return
	}
	w.WriteString(errorReply("ERR Protocol error: " + err.Error()))
	w.Flush()
}
