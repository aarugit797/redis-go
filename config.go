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
}

func DefaultConfig() *Config {
	return &Config{
		Port:        6380,
		Dir:         ".",
		AppendFsync: "everysec",
		SaveRules:   []saveRule{{3600, 1}, {300, 100}, {60, 10000}},
	}
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
