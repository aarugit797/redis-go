package main

import (
	"math"
	"sort"
	"strconv"
)

type setEncoding int

const (
	setIntset setEncoding = iota
	setListpack
	setHashtable
)

type Set struct {
	enc     setEncoding
	ints    []int64
	width   int
	small   []string
	big     *Dict[struct{}]
	payload int
}

func NewSet() *Set { return &Set{enc: setIntset, width: 2} }

func (s *Set) Encoding() string {
	switch s.enc {
	case setIntset:
		return "intset"
	case setListpack:
		return "listpack"
	default:
		return "hashtable"
	}
}

func (s *Set) Mem() int {
	switch s.enc {
	case setIntset:
		return 8 + s.width*len(s.ints)
	case setListpack:
		return 7 + 2*len(s.small) + s.payload
	default:
		return 64 + 56*s.big.Len() + s.payload
	}
}

func intWidth(n int64) int {
	switch {
	case n >= math.MinInt16 && n <= math.MaxInt16:
		return 2
	case n >= math.MinInt32 && n <= math.MaxInt32:
		return 4
	default:
		return 8
	}
}

func (s *Set) searchInt(n int64) (int, bool) {
	i := sort.Search(len(s.ints), func(i int) bool { return s.ints[i] >= n })
	return i, i < len(s.ints) && s.ints[i] == n
}

func (s *Set) indexSmall(m string) int {
	for i, v := range s.small {
		if v == m {
			return i
		}
	}
	return -1
}

func (s *Set) Add(m string) bool {
	switch s.enc {
	case setIntset:
		if n, ok := parseCanonicalInt(m); ok {
			i, found := s.searchInt(n)
			if found {
				return false
			}
			if len(s.ints)+1 <= setMaxIntsetEntries {
				if w := intWidth(n); w > s.width {
					s.width = w
				}
				s.ints = append(s.ints, 0)
				copy(s.ints[i+1:], s.ints[i:])
				s.ints[i] = n
				s.payload += len(m)
				return true
			}
		}
		s.convertFromIntset(m)
		return s.Add(m)

	case setListpack:
		if s.indexSmall(m) >= 0 {
			return false
		}
		if len(s.small)+1 <= setMaxListpackEntries && len(m) <= setMaxListpackValue {
			s.small = append(s.small, m)
			s.payload += len(m)
			return true
		}
		s.toHashtable()
		return s.Add(m)

	default:
		if _, ok := s.big.Get(m); ok {
			return false
		}
		s.big.Set(m, struct{}{})
		s.payload += len(m)
		return true
	}
}

func (s *Set) convertFromIntset(next string) {
	members := s.Members()
	if len(members)+1 <= setMaxListpackEntries && len(next) <= setMaxListpackValue {
		s.small = members
		s.ints = nil
		s.enc = setListpack
		return
	}
	s.toHashtable()
}

func (s *Set) toHashtable() {
	members := s.Members()
	s.big = NewDict[struct{}]()
	for _, m := range members {
		s.big.Set(m, struct{}{})
	}
	s.ints, s.small = nil, nil
	s.enc = setHashtable
}

func (s *Set) Remove(m string) bool {
	switch s.enc {
	case setIntset:
		n, ok := parseCanonicalInt(m)
		if !ok {
			return false
		}
		i, found := s.searchInt(n)
		if !found {
			return false
		}
		s.ints = append(s.ints[:i], s.ints[i+1:]...)
	case setListpack:
		i := s.indexSmall(m)
		if i < 0 {
			return false
		}
		s.small = append(s.small[:i], s.small[i+1:]...)
	default:
		if !s.big.Delete(m) {
			return false
		}
	}
	s.payload -= len(m)
	return true
}

func (s *Set) IsMember(m string) bool {
	switch s.enc {
	case setIntset:
		n, ok := parseCanonicalInt(m)
		if !ok {
			return false
		}
		_, found := s.searchInt(n)
		return found
	case setListpack:
		return s.indexSmall(m) >= 0
	default:
		_, ok := s.big.Get(m)
		return ok
	}
}

func (s *Set) Card() int {
	switch s.enc {
	case setIntset:
		return len(s.ints)
	case setListpack:
		return len(s.small)
	default:
		return s.big.Len()
	}
}

func (s *Set) Members() []string {
	switch s.enc {
	case setIntset:
		out := make([]string, len(s.ints))
		for i, n := range s.ints {
			out[i] = strconv.FormatInt(n, 10)
		}
		return out
	case setListpack:
		out := make([]string, len(s.small))
		copy(out, s.small)
		return out
	default:
		out := make([]string, 0, s.big.Len())
		s.big.ForEach(func(m string, _ struct{}) { out = append(out, m) })
		return out
	}
}
