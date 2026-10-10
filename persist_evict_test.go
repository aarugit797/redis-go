package main

import (
	"os"
	"testing"
)

func fillAllTypes(s *Store) {
	run(s, "SET str hello", "SET num 12345", "SET temp v EX 1000",
		"RPUSH list a b c", "HSET h f1 v1 f2 v2", "SADD ints 1 2 3", "SADD tags go redis",
		"ZADD z 1.5 a 2.5 b")
}

func assertAllTypes(t *testing.T, s *Store) {
	t.Helper()
	checks := map[string]string{
		"GET str":           bulkString("hello"),
		"GET num":           bulkString("12345"),
		"LRANGE list 0 -1":  arrayReply([]string{"a", "b", "c"}),
		"HGET h f2":         bulkString("v2"),
		"SISMEMBER ints 2":  integerReply(1),
		"SISMEMBER tags go": integerReply(1),
		"ZSCORE z b":        bulkString("2.5"),
	}
	for cmd, want := range checks {
		if got := run(s, cmd); got != want {
			t.Fatalf("%s = %q, want %q", cmd, got, want)
		}
	}
	if ttl := run(s, "TTL temp"); ttl == integerReply(-1) || ttl == integerReply(-2) {
		t.Fatalf("TTL lost: %q", ttl)
	}
}

func TestRDBRoundTrip(t *testing.T) {
	s := newTestStore(t)
	fillAllTypes(s)
	if run(s, "SAVE") != simpleString("OK") {
		t.Fatal("SAVE failed")
	}
	s2 := NewStore(s.cfg)
	if err := s2.loadData(); err != nil {
		t.Fatal(err)
	}
	assertAllTypes(t, s2)

	data, _ := os.ReadFile(s.rdbPath())
	data[10] ^= 0xFF
	os.WriteFile(s.rdbPath(), data, 0o644)
	if err := NewStore(s.cfg).loadRDB(s.rdbPath()); err == nil {
		t.Fatal("corrupted snapshot loaded without error")
	}
}
