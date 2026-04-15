package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"caseagent/internal/api/handler"
	"caseagent/internal/api/router"
	"caseagent/internal/config"
	"caseagent/internal/db"
)

func main() {
	ctx := context.Background()

	// Load config
	if err := config.Load("configs/config.yaml"); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	if err := db.Init(ctx); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	log.Println("Database initialized successfully")

	// Setup router
	h := handler.New(db.DB)
	r := router.SetupRouter(h)

	// Start HTTP server
	addr := ":8080"
	if config.Get().Server.Port != 0 {
		addr = fmt.Sprintf(":%d", config.Get().Server.Port)
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.Printf("Server starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}
	log.Println("Server stopped")
}
