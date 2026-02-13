package configuration_test

import (
	"errors"
	"testing"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"

	"github.com/pdylanross/barnacle/pkg/configuration"
)

func TestConfiguration_EmptyYAML(t *testing.T) {
	k := koanf.New(".")

	// Load empty YAML
	err := k.Load(rawbytes.Provider([]byte("")), yaml.Parser())
	if err != nil {
		t.Fatalf("failed to load empty YAML: %v", err)
	}

	// Unmarshal into Configuration
	var config configuration.Configuration
	err = k.Unmarshal("", &config)
	if err != nil {
		t.Fatalf("failed to unmarshal empty YAML: %v", err)
	}

	// Verify default/zero values
	if config.Server.Port != 0 {
		t.Errorf("expected default port 0, got %d", config.Server.Port)
	}
}

func TestConfiguration_PartialServer(t *testing.T) {
	k := koanf.New(".")

	// Load YAML with only server section but no port
	yamlContent := `server: {}`
	err := k.Load(rawbytes.Provider([]byte(yamlContent)), yaml.Parser())
	if err != nil {
		t.Fatalf("failed to load YAML: %v", err)
	}

	// Unmarshal into Configuration
	var config configuration.Configuration
	err = k.Unmarshal("", &config)
	if err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	// Verify default port value
	if config.Server.Port != 0 {
		t.Errorf("expected default port 0, got %d", config.Server.Port)
	}
}

func TestConfiguration_MissingServerSection(t *testing.T) {
	k := koanf.New(".")

	// Load YAML without server section
	yamlContent := `othersection:
  value: 123
`
	err := k.Load(rawbytes.Provider([]byte(yamlContent)), yaml.Parser())
	if err != nil {
		t.Fatalf("failed to load YAML: %v", err)
	}

	// Unmarshal into Configuration
	var config configuration.Configuration
	err = k.Unmarshal("", &config)
	if err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	// Verify default values
	if config.Server.Port != 0 {
		t.Errorf("expected default port 0, got %d", config.Server.Port)
	}
}

func TestConfiguration_CompleteConfiguration(t *testing.T) {
	k := koanf.New(".")

	// Load complete YAML
	yamlContent := `server:
  port: 8080
`
	err := k.Load(rawbytes.Provider([]byte(yamlContent)), yaml.Parser())
	if err != nil {
		t.Fatalf("failed to load YAML: %v", err)
	}

	// Unmarshal into Configuration
	var config configuration.Configuration
	err = k.Unmarshal("", &config)
	if err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	// Verify values
	if config.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", config.Server.Port)
	}
}

