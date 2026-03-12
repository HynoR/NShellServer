package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hynor/nshellserver/internal/config"
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

func prepareServerMode(cfg *config.Config) (serverMode, error) {
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

	if err := prepareTLSFiles(cfg); err != nil {
		return serverMode{}, err
	}

	return serverMode{useTLS: true}, nil
}

func prepareTLSFiles(cfg *config.Config) error {
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
	})
}

func prepareTLSFilesAt(cfg *config.Config, workingDir string, run commandRunner) error {
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
		case !keyExists && !certExists:
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
