package main

import (
	"sync"
	"time"
)

const (
	cronInterval      = 100 * time.Millisecond 
	rehashBudget      = time.Millisecond       
	expireSampleSize  = 20                     
	expireCycleBudget = 25 * time.Millisecond  
)

type Store struct {
	mu      sync.Mutex
	data    *Dict[string]
	expires *Dict[int64]  
}

func NewStore() *Store {
	s := &Store{data: NewDict[string](), expires: NewDict[int64]()}
	go s.serverCron()
	return s
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}


func (s *Store) expireIfNeeded(key string) bool {
	at, ok := s.expires.Get(key)
	if !ok || at > nowMillis() {
		return false
	}
	s.data.Delete(key)
	s.expires.Delete(key)
	return true
}

func (s *Store) lookup(key string) (string, bool) {
	if s.expireIfNeeded(key) {
		return "", false
	}
	return s.data.Get(key)
}


func (s *Store) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lookup(key)
}

func (s *Store) Set(key, value string, expireAt int64, keepTTL bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireIfNeeded(key) 
	s.data.Set(key, value)
	switch {
	case expireAt > 0:
		s.expires.Set(key, expireAt)
	case !keepTTL:
		s.expires.Delete(key)
	}
}

func (s *Store) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.expireIfNeeded(key) {
		return false
	}
	s.expires.Delete(key)
	return s.data.Delete(key)
}

func (s *Store) Expire(key string, at int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.lookup(key); !ok {
		return false
	}
	if at <= nowMillis() {
		s.data.Delete(key)
		s.expires.Delete(key)
		return true
	}
	s.expires.Set(key, at)
	return true
}

func (s *Store) Persist(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.lookup(key); !ok {
		return false
	}
	return s.expires.Delete(key)
}

func (s *Store) TTLMillis(key string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.lookup(key); !ok {
		return -2
	}
	at, ok := s.expires.Get(key)
	if !ok {
		return -1
	}
	ttl := at - nowMillis()
	if ttl < 0 {
		ttl = 0
	}
	return ttl
}

func (s *Store) Stats() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return "data -> " + s.data.Stats() + " || expires -> " + s.expires.Stats()
}

func (s *Store) serverCron() {
	ticker := time.NewTicker(cronInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		s.activeExpireCycle(expireCycleBudget)
		s.data.RehashFor(rehashBudget)
		s.expires.RehashFor(rehashBudget)
		s.mu.Unlock()
	}
}

func (s *Store) activeExpireCycle(budget time.Duration) {
	start := time.Now()
	for {
		sample := expireSampleSize
		if left := s.expires.Len(); left < sample {
			sample = left
		}
		if sample == 0 {
			return
		}

		now := nowMillis()
		expired := 0
		for i := 0; i < sample; i++ {
			key, at, ok := s.expires.RandomEntry()
			if !ok {
				break
			}
			if at <= now {
				s.data.Delete(key)
				s.expires.Delete(key)
				expired++
			}
		}

		if expired*4 <= sample { 
			return
		}
		if time.Since(start) > budget {
			return
		}
	}
}