package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"api-watchtower/internal/api"
	"api-watchtower/internal/config"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create context that listens for the interrupt signal from the OS
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initialize and start the server
	server, err := api.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Start server in a goroutine
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-ctx.Done()

	// Shutdown gracefully
	if err := server.Shutdown(context.Background()); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}
}
