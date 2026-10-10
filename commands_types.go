package main

import (
	"strconv"
	"strings"
)

const notIntegerErr = "ERR value is not an integer or out of range"

func getOrCreate[T any](s *Store, key string, t ObjType, create bool, newValue func() T) (value T, exists bool, errReply string) {
	obj, found, wrong := s.lookupType(key, t)
	if wrong {
		return value, false, errorReply(wrongTypeErr)
	}
	if !found {
		if !create {
			return value, false, ""
		}
		value = newValue()
		s.addObject(key, &Object{typ: t, value: value})
		return value, true, ""
	}
	return obj.value.(T), true, ""
}

func dropIfEmpty(s *Store, key string, n int) {
	if n == 0 {
		s.deleteKey(key)
	}
}

func cmdPush(s *Store, args []string) string {
	l, _, errR := getOrCreate(s, args[1], TypeList, true, NewList)
	if errR != "" {
		return errR
	}
	left := strings.ToUpper(args[0]) == "LPUSH"
	for _, v := range args[2:] {
		if left {
			l.PushFront(v)
		} else {
			l.PushBack(v)
		}
	}
	return integerReply(l.Len())
}

func cmdPop(s *Store, args []string) string {
	l, ok, errR := getOrCreate(s, args[1], TypeList, false, NewList)
	if errR != "" {
		return errR
	}
	if !ok {
		return nullBulkString()
	}
	var v string
	if strings.ToUpper(args[0]) == "LPOP" {
		v, _ = l.PopFront()
	} else {
		v, _ = l.PopBack()
	}
	dropIfEmpty(s, args[1], l.Len())
	return bulkString(v)
}

func cmdLLen(s *Store, args []string) string {
	l, ok, errR := getOrCreate(s, args[1], TypeList, false, NewList)
	if errR != "" {
		return errR
	}
	if !ok {
		return integerReply(0)
	}
	return integerReply(l.Len())
}

func cmdLRange(s *Store, args []string) string {
	start, err1 := strconv.Atoi(args[2])
	stop, err2 := strconv.Atoi(args[3])
	if err1 != nil || err2 != nil {
		return errorReply(notIntegerErr)
	}
	l, ok, errR := getOrCreate(s, args[1], TypeList, false, NewList)
	if errR != "" {
		return errR
	}
	if !ok {
		return arrayReply(nil)
	}
	return arrayReply(l.Range(start, stop))
}

func cmdLIndex(s *Store, args []string) string {
	idx, err := strconv.Atoi(args[2])
	if err != nil {
		return errorReply(notIntegerErr)
	}
	l, ok, errR := getOrCreate(s, args[1], TypeList, false, NewList)
	if errR != "" {
		return errR
	}
	if !ok {
		return nullBulkString()
	}
	if v, found := l.Index(idx); found {
		return bulkString(v)
	}
	return nullBulkString()
}
