package main

import (
	"fmt"
	"os"

	"github.com/pdylanross/barnacle/internal/configloader"
	"github.com/pdylanross/barnacle/internal/logsetup"
	"github.com/pdylanross/barnacle/internal/server"
	"github.com/pdylanross/barnacle/internal/tk"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the barnacle server",
	Long:  "Start the barnacle server to handle requests",
	Run: func(cmd *cobra.Command, _ []string) {
		// Initialize logger
		logger, err := logsetup.InitializeLogger()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		defer tk.IgnoreDeferError(logger.Sync)

		logger.Info("Starting barnacle server")

		// Load configuration
		config, err := configloader.LoadConfig(configDir, logger)
		if err != nil {
			logger.Error("Failed to load configuration", zap.Error(err))
			os.Exit(1)
		}

		logger.Info("Configuration loaded successfully",
			zap.String("configDir", configDir),
		)

		// Create server
		srv, err := server.NewServer(cmd.Context(), config, logger)
		if err != nil {
			logger.Error("Failed to create server", zap.Error(err))
			os.Exit(1)
		}
		defer tk.IgnoreDeferError(srv.Close)

		// Run server
		if err = srv.Run(); err != nil {
			logger.Error("Server failed", zap.Error(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
