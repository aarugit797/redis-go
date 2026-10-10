package main

import (
	"bufio"
	"net"
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

func TestMaxmemoryNoevictionRefusesWrites(t *testing.T) {
	s := newTestStore(t)
	s.cfg.MaxMemory = 20000
	var reply string
	for i := 0; i < 1000 && !strings.HasPrefix(reply, "-OOM"); i++ {
		reply = run(s, "SET key"+strconv.Itoa(i)+" value")
	}
	if !strings.HasPrefix(reply, "-OOM") {
		t.Fatal("expected OOM error")
	}
	if run(s, "GET key0") != bulkString("value") {
		t.Fatal("reads must still work over maxmemory")
	}
}

func TestApproximateLRUKeepsHotKeys(t *testing.T) {
	s := newTestStore(t)
	s.cfg.MaxMemoryPolicy = "allkeys-lru"
	s.cfg.MaxMemorySample = 10
	for i := 0; i < 2000; i++ {
		run(s, "SET key"+strconv.Itoa(i)+" value")
	}

	for i := 0; i < 2000; i++ {
		obj, _ := s.data.Get("key" + strconv.Itoa(i))
		if i >= 200 {
			obj.lru = s.lruClock - 3600
		}
	}
	s.cfg.MaxMemory = s.usedMemory / 2
	run(s, "SET trigger x")

	if s.usedMemory > s.cfg.MaxMemory+200 {
		t.Fatalf("still over maxmemory: %d > %d", s.usedMemory, s.cfg.MaxMemory)
	}
	hotLeft := 0
	for i := 0; i < 200; i++ {
		if _, ok := s.data.Get("key" + strconv.Itoa(i)); ok {
			hotLeft++
		}
	}
	if hotLeft < 190 || s.evictedKeys == 0 {
		t.Fatalf("approximate LRU evicted hot keys: %d of 200 hot keys left", hotLeft)
	}
}

func TestVolatileWithoutTTLKeysCannotEvict(t *testing.T) {
	s := newTestStore(t)
	s.cfg.MaxMemoryPolicy = "volatile-lru"
	s.cfg.MaxMemory = 5000
	var reply string
	for i := 0; i < 500 && !strings.HasPrefix(reply, "-OOM"); i++ {
		reply = run(s, "SET key"+strconv.Itoa(i)+" value")
	}
	if !strings.HasPrefix(reply, "-OOM") {
		t.Fatal("volatile policy with no TTL keys should behave like noeviction")
	}
}

func TestLFUCounterIsLogarithmicAndDecays(t *testing.T) {
	s := newTestStore(t)
	s.cfg.MaxMemoryPolicy = "allkeys-lfu"
	run(s, "SET hot x", "SET cold x")
	for i := 0; i < 100000; i++ {
		run(s, "GET hot")
	}
	hot := run(s, "OBJECT FREQ hot")
	n, _ := strconv.Atoi(strings.Trim(hot, ":\r\n"))
	if n < 100 || n > 200 {
		t.Fatalf("100k hits gave counter %d, expected roughly 140", n)
	}

	obj, _ := s.data.Get("hot")
	ldt := obj.lru >> 8
	counter := obj.lru & 0xFF
	obj.lru = (ldt-30)<<8 | counter
	if got := s.lfuDecayed(obj); got != counter-30 {
		t.Fatalf("decayed counter %d, want %d", got, counter-30)
	}
}

func TestServerModes(t *testing.T) {
	for _, mode := range []string{"single", "locked"} {
		t.Run(mode, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.SaveRules = nil
			cfg.Dir = t.TempDir()
			cfg.Mode = mode
			srv := NewServer(cfg, NewStore(cfg))
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			go srv.Serve(ln)

			done := make(chan error, 20)
			for c := 0; c < 20; c++ {
				go func(c int) {
					conn, err := net.Dial("tcp", ln.Addr().String())
					if err != nil {
						done <- err
						return
					}
					defer conn.Close()
					var batch []byte
					for i := 0; i < 200; i++ {
						key := "c" + strconv.Itoa(c) + "k" + strconv.Itoa(i)
						batch = appendCommand(batch, []string{"SET", key, strconv.Itoa(i)})
						batch = appendCommand(batch, []string{"GET", key})
					}
					conn.Write(batch)
					r := bufio.NewReader(conn)
					for i := 0; i < 200; i++ {
						ok, _ := r.ReadString('\n')
						r.ReadString('\n')
						val, _ := r.ReadString('\n')
						if ok != "+OK\r\n" || strings.TrimSpace(val) != strconv.Itoa(i) {
							done <- &net.AddrError{Err: "bad reply " + ok + val}
							return
						}
					}
					done <- nil
				}(c)
			}
			for c := 0; c < 20; c++ {
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
