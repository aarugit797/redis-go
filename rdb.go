package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"
)

const (
	rdbMagic      = "GORDB01"
	rdbOpExpire   = 0xFC
	rdbOpEOF      = 0xFF
	rdbFileName   = "dump.rdb"
	aofFileName   = "appendonly.aof"
	minAOFRewrite = 64 << 20
)

type rdbWriter struct{ buf bytes.Buffer }

func (w *rdbWriter) uvarint(n uint64) {
	var tmp [binary.MaxVarintLen64]byte
	w.buf.Write(tmp[:binary.PutUvarint(tmp[:], n)])
}

func (w *rdbWriter) str(s string) {
	w.uvarint(uint64(len(s)))
	w.buf.WriteString(s)
}

func (w *rdbWriter) u64(n uint64) {
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], n)
	w.buf.Write(tmp[:])
}

func (s *Store) serializeRDB() []byte {
	w := &rdbWriter{}
	w.buf.WriteString(rdbMagic)
	now := nowMillis()
	s.data.ForEach(func(key string, obj *Object) {
		if at, ok := s.expires.Get(key); ok {
			if at <= now {
				return
			}
			w.buf.WriteByte(rdbOpExpire)
			w.u64(uint64(at))
		}
		w.buf.WriteByte(byte(obj.typ))
		w.str(key)
		switch v := obj.value.(type) {
		case string:
			w.str(v)
		case *List:
			items := v.Range(0, -1)
			w.uvarint(uint64(len(items)))
			for _, it := range items {
				w.str(it)
			}
		case *Hash:
			all := v.All()
			w.uvarint(uint64(len(all) / 2))
			for _, x := range all {
				w.str(x)
			}
		case *Set:
			members := v.Members()
			w.uvarint(uint64(len(members)))
			for _, m := range members {
				w.str(m)
			}
		case *ZSet:
			entries := v.Range(0, -1)
			w.uvarint(uint64(len(entries)))
			for _, e := range entries {
				w.str(e.member)
				w.u64(math.Float64bits(e.score))
			}
		}
	})
	w.buf.WriteByte(rdbOpEOF)
	sum := crc32.ChecksumIEEE(w.buf.Bytes())
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], sum)
	w.buf.Write(tmp[:])
	return w.buf.Bytes()
}

type rdbReader struct{ r *bytes.Reader }

func (r *rdbReader) uvarint() (uint64, error) { return binary.ReadUvarint(r.r) }

func (r *rdbReader) str() (string, error) {
	n, err := r.uvarint()
	if err != nil {
		return "", err
	}
	if n > uint64(r.r.Len()) {
		return "", io.ErrUnexpectedEOF
	}
	b := make([]byte, n)
	_, err = io.ReadFull(r.r, b)
	return string(b), err
}

