package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Printf("toxicity-gate starting")

	cfg := DefaultConfig()
	_, err := LoadBlocklist(cfg.BlocklistPath)
	if err != nil {
		log.Printf("warning: could not load blocklist: %v", err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("toxicity-gate shutting down")
}
