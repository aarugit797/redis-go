package main

import (
	"fmt"
	"strings"
)

func executeCommand(store *Store, args []string) string {
	if len(args) == 0 {
		return errorReply("ERR empty command")
	}

	command := strings.ToUpper(args[0])

	switch command {
	case "PING":
		if len(args) == 1 {
			return simpleString("PONG")
		}
		if len(args) == 2 {
			return bulkString(args[1])
		}
		return errorReply("ERR wrong number of arguments for 'ping' command")

	case "SET":
		if len(args) != 3 {
			return errorReply("ERR wrong number of arguments for 'set' command")
		}
		store.Set(args[1], args[2])
		return simpleString("OK")

	case "GET":
		if len(args) != 2 {
			return errorReply("ERR wrong number of arguments for 'get' command")
		}
		value, exists := store.Get(args[1])
		if !exists {
			return nullBulkString()
		}
		return bulkString(value)

	default:
		return errorReply(fmt.Sprintf("ERR unknown command '%s'", args[0]))
	}
}