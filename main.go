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
	var save string
	flag.IntVar(&cfg.Port, "port", cfg.Port, "TCP port")
	flag.StringVar(&cfg.Dir, "dir", cfg.Dir, "directory for dump.rdb and appendonly.aof")
	flag.BoolVar(&cfg.AppendOnly, "appendonly", cfg.AppendOnly, "enable the append-only file")
	flag.StringVar(&cfg.AppendFsync, "appendfsync", cfg.AppendFsync, "always | everysec | no")
	flag.StringVar(&save, "save", cfg.saveRulesString(), `snapshot rules "<seconds> <changes> ..."; "" disables`)
	flag.Parse()

	if err := finishConfig(cfg, save); err != nil {
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

func finishConfig(cfg *Config, save string) error {
	switch cfg.AppendFsync {
	case "always", "everysec", "no":
	default:
		return fmt.Errorf("-appendfsync must be always, everysec or no")
	}
	var err error
	if cfg.SaveRules, err = parseSaveRules(save); err != nil {
		return err
	}
	return nil
}
