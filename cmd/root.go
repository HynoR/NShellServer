package cmd

import (
	"errors"
	"log/slog"
	"os"

	"github.com/hynor/nshellserver/internal/config"
	"github.com/hynor/nshellserver/internal/server"
	"github.com/spf13/cobra"
)

var errSubcommandRequired = errors.New("requires a subcommand")

type runServerFunc func(*config.Config, *slog.Logger) error

func Execute() error {
	return NewRootCmd(server.Run, os.Getenv).Execute()
}

func NewRootCmd(runServe runServerFunc, getenv func(string) string) *cobra.Command {
	if runServe == nil {
		runServe = server.Run
	}
	if getenv == nil {
		getenv = os.Getenv
	}

	rootCmd := &cobra.Command{
		Use:           "nshellserver",
		Short:         "NextShell Cloud Sync Server",
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Help(); err != nil {
				return err
			}
			return errSubcommandRequired
		},
	}

	rootCmd.AddCommand(newServeCmd(runServe, getenv))
	return rootCmd
}
