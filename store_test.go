package main

import (
	"strconv"
	"testing"
	"time"
)

func TestPassiveExpiry(t *testing.T) {
	s := NewStore()
	s.Set("k", "v", nowMillis()+50, false)
	if _, ok := s.Get("k"); !ok {
		t.Fatal("key should exist before its deadline")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := s.Get("k"); ok {
		t.Fatal("an expired key was returned")
	}
}

func TestTTLReplyCodes(t *testing.T) {
	s := NewStore()
	if got := s.TTLMillis("missing"); got != -2 {
		t.Fatalf("missing key: expected -2, got %d", got)
	}
	s.Set("x", "1", 0, false)
	if got := s.TTLMillis("x"); got != -1 {
		t.Fatalf("key without TTL: expected -1, got %d", got)
	}
	if !s.Expire("x", nowMillis()+5000) {
		t.Fatal("EXPIRE on an existing key should succeed")
	}
	if ttl := s.TTLMillis("x"); ttl <= 0 || ttl > 5000 {
		t.Fatalf("expected TTL in (0, 5000], got %d", ttl)
	}
	if !s.Persist("x") {
		t.Fatal("PERSIST should remove the TTL")
	}
	if got := s.TTLMillis("x"); got != -1 {
		t.Fatalf("after PERSIST: expected -1, got %d", got)
	}
}

func TestPlainSetClearsTTLButKeepTTLKeepsIt(t *testing.T) {
	s := NewStore()
	s.Set("a", "1", nowMillis()+10000, false)
	s.Set("a", "2", 0, false)
	if got := s.TTLMillis("a"); got != -1 {
		t.Fatalf("plain SET should wipe the TTL, got %d", got)
	}
	s.Set("b", "1", nowMillis()+10000, false)
	s.Set("b", "2", 0, true)
	if got := s.TTLMillis("b"); got <= 0 {
		t.Fatalf("SET with KEEPTTL should keep the TTL, got %d", got)
	}
}

func TestActiveExpiryCleansUnreadKeys(t *testing.T) {
	s := NewStore()
	for i := 0; i < 1000; i++ {
		s.Set("k"+strconv.Itoa(i), "v", nowMillis()+10, false)
	}
	time.Sleep(30 * time.Millisecond)

	s.mu.Lock()
	s.activeExpireCycle(time.Second)
	dataLeft, expLeft := s.data.Len(), s.expires.Len()
	s.mu.Unlock()

	if dataLeft != 0 || expLeft != 0 {
		t.Fatalf("active expiry left %d data keys and %d deadlines", dataLeft, expLeft)
	}
}