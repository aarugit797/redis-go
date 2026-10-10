package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	maxArgs    = 1024 * 1024
	maxBulkLen = 512 * 1024 * 1024
)

func parseRESP(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	if len(line) < 3 || line[0] != '*' {
		return nil, fmt.Errorf("expected RESP array, got %q", line)
	}
	if !strings.HasSuffix(line, "\r\n") {
		return nil, fmt.Errorf("invalid array header terminator")
	}

	count, err := strconv.Atoi(strings.TrimSuffix(line[1:], "\r\n"))
	if err != nil {
		return nil, fmt.Errorf("invalid array length: %w", err)
	}
	if count < 0 || count > maxArgs {
		return nil, fmt.Errorf("invalid array length %d", count)
	}

	args := make([]string, count)

	for i := 0; i < count; i++ {
		lengthLine, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		if len(lengthLine) < 3 || lengthLine[0] != '$' {
			return nil, fmt.Errorf("expected bulk string, got %q", lengthLine)
		}
		if !strings.HasSuffix(lengthLine, "\r\n") {
			return nil, fmt.Errorf("invalid bulk length header terminator")
		}

		length, err := strconv.Atoi(strings.TrimSuffix(lengthLine[1:], "\r\n"))
		if err != nil {
			return nil, fmt.Errorf("invalid bulk length: %w", err)
		}
		if length < 0 || length > maxBulkLen {
			return nil, fmt.Errorf("invalid bulk length %d", length)
		}

		buf := make([]byte, length+2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, err
		}

		if buf[length] != '\r' || buf[length+1] != '\n' {
			return nil, fmt.Errorf("invalid bulk string terminator")
		}

		args[i] = string(buf[:length])
	}

	return args, nil
}

func simpleString(s string) string {
	return "+" + s + "\r\n"
}

func errorReply(msg string) string {
	return "-" + msg + "\r\n"
}

func bulkString(s string) string {
	return "$" + strconv.Itoa(len(s)) + "\r\n" + s + "\r\n"
}

func nullBulkString() string {
	return "$-1\r\n"
}

func integerReply(n int) string {
	return ":" + strconv.Itoa(n) + "\r\n"
}
