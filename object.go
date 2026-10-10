package main

import "strconv"

type ObjType int

const (
	TypeString ObjType = iota
	TypeList
	TypeHash
	TypeSet
	TypeZSet
)

func (t ObjType) String() string {
	switch t {
	case TypeString:
		return "string"
	case TypeList:
		return "list"
	case TypeHash:
		return "hash"
	case TypeSet:
		return "set"
	default:
		return "zset"
	}
}

type Object struct {
	typ   ObjType
	value any

	lru uint32

	size int
}

const (
	embstrMaxLen           = 44
	listMaxBlockBytes      = 8 * 1024
	hashMaxListpackEntries = 128
	hashMaxListpackValue   = 64
	setMaxIntsetEntries    = 512
	setMaxListpackEntries  = 128
	setMaxListpackValue    = 64
	zsetMaxListpackEntries = 128
	zsetMaxListpackValue   = 64
)

const wrongTypeErr = "WRONGTYPE Operation against a key holding the wrong kind of value"

func (o *Object) Encoding() string {
	switch v := o.value.(type) {
	case string:
		if _, ok := parseCanonicalInt(v); ok {
			return "int"
		}
		if len(v) <= embstrMaxLen {
			return "embstr"
		}
		return "raw"
	case *List:
		return v.Encoding()
	case *Hash:
		return v.Encoding()
	case *Set:
		return v.Encoding()
	case *ZSet:
		return v.Encoding()
	}
	return "unknown"
}

func parseCanonicalInt(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || strconv.FormatInt(n, 10) != s {
		return 0, false
	}
	return n, true
}

func roundAlloc(n int) int {
	classes := []int{8, 16, 32, 48, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 448, 512}
	for _, c := range classes {
		if n <= c {
			return c
		}
	}
	return (n + 63) / 64 * 64
}

func stringValueSize(v string) int {
	if n, ok := parseCanonicalInt(v); ok {
		if n >= 0 && n < 10000 {
			return 0
		}
		return 16
	}
	if len(v) <= embstrMaxLen {
		return roundAlloc(16 + 3 + len(v) + 1)
	}
	return 16 + roundAlloc(3+len(v)+1)
}

func estimateKeySize(key string, o *Object) int {
	n := 12 + 32 + roundAlloc(3+len(key)+1)
	switch v := o.value.(type) {
	case string:
		n += stringValueSize(v)
	case *List:
		n += 16 + v.Mem()
	case *Hash:
		n += 16 + v.Mem()
	case *Set:
		n += 16 + v.Mem()
	case *ZSet:
		n += 16 + v.Mem()
	}
	return n
}
