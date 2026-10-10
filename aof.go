package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

type AOF struct {
	mu       sync.Mutex
	path     string
	f        *os.File
	buf      []byte
	policy   string
	size     int64
	baseSize int64

	rewriting  bool
	rewriteBuf []byte

	stop chan struct{}
}

func OpenAOF(path, policy string) (*AOF, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	a := &AOF{path: path, f: f, policy: policy, size: st.Size(), baseSize: st.Size(), stop: make(chan struct{})}
	if policy == "everysec" {
		go a.fsyncLoop()
	}
	return a, nil
}

func (a *AOF) Append(args []string) {
	a.mu.Lock()
	a.buf = appendCommand(a.buf, args)
	if a.rewriting {
		a.rewriteBuf = appendCommand(a.rewriteBuf, args)
	}
	a.mu.Unlock()
}

func (a *AOF) Flush() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.buf) == 0 {
		return nil
	}
	n, err := a.f.Write(a.buf)
	a.size += int64(n)
	a.buf = a.buf[:0]
	if err != nil {
		return err
	}
	if a.policy == "always" {
		return a.f.Sync()
	}
	return nil
}

func (a *AOF) fsyncLoop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-t.C:
			a.mu.Lock()
			f := a.f
			a.mu.Unlock()
			if err := f.Sync(); err != nil && !errors.Is(err, os.ErrClosed) {
				logf("AOF fsync failed: %v", err)
			}
		}
	}
}

func (a *AOF) Status() (size int64, rewriting int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rewriting {
		rewriting = 1
	}
	return a.size + int64(len(a.buf)), rewriting
}

func (a *AOF) NeedsRewrite() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.rewriting && a.size >= minAOFRewrite && a.size >= 2*a.baseSize
}

func (a *AOF) StartRewrite(content []byte) error {
	a.mu.Lock()
	if a.rewriting {
		a.mu.Unlock()
		return errors.New("Background append only file rewriting already in progress")
	}
	a.rewriting = true
	a.rewriteBuf = nil
	a.mu.Unlock()

	go func() {
		err := a.finishRewrite(content)
		if err != nil {
			logf("AOF rewrite failed: %v", err)
			a.mu.Lock()
			a.rewriting = false
			a.rewriteBuf = nil
			a.mu.Unlock()
		}
	}()
	return nil
}

func (a *AOF) finishRewrite(content []byte) error {
	tmp := a.path + ".rewrite.tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if _, err := f.Write(content); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, err := f.Write(a.rewriteBuf); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	size := int64(len(content) + len(a.rewriteBuf))

	// Windows refuses to rename a file that still has an open handle on
	// either end, so the new log and the old one are both closed first and
	// the survivor is reopened for appending.
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	a.f.Close()
	if err := os.Rename(tmp, a.path); err != nil {
		os.Remove(tmp)
		a.f, _ = os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		return err
	}
	nf, err := os.OpenFile(a.path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	a.f = nf

	a.buf = a.buf[:0]
	a.size = size
	a.baseSize = a.size
	a.rewriting = false
	a.rewriteBuf = nil
	logf("AOF rewrite done: %d bytes", a.size)
	return nil
}

func (a *AOF) Close() error {
	close(a.stop)
	if err := a.Flush(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.f.Sync()
	return a.f.Close()
}

func (s *Store) rewriteAOFContent() []byte {
	var out []byte
	const chunk = 64
	emit := func(name, key string, items []string) {
		for i := 0; i < len(items); i += chunk {
			end := i + chunk
			if end > len(items) {
				end = len(items)
			}
			out = appendCommand(out, append([]string{name, key}, items[i:end]...))
		}
	}
	now := nowMillis()
	s.data.ForEach(func(key string, obj *Object) {
		at, hasTTL := s.expires.Get(key)
		if hasTTL && at <= now {
			return
		}
		switch v := obj.value.(type) {
		case string:
			out = appendCommand(out, []string{"SET", key, v})
		case *List:
			emit("RPUSH", key, v.Range(0, -1))
		case *Hash:
			emit("HSET", key, v.All())
		case *Set:
			emit("SADD", key, v.Members())
		case *ZSet:
			var items []string
			for _, e := range v.Range(0, -1) {
				items = append(items, formatScore(e.score), e.member)
			}
			emit("ZADD", key, items)
		}
		if hasTTL {
			out = appendCommand(out, []string{"PEXPIREAT", key, strconv.FormatInt(at, 10)})
		}
	})
	return out
}

func (s *Store) bgrewriteaof() error {
	if s.aof == nil {
		return errors.New("AOF is not enabled")
	}
	return s.aof.StartRewrite(s.rewriteAOFContent())
}

func (s *Store) loadAOF(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	br := bytes.NewReader(data)
	reader := bufio.NewReader(br)
	good := 0
	for {
		args, err := parseRESP(reader)
		if err == io.EOF {
			return nil
		}
		if err == io.ErrUnexpectedEOF {
			logf("AOF ends with a truncated command: dropping %d bytes", len(data)-good)
			return os.Truncate(path, int64(good))
		}
		if err != nil {
			return err
		}
		if reply := executeCommand(s, args); len(reply) > 0 && reply[0] == '-' {
			logf("AOF replay: %v -> %s", args, reply)
		}
		good = len(data) - br.Len() - reader.Buffered()
	}
}
