package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type handler func(s *Store, args []string) string

type command struct {
	fn    handler
	arity int
}

var commandTable map[string]command

func init() {
	commandTable = map[string]command{
		"PING":      {fn: cmdPing, arity: -1},
		"ECHO":      {fn: cmdEcho, arity: 2},
		"SET":       {fn: cmdSet, arity: -3},
		"GET":       {fn: cmdGet, arity: 2},
		"DEL":       {fn: cmdDel, arity: -2},
		"EXPIRE":    {fn: cmdExpire, arity: 3},
		"PEXPIREAT": {fn: cmdPExpireAt, arity: 3},
		"TTL":       {fn: cmdTTL, arity: 2},
		"PTTL":      {fn: cmdTTL, arity: 2},
		"PERSIST":   {fn: cmdPersist, arity: 2},
		"TYPE":      {fn: cmdType, arity: 2},
		"OBJECT":    {fn: cmdObject, arity: 3},
		"DBSIZE":    {fn: cmdDBSize, arity: 1},
		"FLUSHALL":  {fn: cmdFlushAll, arity: -1},
		"HTSTATS":   {fn: cmdHTStats, arity: 1},

		"LPUSH":  {fn: cmdPush, arity: -3},
		"RPUSH":  {fn: cmdPush, arity: -3},
		"LPOP":   {fn: cmdPop, arity: 2},
		"RPOP":   {fn: cmdPop, arity: 2},
		"LLEN":   {fn: cmdLLen, arity: 2},
		"LRANGE": {fn: cmdLRange, arity: 4},
		"LINDEX": {fn: cmdLIndex, arity: 3},

		"HSET":    {fn: cmdHSet, arity: -4},
		"HGET":    {fn: cmdHGet, arity: 3},
		"HDEL":    {fn: cmdHDel, arity: -3},
		"HLEN":    {fn: cmdHLen, arity: 2},
		"HGETALL": {fn: cmdHGetAll, arity: 2},
	}
}

func executeCommand(s *Store, args []string) string {
	if len(args) == 0 {
		return errorReply("ERR empty command")
	}
	name := strings.ToUpper(args[0])
	cmd, ok := commandTable[name]
	if !ok {
		return errorReply(fmt.Sprintf("ERR unknown command '%s'", args[0]))
	}
	if (cmd.arity > 0 && len(args) != cmd.arity) || (cmd.arity < 0 && len(args) < -cmd.arity) {
		return wrongArgs(name)
	}
	s.totalCommands++
	return cmd.fn(s, args)
}

func wrongArgs(command string) string {
	return errorReply(fmt.Sprintf("ERR wrong number of arguments for '%s' command", strings.ToLower(command)))
}

func cmdPing(s *Store, args []string) string {
	switch len(args) {
	case 1:
		return simpleString("PONG")
	case 2:
		return bulkString(args[1])
	}
	return wrongArgs("ping")
}

func cmdEcho(s *Store, args []string) string { return bulkString(args[1]) }

func cmdSet(s *Store, args []string) string {
	key, value := args[1], args[2]
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

	s.expireIfNeeded(key)
	s.addObject(key, &Object{typ: TypeString, value: value})
	switch {
	case expireAt > 0:
		s.expires.Set(key, expireAt)
	case !keepTTL:
		s.expires.Delete(key)
	}
	return simpleString("OK")
}

func cmdGet(s *Store, args []string) string {
	obj, found, wrong := s.lookupType(args[1], TypeString)
	switch {
	case wrong:
		return errorReply(wrongTypeErr)
	case !found:
		return nullBulkString()
	}
	return bulkString(obj.value.(string))
}

func cmdType(s *Store, args []string) string {
	if obj, found := s.lookup(args[1]); found {
		return simpleString(obj.typ.String())
	}
	return simpleString("none")
}

func cmdObject(s *Store, args []string) string {
	if strings.ToUpper(args[1]) != "ENCODING" {
		return errorReply("ERR OBJECT supports ENCODING")
	}
	obj, found := s.lookup(args[2])
	if !found {
		return nullBulkString()
	}
	return bulkString(obj.Encoding())
}

func cmdDel(s *Store, args []string) string {
	deleted := 0
	for _, key := range args[1:] {
		if !s.expireIfNeeded(key) && s.deleteKey(key) {
			deleted++
		}
	}
	return integerReply(deleted)
}

func setDeadline(s *Store, key string, at int64) string {
	if _, ok := s.lookup(key); !ok {
		return integerReply(0)
	}
	if at <= nowMillis() {
		s.deleteKey(key)
		return integerReply(1)
	}
	s.expires.Set(key, at)
	return integerReply(1)
}

func cmdExpire(s *Store, args []string) string {
	at, errMsg := parseExpireAt(args[2], 1000, "expire", true)
	if errMsg != "" {
		return errMsg
	}
	return setDeadline(s, args[1], at)
}

func cmdPExpireAt(s *Store, args []string) string {
	at, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return errorReply(notIntegerErr)
	}
	return setDeadline(s, args[1], at)
}

func cmdTTL(s *Store, args []string) string {
	if _, ok := s.lookup(args[1]); !ok {
		return integerReply(-2)
	}
	at, ok := s.expires.Get(args[1])
	if !ok {
		return integerReply(-1)
	}
	ms := at - nowMillis()
	if ms < 0 {
		ms = 0
	}
	if strings.ToUpper(args[0]) == "PTTL" {
		return integerReply(int(ms))
	}
	return integerReply(int((ms + 500) / 1000))
}

func cmdPersist(s *Store, args []string) string {
	if _, ok := s.lookup(args[1]); !ok {
		return integerReply(0)
	}
	if s.expires.Delete(args[1]) {
		return integerReply(1)
	}
	return integerReply(0)
}

func cmdDBSize(s *Store, args []string) string { return integerReply(s.data.Len()) }

func cmdFlushAll(s *Store, args []string) string {
	s.flushAll()
	return simpleString("OK")
}

func cmdHTStats(s *Store, args []string) string {
	return bulkString("data -> " + s.data.Stats() + " || expires -> " + s.expires.Stats())
}

func parseExpireAt(arg string, unitMs int64, cmd string, allowPast bool) (int64, string) {
	n, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return 0, errorReply(notIntegerErr)
	}
	now := time.Now().UnixMilli()
	if n <= 0 {
		if allowPast {
			return now, ""
		}
		return 0, errorReply(fmt.Sprintf("ERR invalid expire time in '%s' command", cmd))
	}
	if n > (math.MaxInt64-now)/unitMs {
		return 0, errorReply(fmt.Sprintf("ERR invalid expire time in '%s' command", cmd))
	}
	return now + n*unitMs, ""
}
