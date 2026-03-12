package config

import (
	"flag"
	"fmt"
	"io"
	"os"
)

type Config struct {
	Addr     string
	CertFile string
	KeyFile  string
	DBPath   string
	Dev      bool
	SelfHost bool
	Public   bool
}

func Load() *Config {
	c, err := parseArgs(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return c
}

func parseArgs(args []string, getenv func(string) string) (*Config, error) {
	c := &Config{}
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.Addr, "addr", envOrDefault(getenv, "NSHELL_ADDR", ":8443"), "listen address")
	fs.StringVar(&c.CertFile, "cert", "", "TLS certificate file")
	fs.StringVar(&c.KeyFile, "key", "", "TLS private key file")
	fs.StringVar(&c.DBPath, "db", envOrDefault(getenv, "NSHELL_DB", "./nshell.db"), "SQLite database path")
	fs.BoolVar(&c.Dev, "dev", false, "generate and use a local self-signed TLS certificate pair in the current directory")
	fs.BoolVar(&c.SelfHost, "self-host", false, "listen over plain HTTP for local reverse-proxy deployment")
	fs.BoolVar(&c.Public, "public", false, "with --self-host, listen on 0.0.0.0 instead of 127.0.0.1")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if !c.Dev && !c.SelfHost {
		if c.CertFile == "" {
			c.CertFile = envOrDefault(getenv, "NSHELL_CERT", "")
		}
		if c.KeyFile == "" {
			c.KeyFile = envOrDefault(getenv, "NSHELL_KEY", "")
		}
	}
	return c, nil
}

func envOrDefault(getenv func(string) string, key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}
