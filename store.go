package main

import "sync"

// Store is the concurrency boundary around Dict.
//
// Every Dict operation mutates the dict, including Get, which advances the
// incremental rehash cursor. So all four methods below take the exclusive
// Lock: a shared read lock would let two concurrent GETs step the cursor at
// once and corrupt the bucket chains.
type Store struct {
	mu   sync.Mutex
	dict *Dict
}

func NewStore() *Store {
	return &Store{dict: NewDict()}
}

func (s *Store) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dict.Set(key, value)
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dict.Get(key)
}

func (s *Store) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dict.Delete(key)
}

func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dict.Len()
}

func (s *Store) Stats() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dict.Stats()
}
