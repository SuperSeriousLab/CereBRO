package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Printf("language-detector starting")

	cfg := DefaultConfig()
	profiles, err := LoadProfiles("data/language-profiles")
	if err != nil {
		log.Printf("warning: could not load profiles: %v", err)
	} else {
		log.Printf("loaded %d language profiles", len(profiles))
	}
	_ = cfg

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("language-detector shutting down")
}
