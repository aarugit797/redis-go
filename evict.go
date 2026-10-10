package main

import "math"

const evictionPoolSize = 16

type poolEntry struct {
	key   string
	score uint64
	used  bool
}

func (s *Store) freeMemoryIfNeeded() bool {
	if s.cfg.MaxMemory == 0 || s.usedMemory <= s.cfg.MaxMemory {
		return true
	}
	if s.cfg.MaxMemoryPolicy == "noeviction" {
		return false
	}
	for s.usedMemory > s.cfg.MaxMemory {
		key, ok := s.pickVictim()
		if !ok {
			return false
		}
		s.deleteKey(key)
		s.evictedKeys++
		s.propagate("DEL", key)
	}
	return true
}

func (s *Store) volatilePolicy() bool {
	switch s.cfg.MaxMemoryPolicy {
	case "volatile-lru", "volatile-lfu", "volatile-random", "volatile-ttl":
		return true
	}
	return false
}

func (s *Store) sampleKey() (string, bool) {
	if s.volatilePolicy() {
		key, _, ok := s.expires.RandomEntry()
		return key, ok
	}
	key, _, ok := s.data.RandomEntry()
	return key, ok
}

func (s *Store) pickVictim() (string, bool) {
	switch s.cfg.MaxMemoryPolicy {
	case "allkeys-random", "volatile-random":
		return s.sampleKey()
	}

	for attempt := 0; attempt < 16; attempt++ {
		s.populatePool()
		if key, ok := s.popBest(); ok {
			return key, true
		}
		if s.volatilePolicy() && s.expires.Len() == 0 || s.data.Len() == 0 {
			return "", false
		}
	}
	return "", false
}

func (s *Store) score(key string, obj *Object) uint64 {
	switch s.cfg.MaxMemoryPolicy {
	case "allkeys-lfu", "volatile-lfu":
		return 255 - uint64(s.lfuDecayed(obj))
	case "volatile-ttl":
		at, _ := s.expires.Get(key)
		return math.MaxUint64 - uint64(at)
	default:
		return uint64(s.idleSeconds(obj))
	}
}

func (s *Store) populatePool() {
	for i := 0; i < s.cfg.MaxMemorySample; i++ {
		key, ok := s.sampleKey()
		if !ok {
			return
		}
		obj, ok := s.data.Get(key)
		if !ok {
			continue
		}
		s.poolInsert(key, s.score(key, obj))
	}
}

// The pool stays sorted by score ascending, so the best victim is last.
func (s *Store) poolInsert(key string, score uint64) {
	p := &s.pool
	for i := range p {
		if p[i].used && p[i].key == key {
			p[i].score = score
			return
		}
	}

	k := 0
	for k < evictionPoolSize && p[k].used && p[k].score < score {
		k++
	}
	switch {
	case k == 0 && p[evictionPoolSize-1].used:
		return
	case k < evictionPoolSize && !p[evictionPoolSize-1].used:
		copy(p[k+1:], p[k:evictionPoolSize-1])
	default:
		k--
		copy(p[:k], p[1:k+1])
	}
	p[k] = poolEntry{key: key, score: score, used: true}
}

func (s *Store) popBest() (string, bool) {
	for i := evictionPoolSize - 1; i >= 0; i-- {
		if !s.pool[i].used {
			continue
		}
		key := s.pool[i].key
		s.pool[i] = poolEntry{}
		if _, ok := s.data.Get(key); !ok {
			continue
		}
		if s.volatilePolicy() {
			if _, ok := s.expires.Get(key); !ok {
				continue
			}
		}
		return key, true
	}
	return "", false
}
