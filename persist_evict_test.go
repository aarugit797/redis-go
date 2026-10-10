package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
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

func openAOFStore(t *testing.T, cfg *Config) *Store {
	t.Helper()
	s := NewStore(cfg)
	if err := s.loadData(); err != nil {
		t.Fatal(err)
	}
	aof, err := OpenAOF(s.aofPath(), cfg.AppendFsync)
	if err != nil {
		t.Fatal(err)
	}
	s.aof = aof
	return s
}

func TestAOFReplayRewriteAndTruncatedTail(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SaveRules = nil
	cfg.Dir = t.TempDir()
	cfg.AppendOnly = true

	s := openAOFStore(t, cfg)
	fillAllTypes(s)
	for i := 0; i < 1000; i++ {
		run(s, "SET counter "+strconv.Itoa(i))
	}
	run(s, "SET gone x EX 1000", "EXPIRE gone 0")
	s.aof.Flush()
	s.aof.Close()

	raw, _ := os.ReadFile(s.aofPath())
	if !strings.Contains(string(raw), "PEXPIREAT") || strings.Contains(string(raw), "EXPIRE\r\n$4\r\ntemp") {
		t.Fatal("expiry was not logged as PEXPIREAT")
	}

	s2 := openAOFStore(t, cfg)
	assertAllTypes(t, s2)
	if run(s2, "GET counter") != bulkString("999") || run(s2, "GET gone") != nullBulkString() {
		t.Fatal("replay gave the wrong final state")
	}

	before, _ := s2.aof.Status()
	if err := s2.bgrewriteaof(); err != nil {
		t.Fatal(err)
	}
	run(s2, "SET during rewrite")
	s2.aof.Flush()
	waitFor(t, func() bool { _, r := s2.aof.Status(); return r == 0 })
	after, _ := s2.aof.Status()
	if after >= before {
		t.Fatalf("rewrite did not shrink the AOF: %d -> %d", before, after)
	}
	s2.aof.Close()

	f, _ := os.OpenFile(s.aofPath(), os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("*3\r\n$3\r\nSET\r\n$4\r\nhalf")
	f.Close()

	s3 := openAOFStore(t, cfg)
	defer s3.aof.Close()
	assertAllTypes(t, s3)
	if run(s3, "GET during") != bulkString("rewrite") {
		t.Fatal("a write made during the rewrite was lost")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
