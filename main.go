package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	cfg := DefaultConfig()
	flag.IntVar(&cfg.Port, "port", cfg.Port, "TCP port")
	flag.Parse()

	srv := NewServer(cfg, NewStore(cfg))
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
