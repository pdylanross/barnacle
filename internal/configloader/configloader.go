package configloader

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drone/envsubst"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"go.uber.org/zap"

	"github.com/pdylanross/barnacle/pkg/configuration"
)

// Errors that can be returned from configloader functions.
// These global error variables allow callers to use [errors.Is] and [errors.As]
// to check for specific error conditions.
var (
	ErrLoadYAMLFiles            = errors.New("failed to load YAML files")
	ErrLoadEnvironmentVariables = errors.New("failed to load environment variables")
	ErrUnmarshalConfiguration   = errors.New("failed to unmarshal configuration")
	ErrValidateConfiguration    = errors.New("failed to validate configuration")
	ErrReadConfigDirectory      = errors.New("failed to read config directory")
	ErrReadFile                 = errors.New("failed to read file")
	ErrSubstituteEnvVars        = errors.New("failed to substitute environment variables")
	ErrParseYAMLFile            = errors.New("failed to parse YAML file")
)

// LoadConfig loads configuration from YAML files in the specified directory
// and environment variables prefixed with BARNACLE_.
func LoadConfig(configDirectory string, logger *zap.Logger) (*configuration.Configuration, error) {
	log := logger.Named("configloader")
	log.Debug("Starting configuration loading", zap.String("configDirectory", configDirectory))

	k := koanf.New(".")

	// Load all YAML files from the config directory
	log.Debug("Loading YAML files from config directory")
	if err := loadYAMLFiles(k, configDirectory, log); err != nil {
		log.Error("Failed to load YAML files", zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrLoadYAMLFiles, err)
	}

	// Load environment variables with BARNACLE_ prefix
	// Convert BARNACLE_SERVER_PORT to server.port
	log.Debug("Loading environment variables with BARNACLE_ prefix")
	if err := k.Load(env.Provider("BARNACLE_", ".", func(s string) string {
		return strings.ReplaceAll(strings.ToLower(
			strings.TrimPrefix(s, "BARNACLE_")), "_", ".")
	}), nil); err != nil {
		log.Error("Failed to load environment variables", zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrLoadEnvironmentVariables, err)
	}

	// Unmarshal into Configuration struct, starting with defaults
	log.Debug("Unmarshaling configuration into struct")
	config := configuration.Default()
	if err := k.Unmarshal("", config); err != nil {
		log.Error("Failed to unmarshal configuration", zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrUnmarshalConfiguration, err)
	}

	// Validate the configuration
	log.Debug("Validating configuration")
	if err := config.Validate(log); err != nil {
		log.Error("Configuration validation failed", zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrValidateConfiguration, err)
	}

	log.Debug("Configuration loaded and validated successfully")
	return config, nil
}

// loadYAMLFiles loads all YAML files from the specified directory.
func loadYAMLFiles(k *koanf.Koanf, configDirectory string, log *zap.Logger) error {
	// Check if directory exists
	if _, err := os.Stat(configDirectory); os.IsNotExist(err) {
		log.Debug("Config directory does not exist, using defaults", zap.String("directory", configDirectory))
		return nil
	}

	// Read all files in the directory
	log.Debug("Reading config directory", zap.String("directory", configDirectory))
	entries, err := os.ReadDir(configDirectory)
	if err != nil {
		log.Error("Failed to read config directory", zap.Error(err))
		return fmt.Errorf("%w: %w", ErrReadConfigDirectory, err)
	}

	// Load each YAML file
	yamlFileCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			log.Debug("Skipping directory entry", zap.String("name", entry.Name()))
			continue
		}

		// Only process .yaml and .yml files
		filename := entry.Name()
		ext := filepath.Ext(filename)
		if ext != ".yaml" && ext != ".yml" {
			log.Debug("Skipping non-YAML file", zap.String("filename", filename))
			continue
		}

		// Read file content
		filePath := filepath.Join(configDirectory, filename)
		log.Debug("Loading YAML file", zap.String("file", filePath))
		var content []byte
		content, err = os.ReadFile(filePath)
		if err != nil {
			log.Error("Failed to read file", zap.String("file", filename), zap.Error(err))
			return fmt.Errorf("%w %s: %w", ErrReadFile, filename, err)
		}

		// Run through envsubst
		log.Debug("Substituting environment variables", zap.String("file", filename))
		var substituted string
		substituted, err = envsubst.EvalEnv(string(content))
		if err != nil {
			log.Error("Failed to substitute environment variables", zap.String("file", filename), zap.Error(err))
			return fmt.Errorf("%w in %s: %w", ErrSubstituteEnvVars, filename, err)
		}

		// Load into koanf
		log.Debug("Parsing YAML file", zap.String("file", filename))
		err = k.Load(rawbytes.Provider([]byte(substituted)), yaml.Parser())
		if err != nil {
			log.Error("Failed to parse YAML file", zap.String("file", filename), zap.Error(err))
			return fmt.Errorf("%w %s: %w", ErrParseYAMLFile, filename, err)
		}

		yamlFileCount++
		log.Debug("YAML file loaded successfully", zap.String("file", filename))
	}

	log.Debug("Finished loading YAML files", zap.Int("filesLoaded", yamlFileCount))
	return nil
}
