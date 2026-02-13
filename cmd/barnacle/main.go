package main

import (
	"os"

	"github.com/joho/godotenv"
)

// Build-time variables injected via ldflags by goreleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

//go:generate swag init -g main.go --parseInternal -d ./cmd/barnacle/,./internal/routes/,./pkg/api/,./internal/tk/httptk/ -o docs

// @title           Barnacle API
// @version         1.0
// @description     Pull-through caching proxy for OCI container registries.
// @description
// @description     Barnacle provides a distributed caching layer in front of upstream
// @description     container registries, with multi-node coordination via Redis.

// @contact.name    Barnacle Support
// @contact.url     https://github.com/pdylanross/barnacle/issues

// @license.name    MIT
// @license.url     https://opensource.org/licenses/MIT

// @host            localhost:8080
// @BasePath        /

// @accept          json
// @produce         json

// @tag.name        system
// @tag.description Health and system status
// @tag.name        nodes
// @tag.description Node lifecycle and health management
// @tag.name        upstreams
// @tag.description Upstream container registry configuration
// @tag.name        blobs
// @tag.description Blob cache inspection and management

func main() {
	// Load .env file if it exists (ignore error if file doesn't exist)
	_ = godotenv.Load()

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
