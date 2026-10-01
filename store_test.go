package main

import (
	"strconv"
	"sync"
	"testing"
)

// TestStoreConcurrentAccess hammers Store from many goroutines at once.
//
// Dict is not safe for concurrent use at all -- Get advances the rehash
// cursor -- so without Store's mutex this reliably loses keys or panics in
// rehash. It is a correctness check, not a race-detector check: this machine
// has CGO_ENABLED=0 and no gcc, so -race cannot run here.
func TestStoreConcurrentAccess(t *testing.T) {
	const writers = 8
	const perWriter = 2000

	s := NewStore()
	var wg sync.WaitGroup

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				k := "w" + strconv.Itoa(w) + ":" + strconv.Itoa(i)
				s.Set(k, k)
				if v, ok := s.Get(k); !ok || v != k {
					t.Errorf("key %s missing immediately after Set (got %q, ok=%v)", k, v, ok)
					return
				}
			}
		}(w)
	}

	// Concurrent readers and deleters of their own disjoint keyspace, so the
	// expected final count stays deterministic.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				k := "tmp" + strconv.Itoa(r) + ":" + strconv.Itoa(i)
				s.Set(k, "x")
				if !s.Delete(k) {
					t.Errorf("delete of own key %s returned false", k)
					return
				}
				s.Get("w0:0")
				_ = s.Stats()
			}
		}(r)
	}

	wg.Wait()
	if t.Failed() {
		return
	}

	if got := s.Len(); got != writers*perWriter {
		t.Fatalf("expected %d keys, got %d", writers*perWriter, got)
	}
	for w := 0; w < writers; w++ {
		for i := 0; i < perWriter; i++ {
			k := "w" + strconv.Itoa(w) + ":" + strconv.Itoa(i)
			if v, ok := s.Get(k); !ok || v != k {
				t.Fatalf("lost key %s after concurrent load (got %q, ok=%v)", k, v, ok)
			}
		}
	}
}
