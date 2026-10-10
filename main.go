package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg := DefaultConfig()
	var maxmemory, save string
	flag.IntVar(&cfg.Port, "port", cfg.Port, "TCP port")
	flag.StringVar(&cfg.Mode, "mode", cfg.Mode, "execution mode: single (one goroutine owns the data) or locked (global mutex)")
	flag.StringVar(&cfg.Dir, "dir", cfg.Dir, "directory for dump.rdb and appendonly.aof")
	flag.BoolVar(&cfg.AppendOnly, "appendonly", cfg.AppendOnly, "enable the append-only file")
	flag.StringVar(&cfg.AppendFsync, "appendfsync", cfg.AppendFsync, "always | everysec | no")
	flag.StringVar(&save, "save", cfg.saveRulesString(), `snapshot rules "<seconds> <changes> ..."; "" disables`)
	flag.StringVar(&maxmemory, "maxmemory", "0", "memory limit, e.g. 100mb; 0 = none")
	flag.StringVar(&cfg.MaxMemoryPolicy, "maxmemory-policy", cfg.MaxMemoryPolicy, "eviction policy")
	flag.IntVar(&cfg.MaxMemorySample, "maxmemory-samples", cfg.MaxMemorySample, "keys sampled per eviction round")
	flag.IntVar(&cfg.LFULogFactor, "lfu-log-factor", cfg.LFULogFactor, "LFU counter growth factor")
	flag.IntVar(&cfg.LFUDecayTime, "lfu-decay-time", cfg.LFUDecayTime, "minutes per point of LFU decay")
	flag.Parse()

	if err := finishConfig(cfg, maxmemory, save); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	store := NewStore(cfg)
	if err := store.loadData(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cfg.AppendOnly {
		aof, err := OpenAOF(store.aofPath(), cfg.AppendFsync)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		store.aof = aof
		defer aof.Close()
	}

	srv := NewServer(cfg, store)

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		logf("shutting down")
		if store.aof != nil {
			store.aof.Close()
		}
		os.Exit(0)
	}()

	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func finishConfig(cfg *Config, maxmemory, save string) error {
	if cfg.Mode != "single" && cfg.Mode != "locked" {
		return fmt.Errorf("-mode must be single or locked")
	}
	switch cfg.AppendFsync {
	case "always", "everysec", "no":
	default:
		return fmt.Errorf("-appendfsync must be always, everysec or no")
	}
	if !validPolicies[cfg.MaxMemoryPolicy] {
		return fmt.Errorf("unknown -maxmemory-policy %q", cfg.MaxMemoryPolicy)
	}
	if cfg.MaxMemorySample < 1 {
		return fmt.Errorf("-maxmemory-samples must be at least 1")
	}
	mm, err := parseMemory(maxmemory)
	if err != nil {
		return err
	}
	cfg.MaxMemory = mm
	if cfg.SaveRules, err = parseSaveRules(save); err != nil {
		return err
	}
	return nil
}
