package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hynor/nshellserver/internal/config"
	"github.com/hynor/nshellserver/internal/db"
	"github.com/hynor/nshellserver/internal/handler"
)

func main() {
	cfg := config.Load()

	mode, err := prepareServerMode(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	store := db.NewStore(database)
	h := handler.New(store)

	r := chi.NewRouter()
	r.Use(middleware.Compress(5))
	r.Use(h.RateLimiter.Middleware)
	r.Use(handler.BodyLimitMiddleware)

	r.Route("/api/v1/sync", func(r chi.Router) {
		r.Use(h.AuthMiddleware)
		r.Post("/workspace/status", h.WorkspaceStatus)
		r.Post("/pull", h.Pull)
		r.Post("/connections/upsert", h.UpsertConnection)
		r.Post("/connections/delete", h.DeleteConnection)
		r.Post("/ssh-keys/upsert", h.UpsertSSHKey)
		r.Post("/ssh-keys/delete", h.DeleteSSHKey)
		r.Post("/proxies/upsert", h.UpsertProxy)
		r.Post("/proxies/delete", h.DeleteProxy)
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if mode.useTLS {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			log.Fatalf("failed to load TLS cert/key: %v", err)
		}
		srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if mode.warning != "" {
			log.Print(mode.warning)
		}

		var err error
		if mode.useTLS {
			log.Printf("listening on %s (TLS)", cfg.Addr)
			err = srv.ListenAndServeTLS("", "")
		} else {
			log.Printf("listening on %s (HTTP)", cfg.Addr)
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("server stopped")
}
