package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var configDir string

var rootCmd = &cobra.Command{
	Use:   "barnacle",
	Short: "Barnacle server",
	Long:  "Barnacle is a server application",
}

func init() {
	// Set default config directory
	defaultConfigDir := filepath.Join(getXDGConfigHome(), "barnacle")

	rootCmd.PersistentFlags().StringVar(&configDir, "configDir", defaultConfigDir, "configuration directory")
}

// getXDGConfigHome returns the XDG_CONFIG_HOME directory or a sensible default.
func getXDGConfigHome() string {
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return xdgConfigHome
	}

	// Default to ~/.config if XDG_CONFIG_HOME is not set
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(homeDir, ".config")
}
