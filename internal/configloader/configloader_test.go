package configloader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pdylanross/barnacle/internal/configloader"
	"github.com/pdylanross/barnacle/pkg/configuration"
	testutils "github.com/pdylanross/barnacle/test"
)

func TestLoadConfig_SingleFile(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Write single YAML file
	configContent := `server:
  port: 8080
`
	err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	// Load config
	logger := testutils.CreateTestLogger(t)
	var config *configuration.Configuration
	config, err = configloader.LoadConfig(tmpDir, logger)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify
	if config.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", config.Server.Port)
	}
}

func TestLoadConfig_MultipleFiles(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Write first YAML file
	config1 := `server:
  port: 8080
`
	err := os.WriteFile(filepath.Join(tmpDir, "config1.yaml"), []byte(config1), 0644)
	if err != nil {
		t.Fatalf("failed to write first config file: %v", err)
	}

	// Write second YAML file (should override)
	config2 := `server:
  port: 9090
`
	err = os.WriteFile(filepath.Join(tmpDir, "config2.yaml"), []byte(config2), 0644)
	if err != nil {
		t.Fatalf("failed to write second config file: %v", err)
	}

	// Load config
	logger := testutils.CreateTestLogger(t)
	config, err := configloader.LoadConfig(tmpDir, logger)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify - later file should override
	if config.Server.Port != 9090 {
		t.Errorf("expected port 9090 from second file, got %d", config.Server.Port)
	}
}

func TestLoadConfig_EnvVarOverride(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Write YAML file
	configContent := `server:
  port: 8080
`
	err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	// Set environment variable
	t.Setenv("BARNACLE_SERVER_PORT", "7070")

	// Load config
	logger := testutils.CreateTestLogger(t)
	config, err := configloader.LoadConfig(tmpDir, logger)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify - environment variable should override
	if config.Server.Port != 7070 {
		t.Errorf("expected port 7070 from env var, got %d", config.Server.Port)
	}
}

func TestLoadConfig_EnvSubstitution(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Set environment variable for substitution
	t.Setenv("TEST_PORT", "6060")

	// Write YAML file with env var substitution
	configContent := `server:
  port: ${TEST_PORT}
`
	err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	// Load config
	logger := testutils.CreateTestLogger(t)
	config, err := configloader.LoadConfig(tmpDir, logger)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify - environment variable should be substituted
	if config.Server.Port != 6060 {
		t.Errorf("expected port 6060 from env substitution, got %d", config.Server.Port)
	}
}

func TestLoadConfig_EmptyDirectory(t *testing.T) {
	// Create empty temporary directory
	tmpDir := t.TempDir()

	// Load config from empty directory
	logger := testutils.CreateTestLogger(t)
	config, err := configloader.LoadConfig(tmpDir, logger)
	if err != nil {
		t.Fatalf("LoadConfig should not fail on empty directory: %v", err)
	}

	// Verify - should return default values
	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if config.Server.Port != configuration.DefaultServerPort {
		t.Errorf("expected default port %d, got %d", configuration.DefaultServerPort, config.Server.Port)
	}
}

func TestLoadConfig_MissingDirectory(t *testing.T) {
	// Use a path that doesn't exist
	nonExistentPath := filepath.Join(t.TempDir(), "does-not-exist")

	// Load config from missing directory
	logger := testutils.CreateTestLogger(t)
	config, err := configloader.LoadConfig(nonExistentPath, logger)
	if err != nil {
		t.Fatalf("LoadConfig should not fail on missing directory: %v", err)
	}

	// Verify - should return default values
	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if config.Server.Port != configuration.DefaultServerPort {
		t.Errorf("expected default port %d, got %d", configuration.DefaultServerPort, config.Server.Port)
	}
}

func TestLoadConfig_YmlExtension(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Write YAML file with .yml extension
	configContent := `server:
  port: 5050
`
	err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	// Load config
	logger := testutils.CreateTestLogger(t)
	config, err := configloader.LoadConfig(tmpDir, logger)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify
	if config.Server.Port != 5050 {
		t.Errorf("expected port 5050, got %d", config.Server.Port)
	}
}

func TestLoadConfig_IgnoresNonYAMLFiles(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Write YAML file
	configContent := `server:
  port: 4040
`
	err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	// Write non-YAML file that would cause errors if parsed
	err = os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("not yaml"), 0644)
	if err != nil {
		t.Fatalf("failed to write test text file: %v", err)
	}

	// Load config
	logger := testutils.CreateTestLogger(t)
	config, err := configloader.LoadConfig(tmpDir, logger)
	if err != nil {
		t.Fatalf("LoadConfig should ignore non-YAML files: %v", err)
	}

	// Verify
	if config.Server.Port != 4040 {
		t.Errorf("expected port 4040, got %d", config.Server.Port)
	}
}

func TestLoadConfig_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		yamlContent  string
		envVars      map[string]string
		expectedPort int
		shouldError  bool
	}{
		{
			name: "valid config",
			yamlContent: `server:
  port: 3000
`,
			expectedPort: 3000,
			shouldError:  false,
		},
		{
			name:         "empty yaml uses defaults",
			yamlContent:  "",
			expectedPort: configuration.DefaultServerPort,
			shouldError:  false,
		},
		{
			name: "env override",
			yamlContent: `server:
  port: 3000
`,
			envVars: map[string]string{
				"BARNACLE_SERVER_PORT": "2000",
			},
			expectedPort: 2000,
			shouldError:  false,
		},
		{
			name: "invalid yaml",
			yamlContent: `server:
  port: not a number
`,
			shouldError: true,
		},
		{
			name: "invalid port too high",
			yamlContent: `server:
  port: 70000
`,
			shouldError: true,
		},
		{
			name: "invalid negative port",
			yamlContent: `server:
  port: -5
`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir := t.TempDir()

			// Write YAML file if content provided
			if tt.yamlContent != "" {
				err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(tt.yamlContent), 0644)
				if err != nil {
					t.Fatalf("failed to write test config file: %v", err)
				}
			}

			// Set environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Load config
			logger := testutils.CreateTestLogger(t)
			config, err := configloader.LoadConfig(tmpDir, logger)

			// Check error expectation
			if tt.shouldError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify port
			if config.Server.Port != tt.expectedPort {
				t.Errorf("expected port %d, got %d", tt.expectedPort, config.Server.Port)
			}
		})
	}
}