func TestConfiguration_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		yamlContent  string
		expectedPort int
		shouldError  bool
	}{
		{
			name:         "empty yaml",
			yamlContent:  "",
			expectedPort: 0,
			shouldError:  false,
		},
		{
			name:         "empty object",
			yamlContent:  "{}",
			expectedPort: 0,
			shouldError:  false,
		},
		{
			name:         "server section only",
			yamlContent:  `server: {}`,
			expectedPort: 0,
			shouldError:  false,
		},
		{
			name: "complete config",
			yamlContent: `server:
  port: 9090
`,
			expectedPort: 9090,
			shouldError:  false,
		},
		{
			name: "port zero explicit",
			yamlContent: `server:
  port: 0
`,
			expectedPort: 0,
			shouldError:  false,
		},
		{
			name: "port negative (valid int)",
			yamlContent: `server:
  port: -1
`,
			expectedPort: -1,
			shouldError:  false,
		},
		{
			name: "partial with other fields",
			yamlContent: `server:
  port: 5000
other:
  value: test
`,
			expectedPort: 5000,
			shouldError:  false,
		},
		{
			name: "invalid port type",
			yamlContent: `server:
  port: "not a number"
`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := koanf.New(".")

			// Load YAML
			err := k.Load(rawbytes.Provider([]byte(tt.yamlContent)), yaml.Parser())
			if err != nil && !tt.shouldError {
				t.Fatalf("failed to load YAML: %v", err)
			}

			// Unmarshal into Configuration
			var config configuration.Configuration
			err = k.Unmarshal("", &config)

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

func TestServerConfiguration_Standalone(t *testing.T) {
	k := koanf.New(".")

	// Load YAML directly into ServerConfiguration
	yamlContent := `port: 7070`
	err := k.Load(rawbytes.Provider([]byte(yamlContent)), yaml.Parser())
	if err != nil {
		t.Fatalf("failed to load YAML: %v", err)
	}

	// Unmarshal into ServerConfiguration directly
	var serverConfig configuration.ServerConfiguration
	err = k.Unmarshal("", &serverConfig)
	if err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	// Verify
	if serverConfig.Port != 7070 {
		t.Errorf("expected port 7070, got %d", serverConfig.Port)
	}
}

func TestServerConfiguration_EmptyYAML(t *testing.T) {
	k := koanf.New(".")

	// Load empty YAML
	err := k.Load(rawbytes.Provider([]byte("")), yaml.Parser())
	if err != nil {
		t.Fatalf("failed to load empty YAML: %v", err)
	}

	// Unmarshal into ServerConfiguration
	var serverConfig configuration.ServerConfiguration
	err = k.Unmarshal("", &serverConfig)
	if err != nil {
		t.Fatalf("failed to unmarshal empty YAML: %v", err)
	}

	// Verify default value
	if serverConfig.Port != 0 {
		t.Errorf("expected default port 0, got %d", serverConfig.Port)
	}
}

func TestConfiguration_Validate(t *testing.T) {
	validRedis := configuration.RedisConfiguration{Addr: "localhost:6379"}
	validCache := configuration.CacheConfiguration{
		Disk: configuration.DiskCacheConfiguration{
			Tiers: []configuration.DiskTierConfiguration{
				{Tier: 0, Path: "/var/cache/test"},
			},
		},
	}

	tests := []struct {
		name    string
		config  configuration.Configuration
		wantErr bool
		errType error
	}{
		{
			name: "valid configuration",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: 8080,
				},
				Redis: validRedis,
				Cache: validCache,
			},
			wantErr: false,
		},
		{
			name: "valid port 0",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: 0,
				},
				Redis: validRedis,
				Cache: validCache,
			},
			wantErr: false,
		},
		{
			name: "valid max port",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: 65535,
				},
				Redis: validRedis,
				Cache: validCache,
			},
			wantErr: false,
		},
		{
			name: "invalid negative port",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: -1,
				},
				Redis: validRedis,
				Cache: validCache,
			},
			wantErr: true,
			errType: configuration.ErrInvalidPort,
		},
		{
			name: "invalid port too high",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: 65536,
				},
				Redis: validRedis,
				Cache: validCache,
			},
			wantErr: true,
			errType: configuration.ErrInvalidPort,
		},
		{
			name: "invalid redis - empty addr",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: 8080,
				},
				Redis: configuration.RedisConfiguration{Addr: ""},
				Cache: validCache,
			},
			wantErr: true,
			errType: configuration.ErrInvalidConfiguration,
		},
		{
			name: "invalid redis - negative db",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: 8080,
				},
				Redis: configuration.RedisConfiguration{Addr: "localhost:6379", DB: -1},
				Cache: validCache,
			},
			wantErr: true,
			errType: configuration.ErrInvalidConfiguration,
		},
		{
			name: "valid configuration with upstreams",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: 8080,
				},
				Redis: validRedis,
				Cache: validCache,
				Upstreams: map[string]configuration.UpstreamConfiguration{
					"docker.io": {
						Registry: "https://registry-1.docker.io",
						Authentication: configuration.UpstreamAuthentication{
							Anonymous: &configuration.UpstreamAnonymousAuthentication{},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid configuration with multiple upstreams",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: 8080,
				},
				Redis: validRedis,
				Cache: validCache,
				Upstreams: map[string]configuration.UpstreamConfiguration{
					"docker.io": {
						Registry: "https://registry-1.docker.io",
						Authentication: configuration.UpstreamAuthentication{
							Anonymous: &configuration.UpstreamAnonymousAuthentication{},
						},
					},
					"gcr.io": {
						Registry: "https://gcr.io",
						Authentication: configuration.UpstreamAuthentication{
							Basic: &configuration.UpstreamBasicAuthentication{
								Username: "user",
								Password: "pass",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid empty upstream alias",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: 8080,
				},
				Redis: validRedis,
				Cache: validCache,
				Upstreams: map[string]configuration.UpstreamConfiguration{
					"": {
						Registry: "https://registry-1.docker.io",
						Authentication: configuration.UpstreamAuthentication{
							Anonymous: &configuration.UpstreamAnonymousAuthentication{},
						},
					},
				},
			},
			wantErr: true,
			errType: configuration.ErrInvalidConfiguration,
		},
		{
			name: "invalid upstream configuration - empty registry",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: 8080,
				},
				Redis: validRedis,
				Cache: validCache,
				Upstreams: map[string]configuration.UpstreamConfiguration{
					"docker.io": {
						Registry: "",
						Authentication: configuration.UpstreamAuthentication{
							Anonymous: &configuration.UpstreamAnonymousAuthentication{},
						},
					},
				},
			},
			wantErr: true,
			errType: configuration.ErrInvalidAuthConfiguration,
		},
		{
			name: "invalid upstream authentication",
			config: configuration.Configuration{
				Server: configuration.ServerConfiguration{
					Port: 8080,
				},
				Redis: validRedis,
				Cache: validCache,
				Upstreams: map[string]configuration.UpstreamConfiguration{
					"docker.io": {
						Registry: "https://registry-1.docker.io",
						Authentication: configuration.UpstreamAuthentication{
							Basic: &configuration.UpstreamBasicAuthentication{
								Username: "",
								Password: "pass",
							},
						},
					},
				},
			},
			wantErr: true,
			errType: configuration.ErrInvalidAuthConfiguration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if tt.wantErr && tt.errType != nil {
				if !errors.Is(err, tt.errType) {
					t.Errorf("expected error type %v, got %v", tt.errType, err)
				}
			}
		})
	}
}

