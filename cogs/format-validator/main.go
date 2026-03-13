package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Printf("format-validator starting")

	_ = DefaultConfig() // TODO: load from config, wire to bus transport

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("format-validator shutting down")
}
