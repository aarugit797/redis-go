package main

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	return NewStore(DefaultConfig())
}

func run(s *Store, cmds ...string) string {
	var reply string
	for _, c := range cmds {
		reply = executeCommand(s, strings.Fields(c))
	}
	return reply
}

func TestDictSurvivesRehashing(t *testing.T) {
	d := NewDict[string]()
	const n = 100000
	for i := 0; i < n; i++ {
		d.Set("key"+strconv.Itoa(i), strconv.Itoa(i))
		if v, ok := d.Get("key0"); !ok || v != "0" {
			t.Fatalf("lost key0 after inserting %d keys", i+1)
		}
	}
	for i := 0; i < n; i++ {
		k := "key" + strconv.Itoa(i)
		if v, ok := d.Get(k); !ok || v != strconv.Itoa(i) {
			t.Fatalf("missing or wrong value for %s", k)
		}
	}
	if d.Len() != n {
		t.Fatalf("expected %d keys, got %d", n, d.Len())
	}
}

func TestDictShrinksAfterDeletes(t *testing.T) {
	d := NewDict[string]()
	for i := 0; i < 10000; i++ {
		d.Set("key"+strconv.Itoa(i), "v")
	}
	for d.rehash(100) {
	}
	peak := d.ht[0].size()
	for i := 0; i < 10000; i++ {
		d.Delete("key" + strconv.Itoa(i))
	}
	for d.rehash(100) {
	}
	if d.Len() != 0 || d.ht[0].size() >= peak {
		t.Fatalf("table did not shrink: %d keys, %d buckets", d.Len(), d.ht[0].size())
	}
}

func TestRandomEntryReturnsRealKeys(t *testing.T) {
	d := NewDict[int]()
	for i := 0; i < 1000; i++ {
		d.Set("k"+strconv.Itoa(i), i)
	}
	seen := map[string]bool{}
	for i := 0; i < 5000; i++ {
		k, v, ok := d.RandomEntry()
		if got, _ := d.Get(k); !ok || got != v {
			t.Fatalf("RandomEntry returned %s=%d, dict holds %d", k, v, got)
		}
		seen[k] = true
	}
	if len(seen) < 500 {
		t.Fatalf("sampling looks stuck: %d distinct keys", len(seen))
	}
}

func TestPassiveAndActiveExpiry(t *testing.T) {
	s := newTestStore(t)
	run(s, "SET k v PX 50")
	if run(s, "GET k") != bulkString("v") {
		t.Fatal("key should exist before its deadline")
	}
	for i := 0; i < 1000; i++ {
		run(s, "SET t"+strconv.Itoa(i)+" v PX 10")
	}
	time.Sleep(80 * time.Millisecond)
	if run(s, "GET k") != nullBulkString() {
		t.Fatal("an expired key was returned")
	}
	s.activeExpireCycle(time.Second)
	if s.data.Len() != 0 || s.expires.Len() != 0 {
		t.Fatalf("active expiry left %d keys", s.data.Len())
	}
}

func TestTTLCodesAndKeepTTL(t *testing.T) {
	s := newTestStore(t)
	if run(s, "TTL missing") != integerReply(-2) {
		t.Fatal("missing key should be -2")
	}
	if run(s, "SET x 1", "TTL x") != integerReply(-1) {
		t.Fatal("key without TTL should be -1")
	}
	run(s, "EXPIRE x 100")
	if run(s, "TTL x") != integerReply(100) {
		t.Fatalf("TTL = %q", run(s, "TTL x"))
	}
	if run(s, "SET x 2 KEEPTTL", "TTL x") != integerReply(100) {
		t.Fatal("KEEPTTL should keep the TTL")
	}
	if run(s, "SET x 3", "TTL x") != integerReply(-1) {
		t.Fatal("plain SET should wipe the TTL")
	}
}

func TestEncodingsChange(t *testing.T) {
	s := newTestStore(t)
	check := func(key, want string) {
		t.Helper()
		if got := run(s, "OBJECT ENCODING "+key); got != bulkString(want) {
			t.Fatalf("encoding of %s = %q, want %s", key, got, want)
		}
	}
	run(s, "SET n 42", "SET s hello", "SET r "+strings.Repeat("x", 45))
	check("n", "int")
	check("s", "embstr")
	check("r", "raw")

	run(s, "SADD ids 1 2 3")
	check("ids", "intset")
	run(s, "SADD ids apple")
	check("ids", "listpack")

	run(s, "HSET h name aaru")
	check("h", "listpack")
	run(s, "HSET h bio "+strings.Repeat("x", 80))
	check("h", "hashtable")
	run(s, "HDEL h bio")
	check("h", "hashtable")

	for i := 0; i < 3000; i++ {
		run(s, "RPUSH q item"+strconv.Itoa(i))
	}
	check("q", "quicklist")
	if run(s, "LPUSH n x") != errorReply(wrongTypeErr) {
		t.Fatal("expected WRONGTYPE")
	}
}

func TestListCommands(t *testing.T) {
	s := newTestStore(t)
	run(s, "RPUSH l a b c", "LPUSH l z")
	if got := run(s, "LRANGE l 0 -1"); got != arrayReply([]string{"z", "a", "b", "c"}) {
		t.Fatalf("LRANGE = %q", got)
	}
	if run(s, "LLEN l") != integerReply(4) {
		t.Fatal("LLEN wrong")
	}
	if run(s, "LINDEX l -1") != bulkString("c") {
		t.Fatal("LINDEX wrong")
	}
	run(s, "RPOP l", "RPOP l", "RPOP l", "RPOP l")
	if run(s, "TYPE l") != simpleString("none") {
		t.Fatal("empty list should delete its key")
	}
	if !reflect.DeepEqual(NewList().Range(0, -1), []string(nil)) {
		t.Fatal("empty range")
	}
}
