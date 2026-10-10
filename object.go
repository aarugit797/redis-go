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
