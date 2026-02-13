package tk

import (
	"os"
	"strings"
)

// IsDevelopment checks if the application is running in a development environment based on the "DEVELOPMENT" environment variable.
func IsDevelopment() bool {
	development := os.Getenv("DEVELOPMENT")
	return strings.ToLower(development) == "true"
}
