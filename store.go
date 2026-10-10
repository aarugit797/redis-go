package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"
)

const (
	cronInterval      = 100 * time.Millisecond
	rehashBudget      = time.Millisecond
	expireSampleSize  = 20
	expireCycleBudget = 25 * time.Millisecond
	lruClockMax       = 1<<24 - 1
	lfuInitVal        = 5
)

type Store struct {
	cfg     *Config
	data    *Dict[*Object]
	expires *Dict[int64]

	usedMemory int64

	lruClock uint32
	lfuClock uint32

	pool [evictionPoolSize]poolEntry

	aof              *AOF
	loading          bool
	dirty            int64
	dirtyAtSaveStart int64
	lastSave         time.Time
	bgsaveRunning    bool
	saveDone         chan error

	expiredKeys   int64
	evictedKeys   int64
	totalCommands int64
}

func NewStore(cfg *Config) *Store {
	s := &Store{
		cfg:      cfg,
		data:     NewDict[*Object](),
		expires:  NewDict[int64](),
		lastSave: time.Now(),
		saveDone: make(chan error, 1),
	}
	s.updateClocks()
	return s
}

func nowMillis() int64 { return time.Now().UnixMilli() }

func (s *Store) updateClocks() {
	now := time.Now().Unix()
	s.lruClock = uint32(now) & lruClockMax
	s.lfuClock = uint32(now/60) & 0xFFFF
}

func (s *Store) lfuMode() bool {
	p := s.cfg.MaxMemoryPolicy
	return p == "allkeys-lfu" || p == "volatile-lfu"
}

func (s *Store) expireIfNeeded(key string) bool {
	at, ok := s.expires.Get(key)
	if !ok || at > nowMillis() {
		return false
	}
	s.deleteKey(key)
	s.expiredKeys++
	s.propagate("DEL", key)
	return true
}

func (s *Store) lookup(key string) (*Object, bool) {
	if s.expireIfNeeded(key) {
		return nil, false
	}
	obj, ok := s.data.Get(key)
	if ok {
		s.touch(obj)
	}
	return obj, ok
}

func (s *Store) lookupNoTouch(key string) (*Object, bool) {
	if s.expireIfNeeded(key) {
		return nil, false
	}
	return s.data.Get(key)
}

func (s *Store) lookupType(key string, t ObjType) (obj *Object, found, wrongType bool) {
	obj, found = s.lookup(key)
	if found && obj.typ != t {
		return nil, false, true
	}
	return obj, found, false
}

func (s *Store) addObject(key string, obj *Object) {
	s.initAccess(obj)
	s.data.Set(key, obj)
}

func (s *Store) deleteKey(key string) bool {
	obj, ok := s.data.Get(key)
	if !ok {
		return false
	}
	s.usedMemory -= int64(obj.size)
	s.expires.Delete(key)
	s.data.Delete(key)
	return true
}

func (s *Store) refreshSize(key string) {
	obj, ok := s.data.Get(key)
	if !ok {
		return
	}
	n := estimateKeySize(key, obj)
	if _, ok := s.expires.Get(key); ok {
		n += 32
	}
	s.usedMemory += int64(n - obj.size)
	obj.size = n
}

func (s *Store) initAccess(obj *Object) {
	if s.lfuMode() {
		obj.lru = s.lfuClock<<8 | lfuInitVal
	} else {
		obj.lru = s.lruClock
	}
}

func (s *Store) touch(obj *Object) {
	if s.loading {
		return
	}
	if s.lfuMode() {
		counter := s.lfuDecayed(obj)
		counter = s.lfuLogIncr(counter)
		obj.lru = s.lfuClock<<8 | counter
		return
	}
	obj.lru = s.lruClock
}

// The 24-bit clock wraps every ~194 days.
func (s *Store) idleSeconds(obj *Object) uint32 {
	if s.lruClock >= obj.lru {
		return s.lruClock - obj.lru
	}
	return s.lruClock + (lruClockMax - obj.lru)
}

func (s *Store) lfuDecayed(obj *Object) uint32 {
	ldt := obj.lru >> 8
	counter := obj.lru & 0xFF
	var elapsed uint32
	if s.lfuClock >= ldt {
		elapsed = s.lfuClock - ldt
	} else {
		elapsed = 0xFFFF - ldt + s.lfuClock
	}
	if s.cfg.LFUDecayTime <= 0 {
		return counter
	}
	periods := elapsed / uint32(s.cfg.LFUDecayTime)
	if periods >= counter {
		return 0
	}
	return counter - periods
}

// Morris counter: increment with probability 1/((counter-5)*factor+1),
// which lets 8 bits rank millions of hits.
func (s *Store) lfuLogIncr(counter uint32) uint32 {
	if counter == 255 {
		return 255
	}
	base := float64(counter) - lfuInitVal
	if base < 0 {
		base = 0
	}
	p := 1.0 / (base*float64(s.cfg.LFULogFactor) + 1)
	if rand.Float64() < p {
		counter++
	}
	return counter
}

func (s *Store) cron() {
	s.updateClocks()
	s.activeExpireCycle(expireCycleBudget)
	s.data.RehashFor(rehashBudget)
	s.expires.RehashFor(rehashBudget)
	s.persistenceCron()
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
				s.propagate("DEL", key)
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

func (s *Store) propagate(args ...string) {
	if s.loading {
		return
	}
	s.dirty++
	if s.aof != nil {
		s.aof.Append(args)
	}
}

func (s *Store) flushAll() {
	s.data = NewDict[*Object]()
	s.expires = NewDict[int64]()
	s.usedMemory = 0
	s.pool = [evictionPoolSize]poolEntry{}
}

func (s *Store) info(mode string) string {
	aofOn, aofSize, aofRewriting := 0, int64(0), 0
	if s.aof != nil {
		aofOn = 1
		aofSize, aofRewriting = s.aof.Status()
	}
	bg := 0
	if s.bgsaveRunning {
		bg = 1
	}
	return fmt.Sprintf("# Server\r\nexecution_mode:%s\r\n"+
		"# Memory\r\nused_memory_estimate:%d\r\nmaxmemory:%d\r\nmaxmemory_policy:%s\r\n"+
		"# Persistence\r\nloading:0\r\nrdb_changes_since_last_save:%d\r\nrdb_bgsave_in_progress:%d\r\n"+
		"rdb_last_save_time:%d\r\naof_enabled:%d\r\naof_rewrite_in_progress:%d\r\naof_current_size:%d\r\n"+
		"# Stats\r\ntotal_commands_processed:%d\r\nexpired_keys:%d\r\nevicted_keys:%d\r\n"+
		"# Keyspace\r\nkeys:%d\r\nexpires:%d\r\n",
		mode, s.usedMemory, s.cfg.MaxMemory, s.cfg.MaxMemoryPolicy,
		s.dirty, bg, s.lastSave.Unix(), aofOn, aofRewriting, aofSize,
		s.totalCommands, s.expiredKeys, s.evictedKeys, s.data.Len(), s.expires.Len())
}

func logf(format string, args ...any) { log.Printf(format, args...) }
