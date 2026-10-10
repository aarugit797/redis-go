package main

import (
	"log"
	"time"
)

const (
	cronInterval      = 100 * time.Millisecond
	rehashBudget      = time.Millisecond
	expireSampleSize  = 20
	expireCycleBudget = 25 * time.Millisecond
)

type Store struct {
	cfg     *Config
	data    *Dict[string]
	expires *Dict[int64]

	expiredKeys   int64
	totalCommands int64
}

func NewStore(cfg *Config) *Store {
	return &Store{cfg: cfg, data: NewDict[string](), expires: NewDict[int64]()}
}

func nowMillis() int64 { return time.Now().UnixMilli() }

func (s *Store) expireIfNeeded(key string) bool {
	at, ok := s.expires.Get(key)
	if !ok || at > nowMillis() {
		return false
	}
	s.deleteKey(key)
	s.expiredKeys++
	return true
}

func (s *Store) lookup(key string) (string, bool) {
	if s.expireIfNeeded(key) {
		return "", false
	}
	return s.data.Get(key)
}

func (s *Store) deleteKey(key string) bool {
	s.expires.Delete(key)
	return s.data.Delete(key)
}

func (s *Store) flushAll() {
	s.data = NewDict[string]()
	s.expires = NewDict[int64]()
}

func (s *Store) cron() {
	s.activeExpireCycle(expireCycleBudget)
	s.data.RehashFor(rehashBudget)
	s.expires.RehashFor(rehashBudget)
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
				s.deleteKey(key)
				s.expiredKeys++
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

func logf(format string, args ...any) { log.Printf(format, args...) }
