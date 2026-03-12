package cmd

import (
	"os"

	"github.com/hynor/nshellserver/internal/config"
	"github.com/hynor/nshellserver/internal/logging"
	"github.com/spf13/cobra"
)

func newServeCmd(runServe runServerFunc, getenv func(string) string) *cobra.Command {
	cfg := config.DefaultConfig(getenv)

	serveCmd := &cobra.Command{
		Use:           "serve",
		Short:         "Start the sync server",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			config.ApplyTLSEnv(cfg, getenv)
			_, resolvedLevel, fallback := logging.ResolveLevel(cfg.LogLevel)
			logger := logging.NewLogger(os.Stderr, resolvedLevel)
			if fallback {
				logger.Warn("invalid log level, falling back to info", "provided", cfg.LogLevel, "effective", resolvedLevel)
			}
			cfg.LogLevel = resolvedLevel
			return runServe(cfg, logger)
		},
	}

	flags := serveCmd.Flags()
	flags.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	flags.StringVar(&cfg.CertFile, "cert", "", "TLS certificate file")
	flags.StringVar(&cfg.KeyFile, "key", "", "TLS private key file")
	flags.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	flags.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: error, warning, info, debug")
	flags.BoolVar(&cfg.Dev, "dev", false, "generate and use a local self-signed TLS certificate pair in the current directory")
	flags.BoolVar(&cfg.SelfHost, "self-host", false, "listen over plain HTTP for local reverse-proxy deployment")
	flags.BoolVar(&cfg.Public, "public", false, "with --self-host, listen on 0.0.0.0 instead of 127.0.0.1")

	return serveCmd
}
