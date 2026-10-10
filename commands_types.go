package main

import (
	"math"
	"strconv"
	"strings"
)

const (
	notIntegerErr = "ERR value is not an integer or out of range"
	notFloatErr   = "ERR value is not a valid float"
)

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

func cmdHSet(s *Store, args []string) string {
	if len(args)%2 != 0 {
		return wrongArgs("hset")
	}
	h, _, errR := getOrCreate(s, args[1], TypeHash, true, NewHash)
	if errR != "" {
		return errR
	}
	added := 0
	for i := 2; i < len(args); i += 2 {
		if h.Set(args[i], args[i+1]) {
			added++
		}
	}
	return integerReply(added)
}

func cmdHGet(s *Store, args []string) string {
	h, ok, errR := getOrCreate(s, args[1], TypeHash, false, NewHash)
	if errR != "" {
		return errR
	}
	if !ok {
		return nullBulkString()
	}
	if v, found := h.Get(args[2]); found {
		return bulkString(v)
	}
	return nullBulkString()
}

func cmdHDel(s *Store, args []string) string {
	h, ok, errR := getOrCreate(s, args[1], TypeHash, false, NewHash)
	if errR != "" {
		return errR
	}
	if !ok {
		return integerReply(0)
	}
	deleted := 0
	for _, f := range args[2:] {
		if h.Delete(f) {
			deleted++
		}
	}
	dropIfEmpty(s, args[1], h.Len())
	return integerReply(deleted)
}

func cmdHLen(s *Store, args []string) string {
	h, ok, errR := getOrCreate(s, args[1], TypeHash, false, NewHash)
	if errR != "" {
		return errR
	}
	if !ok {
		return integerReply(0)
	}
	return integerReply(h.Len())
}

func cmdHGetAll(s *Store, args []string) string {
	h, ok, errR := getOrCreate(s, args[1], TypeHash, false, NewHash)
	if errR != "" {
		return errR
	}
	if !ok {
		return arrayReply(nil)
	}
	return arrayReply(h.All())
}

func cmdSAdd(s *Store, args []string) string {
	set, _, errR := getOrCreate(s, args[1], TypeSet, true, NewSet)
	if errR != "" {
		return errR
	}
	added := 0
	for _, m := range args[2:] {
		if set.Add(m) {
			added++
		}
	}
	return integerReply(added)
}

func cmdSRem(s *Store, args []string) string {
	set, ok, errR := getOrCreate(s, args[1], TypeSet, false, NewSet)
	if errR != "" {
		return errR
	}
	if !ok {
		return integerReply(0)
	}
	removed := 0
	for _, m := range args[2:] {
		if set.Remove(m) {
			removed++
		}
	}
	dropIfEmpty(s, args[1], set.Card())
	return integerReply(removed)
}

func cmdSIsMember(s *Store, args []string) string {
	set, ok, errR := getOrCreate(s, args[1], TypeSet, false, NewSet)
	if errR != "" {
		return errR
	}
	if ok && set.IsMember(args[2]) {
		return integerReply(1)
	}
	return integerReply(0)
}

func cmdSCard(s *Store, args []string) string {
	set, ok, errR := getOrCreate(s, args[1], TypeSet, false, NewSet)
	if errR != "" {
		return errR
	}
	if !ok {
		return integerReply(0)
	}
	return integerReply(set.Card())
}

func cmdSMembers(s *Store, args []string) string {
	set, ok, errR := getOrCreate(s, args[1], TypeSet, false, NewSet)
	if errR != "" {
		return errR
	}
	if !ok {
		return arrayReply(nil)
	}
	return arrayReply(set.Members())
}

func parseScore(s string) (float64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) {
		return 0, false
	}
	return f, true
}

func formatScore(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func zReply(entries []zEntry, withScores bool) string {
	out := make([]string, 0, len(entries)*2)
	for _, e := range entries {
		out = append(out, e.member)
		if withScores {
			out = append(out, formatScore(e.score))
		}
	}
	return arrayReply(out)
}

func parseWithScores(args []string, base int) (withScores, ok bool) {
	if len(args) == base {
		return false, true
	}
	if len(args) == base+1 && strings.ToUpper(args[base]) == "WITHSCORES" {
		return true, true
	}
	return false, false
}

func cmdZAdd(s *Store, args []string) string {
	if len(args)%2 != 0 {
		return wrongArgs("zadd")
	}
	scores := make([]float64, 0, (len(args)-2)/2)
	for i := 2; i < len(args); i += 2 {
		f, ok := parseScore(args[i])
		if !ok {
			return errorReply(notFloatErr)
		}
		scores = append(scores, f)
	}
	z, _, errR := getOrCreate(s, args[1], TypeZSet, true, NewZSet)
	if errR != "" {
		return errR
	}
	added := 0
	for i, score := range scores {
		if z.Add(args[3+2*i], score) {
			added++
		}
	}
	return integerReply(added)
}

func cmdZScore(s *Store, args []string) string {
	z, ok, errR := getOrCreate(s, args[1], TypeZSet, false, NewZSet)
	if errR != "" {
		return errR
	}
	if !ok {
		return nullBulkString()
	}
	if score, found := z.Score(args[2]); found {
		return bulkString(formatScore(score))
	}
	return nullBulkString()
}

func cmdZRank(s *Store, args []string) string {
	z, ok, errR := getOrCreate(s, args[1], TypeZSet, false, NewZSet)
	if errR != "" {
		return errR
	}
	if !ok {
		return nullBulkString()
	}
	if r, found := z.Rank(args[2]); found {
		return integerReply(r)
	}
	return nullBulkString()
}

func cmdZRem(s *Store, args []string) string {
	z, ok, errR := getOrCreate(s, args[1], TypeZSet, false, NewZSet)
	if errR != "" {
		return errR
	}
	if !ok {
		return integerReply(0)
	}
	removed := 0
	for _, m := range args[2:] {
		if z.Remove(m) {
			removed++
		}
	}
	dropIfEmpty(s, args[1], z.Card())
	return integerReply(removed)
}

func cmdZCard(s *Store, args []string) string {
	z, ok, errR := getOrCreate(s, args[1], TypeZSet, false, NewZSet)
	if errR != "" {
		return errR
	}
	if !ok {
		return integerReply(0)
	}
	return integerReply(z.Card())
}

func cmdZRange(s *Store, args []string) string {
	withScores, ok := parseWithScores(args, 4)
	if !ok {
		return errorReply("ERR syntax error")
	}
	start, err1 := strconv.Atoi(args[2])
	stop, err2 := strconv.Atoi(args[3])
	if err1 != nil || err2 != nil {
		return errorReply(notIntegerErr)
	}
	z, found, errR := getOrCreate(s, args[1], TypeZSet, false, NewZSet)
	if errR != "" {
		return errR
	}
	if !found {
		return arrayReply(nil)
	}
	return zReply(z.Range(start, stop), withScores)
}

func cmdZRangeByScore(s *Store, args []string) string {
	withScores, ok := parseWithScores(args, 4)
	if !ok {
		return errorReply("ERR syntax error")
	}
	min, ok1 := parseScore(args[2])
	max, ok2 := parseScore(args[3])
	if !ok1 || !ok2 {
		return errorReply("ERR min or max is not a float")
	}
	z, found, errR := getOrCreate(s, args[1], TypeZSet, false, NewZSet)
	if errR != "" {
		return errR
	}
	if !found {
		return arrayReply(nil)
	}
	return zReply(z.RangeByScore(min, max), withScores)
}
