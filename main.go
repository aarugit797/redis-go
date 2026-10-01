package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
)

func main() {
	store := NewStore()

	listener, err := net.Listen("tcp", ":6380")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	fmt.Println("listening on :6380")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("accept error:", err)
			continue
		}
		go handleConnection(conn, store)
	}
}

func handleConnection(conn net.Conn, store *Store) {
	defer conn.Close()

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recovered from panic in client handler:", r)
		}
	}()

	reader := bufio.NewReader(conn)

	for {
		args, err := parseRESP(reader)
		if err != nil {
			if err != io.EOF {
				fmt.Println("parse error:", err)
			}
			return
		}

		reply := executeCommand(store, args)

		if _, err := conn.Write([]byte(reply)); err != nil {
			fmt.Println("write error:", err)
			return
		}
	}
}
