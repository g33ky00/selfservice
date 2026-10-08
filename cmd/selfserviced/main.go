package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/g33ky00/selfservice/internal/api"
	"github.com/g33ky00/selfservice/internal/auth"
	"github.com/g33ky00/selfservice/internal/core"
)

func main() {
	cfg := core.Config{
		CFToken:     mustEnv("SS_CF_TOKEN"),
		CFAccountID: mustEnv("SS_CF_ACCOUNT_ID"),
		TunnelID:    mustEnv("SS_CF_TUNNEL_ID"),
		PublicHost:  mustEnv("SS_PUBLIC_HOST"),
		StateDir:    env("SS_STATE_DIR", ""),
		GottyBin:    env("SS_GOTTY_BIN", ""),
		DefaultTTL:  parseDuration("SS_DEFAULT_TTL", 15*time.Minute),
		MaxTTL:      parseDuration("SS_MAX_TTL", 2*time.Hour),
	}

	mgr, err := core.NewManager(cfg)
	if err != nil {
		log.Fatalf("selfserviced: init: %v", err)
	}

	addr := env("SS_LISTEN_ADDR", "127.0.0.1:8765")
	secret := os.Getenv("SS_AUTH_SECRET") // empty = no auth (loopback-only)

	srv := &http.Server{
		Addr:         addr,
		Handler:      auth.Middleware(secret, api.NewHandler(mgr)),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("selfserviced listening on %s (host: %s)", addr, cfg.PublicHost)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("selfserviced: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("selfserviced: shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		fmt.Fprintf(os.Stderr, "ERROR: %s is required\n", k)
		os.Exit(1)
	}
	return v
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func parseDuration(k string, def time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	if s, err := strconv.Atoi(v); err == nil {
		return time.Duration(s) * time.Second
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	return def
}
