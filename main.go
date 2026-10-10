package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	cfg := DefaultConfig()
	var save string
	flag.IntVar(&cfg.Port, "port", cfg.Port, "TCP port")
	flag.StringVar(&cfg.Dir, "dir", cfg.Dir, "directory for dump.rdb")
	flag.StringVar(&save, "save", cfg.saveRulesString(), `snapshot rules "<seconds> <changes> ..."; "" disables`)
	flag.Parse()

	rules, err := parseSaveRules(save)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg.SaveRules = rules

	store := NewStore(cfg)
	if err := store.loadData(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	srv := NewServer(cfg, store)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
