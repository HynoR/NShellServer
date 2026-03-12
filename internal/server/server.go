package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hynor/nshellserver/internal/config"
	"github.com/hynor/nshellserver/internal/db"
	"github.com/hynor/nshellserver/internal/handler"
	"github.com/hynor/nshellserver/internal/logging"
)

const (
	devKeyFile  = "nshell-key.pem"
	devCertFile = "nshell-crt.pem"
)

type commandRunner func(string, ...string) error
type serverMode struct {
	useTLS  bool
	warning string
}

func Run(cfg *config.Config, logger *slog.Logger) error {
	if logger == nil {
		logger = logging.NewLogger(io.Discard, "info")
	}

	mode, err := prepareServerMode(cfg, logger)
	if err != nil {
		return err
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	store := db.NewStore(database)
	h := handler.New(store, logger)

	r := chi.NewRouter()
	r.Use(middleware.Compress(5))
	r.Use(h.RateLimiter.Middleware)
	r.Use(handler.BodyLimitMiddleware)

	r.Route("/api/v1/sync", func(r chi.Router) {
		r.Use(h.AuthMiddleware)
		r.Use(h.RequestLogger)
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
			return fmt.Errorf("failed to load TLS cert/key: %w", err)
		}
		srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		if mode.warning != "" {
			logger.Warn(mode.warning, "addr", cfg.Addr)
		}

		var err error
		transport := "http"
		if mode.useTLS {
			transport = "tls"
			logger.Info("server starting", "addr", cfg.Addr, "transport", transport, "self_host", cfg.SelfHost, "public", cfg.Public, "log_level", cfg.LogLevel)
			err = srv.ListenAndServeTLS("", "")
		} else {
			logger.Info("server starting", "addr", cfg.Addr, "transport", transport, "self_host", cfg.SelfHost, "public", cfg.Public, "log_level", cfg.LogLevel)
			err = srv.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			logger.Error("server exited unexpectedly", "addr", cfg.Addr, "error", err)
			errCh <- fmt.Errorf("server error: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("server shutting down", "addr", cfg.Addr)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "addr", cfg.Addr, "error", err)
			return fmt.Errorf("shutdown error: %w", err)
		}
		logger.Info("server stopped", "addr", cfg.Addr)
		return nil
	}
}

func prepareServerMode(cfg *config.Config, logger *slog.Logger) (serverMode, error) {
	if cfg.Public && !cfg.SelfHost {
		return serverMode{}, errors.New("--public requires --self-host")
	}

	if cfg.SelfHost {
		if cfg.Dev || cfg.CertFile != "" || cfg.KeyFile != "" {
			return serverMode{}, errors.New("--self-host cannot be used with --cert, --key, or --dev")
		}

		listenHost := "127.0.0.1"
		warning := ""
		if cfg.Public {
			listenHost = "0.0.0.0"
			warning = "warning: insecure HTTP listener exposed on 0.0.0.0; terminate TLS in your reverse proxy"
		}

		addr, err := forceListenHost(cfg.Addr, listenHost)
		if err != nil {
			return serverMode{}, err
		}
		cfg.Addr = addr

		return serverMode{warning: warning}, nil
	}

	if err := prepareTLSFiles(cfg, logger); err != nil {
		return serverMode{}, err
	}

	return serverMode{useTLS: true}, nil
}

func prepareTLSFiles(cfg *config.Config, logger *slog.Logger) error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	return prepareTLSFilesAt(cfg, workingDir, func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Dir = workingDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}, logger)
}

func prepareTLSFilesAt(cfg *config.Config, workingDir string, run commandRunner, logger *slog.Logger) error {
	if cfg.Dev {
		if cfg.CertFile != "" || cfg.KeyFile != "" {
			return errors.New("--dev cannot be used with --cert or --key")
		}

		keyExists, err := fileExists(filepath.Join(workingDir, devKeyFile))
		if err != nil {
			return err
		}
		certExists, err := fileExists(filepath.Join(workingDir, devCertFile))
		if err != nil {
			return err
		}

		switch {
		case keyExists && certExists:
			logger.Debug("reusing existing dev tls certificate pair", "key_file", devKeyFile, "cert_file", devCertFile)
		case !keyExists && !certExists:
			logger.Debug("generating dev tls certificate pair", "key_file", devKeyFile, "cert_file", devCertFile)
			if err := run(
				"openssl",
				"req",
				"-x509",
				"-newkey",
				"rsa:2048",
				"-keyout",
				devKeyFile,
				"-out",
				devCertFile,
				"-days",
				"365",
				"-nodes",
				"-subj",
				"/CN=localhost",
			); err != nil {
				return fmt.Errorf("generate dev TLS cert/key: %w", err)
			}
		default:
			return fmt.Errorf("both %s and %s must exist together", devKeyFile, devCertFile)
		}

		cfg.KeyFile = devKeyFile
		cfg.CertFile = devCertFile
		return nil
	}

	if cfg.CertFile == "" || cfg.KeyFile == "" {
		return errors.New("--cert and --key are required")
	}

	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %s: %w", path, err)
}

func forceListenHost(addr string, host string) (string, error) {
	if strings.HasPrefix(addr, ":") {
		return host + addr, nil
	}
	if !strings.Contains(addr, ":") {
		return net.JoinHostPort(host, addr), nil
	}

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid listen address %q: %w", addr, err)
	}
	return net.JoinHostPort(host, port), nil
}
