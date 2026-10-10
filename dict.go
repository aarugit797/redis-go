package main

import (
	"fmt"
	"hash/maphash"
	"math/rand"
	"time"
)

const (
	initialSize   = 4
	emptyVisitCap = 10
	shrinkRatio   = 8
)

type entry[V any] struct {
	key   string
	value V
	next  *entry[V]
}

type table[V any] struct {
	buckets []*entry[V]
	mask    uint64
	used    int
}

func newTable[V any](size uint64) *table[V] {
	return &table[V]{buckets: make([]*entry[V], size), mask: size - 1}
}

func (t *table[V]) size() uint64 { return t.mask + 1 }

type Dict[V any] struct {
	ht        [2]*table[V]
	rehashIdx int64
	seed      maphash.Seed
}

func NewDict[V any]() *Dict[V] {
	return &Dict[V]{
		ht:        [2]*table[V]{newTable[V](initialSize), nil},
		rehashIdx: -1,
		seed:      maphash.MakeSeed(),
	}
}

func (d *Dict[V]) hash(key string) uint64 { return maphash.String(d.seed, key) }

func (d *Dict[V]) isRehashing() bool { return d.rehashIdx != -1 }

func (d *Dict[V]) Len() int {
	n := d.ht[0].used
	if d.ht[1] != nil {
		n += d.ht[1].used
	}
	return n
}

func (d *Dict[V]) Get(key string) (V, bool) {
	if d.isRehashing() {
		d.rehash(1)
	}
	if e := d.find(key, d.hash(key)); e != nil {
		return e.value, true
	}
	var zero V
	return zero, false
}

func (d *Dict[V]) find(key string, h uint64) *entry[V] {
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

func (d *Dict[V]) Set(key string, value V) {
	if d.isRehashing() {
		d.rehash(1)
	}
	h := d.hash(key)
	if e := d.find(key, h); e != nil {
		e.value = value
		return
	}
	d.expandIfNeeded()

	// During a rehash new keys go into ht[1] only, so ht[0] can only shrink
	// and the rehash is guaranteed to terminate.
	t := d.ht[0]
	if d.isRehashing() {
		t = d.ht[1]
	}
	idx := h & t.mask
	t.buckets[idx] = &entry[V]{key: key, value: value, next: t.buckets[idx]}
	t.used++
}

func (d *Dict[V]) Delete(key string) bool {
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
		var prev *entry[V]
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

func (d *Dict[V]) RandomEntry() (string, V, bool) {
	var zero V
	if d.Len() == 0 {
		return "", zero, false
	}
	if d.isRehashing() {
		d.rehash(1)
	}
	var head *entry[V]
	for head == nil {
		if d.isRehashing() {

			s0 := d.ht[0].size()
			total := s0 - uint64(d.rehashIdx) + d.ht[1].size()
			i := uint64(d.rehashIdx) + uint64(rand.Int63n(int64(total)))
			if i < s0 {
				head = d.ht[0].buckets[i]
			} else {
				head = d.ht[1].buckets[i-s0]
			}
		} else {
			head = d.ht[0].buckets[rand.Int63n(int64(d.ht[0].size()))]
		}
	}
	chainLen := 0
	for e := head; e != nil; e = e.next {
		chainLen++
	}
	e := head
	for k := rand.Intn(chainLen); k > 0; k-- {
		e = e.next
	}
	return e.key, e.value, true
}

func (d *Dict[V]) expandIfNeeded() {
	if d.isRehashing() {
		return
	}
	t := d.ht[0]
	if uint64(t.used) >= t.size() {
		d.startRehash(t.size() * 2)
	}
}

func (d *Dict[V]) shrinkIfNeeded() {
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

func (d *Dict[V]) startRehash(newSize uint64) {
	d.ht[1] = newTable[V](newSize)
	d.rehashIdx = 0
}

func (d *Dict[V]) rehash(n int) bool {
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

func (d *Dict[V]) RehashFor(budget time.Duration) {
	start := time.Now()
	for d.rehash(100) {
		if time.Since(start) > budget {
			return
		}
	}
}

func (d *Dict[V]) Stats() string {
	s := fmt.Sprintf("table0: size=%d used=%d", d.ht[0].size(), d.ht[0].used)
	if d.isRehashing() {
		s += fmt.Sprintf(" | table1: size=%d used=%d | rehashing, next old bucket=%d",
			d.ht[1].size(), d.ht[1].used, d.rehashIdx)
	} else {
		s += " | not rehashing"
	}
	return s
}