func TestServerConfiguration_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  configuration.ServerConfiguration
		wantErr bool
		errType error
	}{
		{
			name:    "valid port 8080",
			config:  configuration.ServerConfiguration{Port: 8080},
			wantErr: false,
		},
		{
			name:    "valid port 0",
			config:  configuration.ServerConfiguration{Port: 0},
			wantErr: false,
		},
		{
			name:    "valid port 80",
			config:  configuration.ServerConfiguration{Port: 80},
			wantErr: false,
		},
		{
			name:    "valid port 443",
			config:  configuration.ServerConfiguration{Port: 443},
			wantErr: false,
		},
		{
			name:    "valid max port 65535",
			config:  configuration.ServerConfiguration{Port: 65535},
			wantErr: false,
		},
		{
			name:    "invalid negative port",
			config:  configuration.ServerConfiguration{Port: -1},
			wantErr: true,
			errType: configuration.ErrInvalidPort,
		},
		{
			name:    "invalid large negative port",
			config:  configuration.ServerConfiguration{Port: -1000},
			wantErr: true,
			errType: configuration.ErrInvalidPort,
		},
		{
			name:    "invalid port too high",
			config:  configuration.ServerConfiguration{Port: 65536},
			wantErr: true,
			errType: configuration.ErrInvalidPort,
		},
		{
			name:    "invalid port way too high",
			config:  configuration.ServerConfiguration{Port: 100000},
			wantErr: true,
			errType: configuration.ErrInvalidPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if tt.wantErr && tt.errType != nil {
				if !errors.Is(err, tt.errType) {
					t.Errorf("expected error type %v, got %v", tt.errType, err)
				}
			}
		})
	}
}

func TestServerConfiguration_ListenAddr(t *testing.T) {
	tests := []struct {
		name     string
		config   configuration.ServerConfiguration
		expected string
	}{
		{
			name:     "port 8080",
			config:   configuration.ServerConfiguration{Port: 8080},
			expected: ":8080",
		},
		{
			name:     "port 0",
			config:   configuration.ServerConfiguration{Port: 0},
			expected: ":0",
		},
		{
			name:     "port 443",
			config:   configuration.ServerConfiguration{Port: 443},
			expected: ":443",
		},
		{
			name:     "port 65535",
			config:   configuration.ServerConfiguration{Port: 65535},
			expected: ":65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := tt.config.ListenAddr()
			if addr != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, addr)
			}
		})
	}
}

func TestServerConfiguration_BuildHTTP(t *testing.T) {
	tests := []struct {
		name         string
		config       configuration.ServerConfiguration
		expectedAddr string
	}{
		{
			name:         "port 8080",
			config:       configuration.ServerConfiguration{Port: 8080},
			expectedAddr: ":8080",
		},
		{
			name:         "port 0",
			config:       configuration.ServerConfiguration{Port: 0},
			expectedAddr: ":0",
		},
		{
			name:         "port 443",
			config:       configuration.ServerConfiguration{Port: 443},
			expectedAddr: ":443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.config.BuildHTTP()

			if server == nil {
				t.Fatal("expected server to be non-nil")
			}

			if server.Addr != tt.expectedAddr {
				t.Errorf("expected addr %q, got %q", tt.expectedAddr, server.Addr)
			}

			if server.ReadHeaderTimeout == 0 {
				t.Error("expected ReadHeaderTimeout to be set")
			}
		})
	}
}