func (r *rdbReader) u64() (uint64, error) {
	var tmp [8]byte
	if _, err := io.ReadFull(r.r, tmp[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(tmp[:]), nil
}

func (s *Store) loadRDB(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) < len(rdbMagic)+5 || string(data[:len(rdbMagic)]) != rdbMagic {
		return errors.New("not a GORDB snapshot")
	}
	body, sumBytes := data[:len(data)-4], data[len(data)-4:]
	if crc32.ChecksumIEEE(body) != binary.LittleEndian.Uint32(sumBytes) {
		return errors.New("snapshot checksum mismatch: file is corrupted")
	}
	r := &rdbReader{bytes.NewReader(body[len(rdbMagic):])}
	now := nowMillis()
	for {
		op, err := r.r.ReadByte()
		if err != nil {
			return err
		}
		if op == rdbOpEOF {
			return nil
		}
		var expireAt int64
		if op == rdbOpExpire {
			at, err := r.u64()
			if err != nil {
				return err
			}
			expireAt = int64(at)
			if op, err = r.r.ReadByte(); err != nil {
				return err
			}
		}
		key, err := r.str()
		if err != nil {
			return err
		}
		obj, err := r.readValue(ObjType(op))
		if err != nil {
			return fmt.Errorf("key %q: %w", key, err)
		}
		if expireAt != 0 && expireAt <= now {
			continue
		}
		s.addObject(key, obj)
		if expireAt != 0 {
			s.expires.Set(key, expireAt)
		}
	}
}

func (r *rdbReader) readValue(t ObjType) (*Object, error) {
	if t == TypeString {
		v, err := r.str()
		return &Object{typ: t, value: v}, err
	}
	n, err := r.uvarint()
	if err != nil {
		return nil, err
	}
	switch t {
	case TypeList:
		l := NewList()
		for i := uint64(0); i < n; i++ {
			v, err := r.str()
			if err != nil {
				return nil, err
			}
			l.PushBack(v)
		}
		return &Object{typ: t, value: l}, nil
	case TypeHash:
		h := NewHash()
		for i := uint64(0); i < n; i++ {
			f, err := r.str()
			if err != nil {
				return nil, err
			}
			v, err := r.str()
			if err != nil {
				return nil, err
			}
			h.Set(f, v)
		}
		return &Object{typ: t, value: h}, nil
	case TypeSet:
		set := NewSet()
		for i := uint64(0); i < n; i++ {
			m, err := r.str()
			if err != nil {
				return nil, err
			}
			set.Add(m)
		}
		return &Object{typ: t, value: set}, nil
	case TypeZSet:
		z := NewZSet()
		for i := uint64(0); i < n; i++ {
			m, err := r.str()
			if err != nil {
				return nil, err
			}
			bits, err := r.u64()
			if err != nil {
				return nil, err
			}
			z.Add(m, math.Float64frombits(bits))
		}
		return &Object{typ: t, value: z}, nil
	}
	return nil, fmt.Errorf("unknown type byte %d", t)
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	if _, err = w.Write(data); err == nil {
		err = w.Flush()
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) rdbPath() string { return filepath.Join(s.cfg.Dir, rdbFileName) }
func (s *Store) aofPath() string { return filepath.Join(s.cfg.Dir, aofFileName) }

func (s *Store) save() error {
	if err := writeFileAtomic(s.rdbPath(), s.serializeRDB()); err != nil {
		return err
	}
	s.dirty = 0
	s.lastSave = time.Now()
	return nil
}

func (s *Store) bgsave() error {
	if s.bgsaveRunning {
		return errors.New("Background save already in progress")
	}
	buf := s.serializeRDB()
	s.dirtyAtSaveStart = s.dirty
	s.bgsaveRunning = true
	path := s.rdbPath()
	go func() { s.saveDone <- writeFileAtomic(path, buf) }()
	return nil
}

func (s *Store) persistenceCron() {
	select {
	case err := <-s.saveDone:
		s.bgsaveRunning = false
		if err != nil {
			logf("background save failed: %v", err)
		} else {
			s.dirty -= s.dirtyAtSaveStart
			s.lastSave = time.Now()
			logf("background save done")
		}
	default:
	}

	if !s.bgsaveRunning {
		elapsed := int64(time.Since(s.lastSave).Seconds())
		for _, r := range s.cfg.SaveRules {
			if s.dirty >= r.changes && elapsed >= r.seconds {
				logf("%d changes in %d seconds, saving", r.changes, r.seconds)
				if err := s.bgsave(); err != nil {
					logf("bgsave: %v", err)
				}
				break
			}
		}
	}

	if s.aof != nil && s.aof.NeedsRewrite() {
		logf("AOF doubled in size, rewriting")
		s.bgrewriteaof()
	}
}

func (s *Store) loadData() error {
	s.loading = true
	defer func() { s.loading = false }()

	aofExists := fileExists(s.aofPath())
	rdbExists := fileExists(s.rdbPath())
	switch {
	case s.cfg.AppendOnly && aofExists:
		if err := s.loadAOF(s.aofPath()); err != nil {
			return fmt.Errorf("loading AOF: %w", err)
		}
		logf("loaded %d keys from %s", s.data.Len(), aofFileName)
	case rdbExists:
		if err := s.loadRDB(s.rdbPath()); err != nil {
			return fmt.Errorf("loading snapshot: %w", err)
		}
		logf("loaded %d keys from %s", s.data.Len(), rdbFileName)
	}

	if s.cfg.AppendOnly && !aofExists {
		if err := writeFileAtomic(s.aofPath(), s.rewriteAOFContent()); err != nil {
			return err
		}
		logf("no AOF found: created one from the current data")
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
