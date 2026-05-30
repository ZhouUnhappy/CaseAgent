package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"caseagent/internal/api/handler"
	"caseagent/internal/api/router"
	"caseagent/internal/config"
	"caseagent/internal/db"
	"caseagent/internal/logging"
	suggestionservice "caseagent/internal/service/suggestion"
)

func main() {
	logging.Init()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := config.Load("configs/config.yaml"); err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if err := db.Init(ctx); err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("database initialized")
	if days := config.Get().Suggestion.AutoDismissPendingDays; days > 0 {
		suggestionservice.StartExpiredPendingCleanup(ctx, db.DB, time.Duration(days)*24*time.Hour, 24*time.Hour)
	}

	h := handler.New(db.DB)
	r := router.SetupRouter(h)

	addr := ":8080"
	if config.Get().Server.Port != 0 {
		addr = fmt.Sprintf(":%d", config.Get().Server.Port)
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		slog.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	slog.Info("server shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
