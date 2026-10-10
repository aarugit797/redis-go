package main

import (
	"fmt"
	"strconv"
	"strings"
)

type saveRule struct {
	seconds int64
	changes int64
}

type Config struct {
	Port int
	Dir  string

	AppendOnly  bool
	AppendFsync string
	SaveRules   []saveRule

	MaxMemory       int64
	MaxMemoryPolicy string
	MaxMemorySample int
	LFULogFactor    int
	LFUDecayTime    int
}

func DefaultConfig() *Config {
	return &Config{
		Port:            6380,
		Dir:             ".",
		AppendFsync:     "everysec",
		SaveRules:       []saveRule{{3600, 1}, {300, 100}, {60, 10000}},
		MaxMemoryPolicy: "noeviction",
		MaxMemorySample: 5,
		LFULogFactor:    10,
		LFUDecayTime:    1,
	}
}

var validPolicies = map[string]bool{
	"noeviction": true, "allkeys-lru": true, "volatile-lru": true,
	"allkeys-lfu": true, "volatile-lfu": true, "allkeys-random": true,
	"volatile-random": true, "volatile-ttl": true,
}

func parseMemory(s string) (int64, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		m      int64
	}{{"gb", 1 << 30}, {"mb", 1 << 20}, {"kb", 1 << 10}, {"b", 1}} {
		if strings.HasSuffix(s, u.suffix) {
			mult = u.m
			s = strings.TrimSuffix(s, u.suffix)
			break
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid memory size %q", s)
	}
	return n * mult, nil
}

func parseSaveRules(s string) ([]saveRule, error) {
	fields := strings.Fields(s)
	if len(fields)%2 != 0 {
		return nil, fmt.Errorf("save rules must be pairs of <seconds> <changes>")
	}
	var rules []saveRule
	for i := 0; i < len(fields); i += 2 {
		sec, err1 := strconv.ParseInt(fields[i], 10, 64)
		ch, err2 := strconv.ParseInt(fields[i+1], 10, 64)
		if err1 != nil || err2 != nil || sec <= 0 || ch <= 0 {
			return nil, fmt.Errorf("invalid save rule %q %q", fields[i], fields[i+1])
		}
		rules = append(rules, saveRule{sec, ch})
	}
	return rules, nil
}

func (c *Config) saveRulesString() string {
	var parts []string
	for _, r := range c.SaveRules {
		parts = append(parts, fmt.Sprintf("%d %d", r.seconds, r.changes))
	}
	return strings.Join(parts, " ")
}
