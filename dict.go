package main

import (
	"fmt"
	"hash/maphash"
	"time"
)

const (
	initialSize   = 4
	emptyVisitCap = 10
	shrinkRatio   = 8
)

type entry struct {
	key   string
	value string
	next  *entry
}

type table struct {
	buckets []*entry
	mask    uint64
	used    int
}

func newTable(size uint64) *table {
	return &table{buckets: make([]*entry, size), mask: size - 1}
}

func (t *table) size() uint64 {
	return t.mask + 1
}

type Dict struct {
	ht        [2]*table
	rehashIdx int64
	seed      maphash.Seed
}

func NewDict() *Dict {
	return &Dict{
		ht:        [2]*table{newTable(initialSize), nil},
		rehashIdx: -1,
		seed:      maphash.MakeSeed(),
	}
}

func (d *Dict) hash(key string) uint64 {
	return maphash.String(d.seed, key)
}

func (d *Dict) isRehashing() bool {
	return d.rehashIdx != -1
}

func (d *Dict) Len() int {
	n := d.ht[0].used
	if d.ht[1] != nil {
		n += d.ht[1].used
	}
	return n
}

func (d *Dict) Get(key string) (string, bool) {
	if d.isRehashing() {
		d.rehash(1)
	}
	if e := d.find(key, d.hash(key)); e != nil {
		return e.value, true
	}
	return "", false
}

func (d *Dict) find(key string, h uint64) *entry {
	for i := 0; i <= 1; i++ {
		t := d.ht[i]
		if t == nil {
			break
		}
		idx := h & t.mask
		if i == 0 && d.isRehashing() && int64(idx) < d.rehashIdx {
			continue
		}
		for e := t.buckets[idx]; e != nil; e = e.next {
			if e.key == key {
				return e
			}
		}
	}
	return nil
}

func (d *Dict) Set(key, value string) {
	if d.isRehashing() {
		d.rehash(1)
	}
	h := d.hash(key)
	if e := d.find(key, h); e != nil {
		e.value = value
		return
	}

	d.expandIfNeeded()
	t := d.ht[0]
	if d.isRehashing() {
		t = d.ht[1]
	}
	idx := h & t.mask
	t.buckets[idx] = &entry{key: key, value: value, next: t.buckets[idx]}
	t.used++
}

func (d *Dict) Delete(key string) bool {
	if d.isRehashing() {
		d.rehash(1)
	}
	h := d.hash(key)

	for i := 0; i <= 1; i++ {
		t := d.ht[i]
		if t == nil {
			break
		}
		idx := h & t.mask
		if i == 0 && d.isRehashing() && int64(idx) < d.rehashIdx {
			continue
		}
		var prev *entry
		for e := t.buckets[idx]; e != nil; e = e.next {
			if e.key == key {
				if prev == nil {
					t.buckets[idx] = e.next
				} else {
					prev.next = e.next
				}
				t.used--
				d.shrinkIfNeeded()
				return true
			}
			prev = e
		}
	}
	return false
}

func (d *Dict) expandIfNeeded() {
	if d.isRehashing() {
		return
	}
	t := d.ht[0]
	if uint64(t.used) >= t.size() {
		d.startRehash(t.size() * 2)
	}
}

func (d *Dict) shrinkIfNeeded() {
	if d.isRehashing() {
		return
	}
	t := d.ht[0]
	if t.size() > initialSize && uint64(t.used)*shrinkRatio <= t.size() {
		d.startRehash(nextPowerOfTwo(uint64(t.used) + 1))
	}
}

func nextPowerOfTwo(n uint64) uint64 {
	size := uint64(initialSize)
	for size < n {
		size *= 2
	}
	return size
}

func (d *Dict) startRehash(newSize uint64) {
	d.ht[1] = newTable(newSize)
	d.rehashIdx = 0
}

func (d *Dict) rehash(n int) bool {
	if !d.isRehashing() {
		return false
	}
	emptyVisits := n * emptyVisitCap
	old, nw := d.ht[0], d.ht[1]

	for n > 0 && old.used != 0 {
		if uint64(d.rehashIdx) >= old.size() {
			panic("rehash cursor ran past the end with keys still in the old table")
		}
		for old.buckets[d.rehashIdx] == nil {
			d.rehashIdx++
			emptyVisits--
			if emptyVisits == 0 {
				return true
			}
		}
		e := old.buckets[d.rehashIdx]
		for e != nil {
			next := e.next
			idx := d.hash(e.key) & nw.mask
			e.next = nw.buckets[idx]
			nw.buckets[idx] = e
			old.used--
			nw.used++
			e = next
		}
		old.buckets[d.rehashIdx] = nil
		d.rehashIdx++
		n--
	}
	if old.used == 0 {
		d.ht[0] = nw
		d.ht[1] = nil
		d.rehashIdx = -1
		return false
	}
	return true
}
func (d *Dict) RehashFor(budget time.Duration) {
	start := time.Now()
	for d.rehash(100) {
		if time.Since(start) > budget {
			return
		}
	}
}
func (d *Dict) Stats() string {
	s := fmt.Sprintf("table0: size=%d used=%d", d.ht[0].size(), d.ht[0].used)
	if d.isRehashing() {
		s += fmt.Sprintf(" | table1: size=%d used=%d | rehashing, next old bucket=%d",
			d.ht[1].size(), d.ht[1].used, d.rehashIdx)
	} else {
		s += " | not rehashing"
	}
	return s
}
