package main

import (
	"fmt"
	"math"
	"strconv"
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
		return wrongArgs(command)

	case "SET": 
		if len(args) < 3 {
			return wrongArgs(command)
		}
		var expireAt int64
		keepTTL := false
		for i := 3; i < len(args); i++ {
			opt := strings.ToUpper(args[i])
			switch {
			case (opt == "EX" || opt == "PX") && i+1 < len(args) && expireAt == 0 && !keepTTL:
				unit := int64(1)
				if opt == "EX" {
					unit = 1000
				}
				at, errMsg := parseExpireAt(args[i+1], unit, "set", false)
				if errMsg != "" {
					return errMsg
				}
				expireAt = at
				i++
			case opt == "KEEPTTL" && expireAt == 0 && !keepTTL:
				keepTTL = true
			default:
				return errorReply("ERR syntax error")
			}
		}
		store.Set(args[1], args[2], expireAt, keepTTL)
		return simpleString("OK")

	case "GET":
		if len(args) != 2 {
			return wrongArgs(command)
		}
		value, exists := store.Get(args[1])
		if !exists {
			return nullBulkString()
		}
		return bulkString(value)

	case "DEL":
		if len(args) < 2 {
			return wrongArgs(command)
		}
		deleted := 0
		for _, key := range args[1:] {
			if store.Delete(key) {
				deleted++
			}
		}
		return integerReply(deleted)

	case "EXPIRE": 
		if len(args) != 3 {
			return wrongArgs(command)
		}
		at, errMsg := parseExpireAt(args[2], 1000, "expire", true)
		if errMsg != "" {
			return errMsg
		}
		if store.Expire(args[1], at) {
			return integerReply(1)
		}
		return integerReply(0)

	case "TTL", "PTTL":
		if len(args) != 2 {
			return wrongArgs(command)
		}
		ms := store.TTLMillis(args[1])
		if ms < 0 || command == "PTTL" {
			return integerReply(int(ms)) 
		}
		return integerReply(int((ms + 500) / 1000)) 

	case "PERSIST":
		if len(args) != 2 {
			return wrongArgs(command)
		}
		if store.Persist(args[1]) {
			return integerReply(1)
		}
		return integerReply(0)

	case "HTSTATS": 
		if len(args) != 1 {
			return wrongArgs(command)
		}
		return bulkString(store.Stats())

	default:
		return errorReply(fmt.Sprintf("ERR unknown command '%s'", args[0]))
	}
}

func wrongArgs(command string) string {
	return errorReply(fmt.Sprintf("ERR wrong number of arguments for '%s' command", strings.ToLower(command)))
}

func parseExpireAt(arg string, unitMs int64, cmd string, allowPast bool) (int64, string) {
	n, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return 0, errorReply("ERR value is not an integer or out of range")
	}
	now := nowMillis()
	if n <= 0 {
		if allowPast {
			return now, "" 
		}
		return 0, errorReply(fmt.Sprintf("ERR invalid expire time in '%s' command", cmd))
	}
	if n > (math.MaxInt64-now)/unitMs { // would overflow
		return 0, errorReply(fmt.Sprintf("ERR invalid expire time in '%s' command", cmd))
	}
	return now + n*unitMs, ""
}