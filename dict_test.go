package main

import (
	"strconv"
	"testing"
)

func TestDictSurvivesRehashing(t *testing.T) {
	d := NewDict[string]()
	const n = 100000
	for i := 0; i < n; i++ {
		d.Set("key"+strconv.Itoa(i), strconv.Itoa(i))
		if v, ok := d.Get("key0"); !ok || v != "0" {
			t.Fatalf("lost key0 after inserting %d keys", i+1)
		}
	}
	if d.Len() != n {
		t.Fatalf("expected %d keys, got %d", n, d.Len())
	}
	for i := 0; i < n; i++ {
		k := "key" + strconv.Itoa(i)
		if v, ok := d.Get(k); !ok || v != strconv.Itoa(i) {
			t.Fatalf("missing or wrong value for %s", k)
		}
	}
}

func TestDictOverwriteAndDelete(t *testing.T) {
	d := NewDict[string]()
	d.Set("x", "1")
	d.Set("x", "2")
	if v, _ := d.Get("x"); v != "2" {
		t.Fatalf("expected overwrite to 2, got %q", v)
	}
	if d.Len() != 1 {
		t.Fatalf("overwrite should not add a key, len=%d", d.Len())
	}
	if !d.Delete("x") {
		t.Fatal("delete of existing key returned false")
	}
	if _, ok := d.Get("x"); ok {
		t.Fatal("key still present after delete")
	}
	if d.Delete("x") {
		t.Fatal("second delete should return false")
	}
}

func TestDictShrinksAfterDeletes(t *testing.T) {
	d := NewDict[string]()
	const n = 10000
	for i := 0; i < n; i++ {
		d.Set("key"+strconv.Itoa(i), "v")
	}
	for d.rehash(100) {
	}
	peak := d.ht[0].size()

	for i := 0; i < n; i++ {
		d.Delete("key" + strconv.Itoa(i))
	}
	for d.rehash(100) {
	}
	if d.Len() != 0 {
		t.Fatalf("expected empty dict, got %d keys", d.Len())
	}
	if d.ht[0].size() >= peak {
		t.Fatalf("table did not shrink: still %d buckets", d.ht[0].size())
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
		if !ok {
			t.Fatal("RandomEntry found nothing in a non-empty dict")
		}
		if got, _ := d.Get(k); got != v {
			t.Fatalf("RandomEntry returned %s=%d, but the dict holds %d", k, v, got)
		}
		seen[k] = true
	}
	if len(seen) < 500 {
		t.Fatalf("sampling looks stuck: only %d distinct keys in 5000 draws", len(seen))
	}

	empty := NewDict[int]()
	if _, _, ok := empty.RandomEntry(); ok {
		t.Fatal("RandomEntry on an empty dict should return ok=false")
	}
}