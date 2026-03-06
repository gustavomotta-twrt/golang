package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TWRT/integration-mapper/internal/api"
	"github.com/TWRT/integration-mapper/internal/repository"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Error("failed to load .env", "err", err)
		os.Exit(1)
	}

	asanaToken := os.Getenv("ASANA_TOKEN")
	clickUpToken := os.Getenv("CLICKUP_TOKEN")
	if asanaToken == "" || clickUpToken == "" {
		slog.Error("ASANA_TOKEN and CLICKUP_TOKEN must be set")
		os.Exit(1)
	}

	db, err := repository.InitDB("./migrator.db")
	if err != nil {
		slog.Error("failed to initialize database", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("database initialized")

	router := api.SetupRouter(db, asanaToken, clickUpToken)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("server starting", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()
	slog.Info("shutdown signal received, waiting for active requests...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced shutdown", "err", err)
	} else {
		slog.Info("server stopped cleanly")
	}
}
