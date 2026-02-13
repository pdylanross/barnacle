package configloader_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdylanross/barnacle/internal/configloader"
	testutils "github.com/pdylanross/barnacle/test"
)

// TestErrorsIs demonstrates the benefit of using global error variables.
// With global error variables, callers can use [errors.Is] to check for specific error conditions.
func TestErrorsIs(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(string) error
		expectedError error
	}{
		{
			name: "ErrReadConfigDirectory when directory read fails",
			setupFunc: func(tmpDir string) error {
				// Create a file instead of directory to cause ReadDir to fail
				invalidPath := filepath.Join(tmpDir, "not-a-dir")
				if err := os.WriteFile(invalidPath, []byte("test"), 0644); err != nil {
					return err
				}

				logger := testutils.CreateTestLogger(t)
				_, err := configloader.LoadConfig(invalidPath, logger)
				return err
			},
			expectedError: configloader.ErrReadConfigDirectory,
		},
		{
			name: "ErrParseYAMLFile when YAML is invalid",
			setupFunc: func(tmpDir string) error {
				// Write invalid YAML
				invalidYAML := `invalid: yaml: content: [unclosed bracket`
				if err := os.WriteFile(filepath.Join(tmpDir, "bad.yaml"), []byte(invalidYAML), 0644); err != nil {
					return err
				}

				logger := testutils.CreateTestLogger(t)
				_, err := configloader.LoadConfig(tmpDir, logger)
				return err
			},
			expectedError: configloader.ErrParseYAMLFile,
		},
		{
			name: "ErrValidateConfiguration when port is out of range",
			setupFunc: func(tmpDir string) error {
				// Write config with invalid port
				invalidConfig := `server:
  port: 99999
`
				if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(invalidConfig), 0644); err != nil {
					return err
				}

				logger := testutils.CreateTestLogger(t)
				_, err := configloader.LoadConfig(tmpDir, logger)
				return err
			},
			expectedError: configloader.ErrValidateConfiguration,
		},
		{
			name: "ErrValidateConfiguration when port is negative",
			setupFunc: func(tmpDir string) error {
				// Write config with negative port
				invalidConfig := `server:
  port: -1
`
				if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(invalidConfig), 0644); err != nil {
					return err
				}

				logger := testutils.CreateTestLogger(t)
				_, err := configloader.LoadConfig(tmpDir, logger)
				return err
			},
			expectedError: configloader.ErrValidateConfiguration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			err := tt.setupFunc(tmpDir)
			if err == nil {
				t.Fatal("expected error but got nil")
			}

			// This is the key benefit: we can use errors.Is() to check for specific error types
			if !errors.Is(err, tt.expectedError) {
				t.Errorf("expected error to be %v, but errors.Is returned false. Got: %v", tt.expectedError, err)
			}
		})
	}
}
