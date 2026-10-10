package main

import "sort"

type zEntry struct {
	member string
	score  float64
}

func zLess(a, b zEntry) bool {
	return a.score < b.score || (a.score == b.score && a.member < b.member)
}

type ZSet struct {
	small   []zEntry
	sl      *SkipList
	dict    *Dict[float64]
	payload int
}

func NewZSet() *ZSet { return &ZSet{} }

func (z *ZSet) isBig() bool { return z.sl != nil }

func (z *ZSet) Encoding() string {
	if z.isBig() {
		return "skiplist"
	}
	return "listpack"
}

func (z *ZSet) Card() int {
	if z.isBig() {
		return z.sl.length
	}
	return len(z.small)
}

func (z *ZSet) Mem() int {
	if z.isBig() {
		return 64 + 100*z.Card() + z.payload
	}
	return 7 + 11*z.Card() + z.payload
}

func (z *ZSet) indexSmall(member string) int {
	for i, e := range z.small {
		if e.member == member {
			return i
		}
	}
	return -1
}

func (z *ZSet) insertSmall(e zEntry) {
	i := sort.Search(len(z.small), func(i int) bool { return !zLess(z.small[i], e) })
	z.small = append(z.small, zEntry{})
	copy(z.small[i+1:], z.small[i:])
	z.small[i] = e
}

func (z *ZSet) Add(member string, score float64) bool {
	if !z.isBig() {
		if i := z.indexSmall(member); i >= 0 {
			if z.small[i].score != score {
				z.small = append(z.small[:i], z.small[i+1:]...)
				z.insertSmall(zEntry{member, score})
			}
			return false
		}
		if len(z.small)+1 <= zsetMaxListpackEntries && len(member) <= zsetMaxListpackValue {
			z.insertSmall(zEntry{member, score})
			z.payload += len(member)
			return true
		}
		z.convert()
	}
	if old, ok := z.dict.Get(member); ok {
		if old != score {
			z.sl.Delete(member, old)
			z.sl.Insert(member, score)
			z.dict.Set(member, score)
		}
		return false
	}
	z.sl.Insert(member, score)
	z.dict.Set(member, score)
	z.payload += len(member)
	return true
}

func (z *ZSet) convert() {
	z.sl = NewSkipList()
	z.dict = NewDict[float64]()
	for _, e := range z.small {
		z.sl.Insert(e.member, e.score)
		z.dict.Set(e.member, e.score)
	}
	z.small = nil
}

func (z *ZSet) Score(member string) (float64, bool) {
	if z.isBig() {
		return z.dict.Get(member)
	}
	if i := z.indexSmall(member); i >= 0 {
		return z.small[i].score, true
	}
	return 0, false
}

func (z *ZSet) Remove(member string) bool {
	if z.isBig() {
		score, ok := z.dict.Get(member)
		if !ok {
			return false
		}
		z.sl.Delete(member, score)
		z.dict.Delete(member)
	} else {
		i := z.indexSmall(member)
		if i < 0 {
			return false
		}
		z.small = append(z.small[:i], z.small[i+1:]...)
	}
	z.payload -= len(member)
	return true
}

func (z *ZSet) Rank(member string) (int, bool) {
	if z.isBig() {
		score, ok := z.dict.Get(member)
		if !ok {
			return 0, false
		}
		return z.sl.Rank(member, score) - 1, true
	}
	i := z.indexSmall(member)
	return i, i >= 0
}

func (z *ZSet) Range(start, stop int) []zEntry {
	start, stop, ok := normalizeRange(start, stop, z.Card())
	if !ok {
		return nil
	}
	if !z.isBig() {
		out := make([]zEntry, stop-start+1)
		copy(out, z.small[start:stop+1])
		return out
	}
	out := make([]zEntry, 0, stop-start+1)
	for x := z.sl.NodeAtRank(start + 1); x != nil && len(out) < stop-start+1; x = x.level[0].forward {
		out = append(out, zEntry{x.member, x.score})
	}
	return out
}

func (z *ZSet) RangeByScore(min, max float64) []zEntry {
	var out []zEntry
	if !z.isBig() {
		for _, e := range z.small {
			if e.score >= min && e.score <= max {
				out = append(out, e)
			}
		}
		return out
	}
	for x := z.sl.FirstAtLeast(min); x != nil && x.score <= max; x = x.level[0].forward {
		out = append(out, zEntry{x.member, x.score})
	}
	return out
}
