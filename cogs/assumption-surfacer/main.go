package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Printf("assumption-surfacer starting")

	_ = DefaultConfig() // TODO: load from config, wire to bus transport

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("assumption-surfacer shutting down")
}
