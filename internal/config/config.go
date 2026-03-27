// Package config provides application configuration loading and validation.
// All other packages receive config values via constructor parameters —
// only this package imports viper directly.
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

// BrowserConfig holds Chromium browser pool settings.
type BrowserConfig struct {
	PoolSize     int    `mapstructure:"pool_size"`
	ChromiumPath string `mapstructure:"chromium_path"`
	// SkipPreWarm disables pool pre-warming on startup when true.
	// Zero value (false) = pre-warm in production; set to true in tests
	// to avoid requiring a real Chromium binary.
	SkipPreWarm bool `mapstructure:"skip_pre_warm"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	URL string `mapstructure:"url"`
}

// SQLiteConfig holds SQLite database settings.
type SQLiteConfig struct {
	Path string `mapstructure:"path"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level string `mapstructure:"level"`
}

// APIConfig holds API authentication settings.
type APIConfig struct {
	KeyPrefix string `mapstructure:"key_prefix"`
}

// LLMConfig holds LLM provider settings for the planner.
type LLMConfig struct {
	// Provider selects the backend: "openai" or "anthropic".
	Provider string `mapstructure:"provider"`

	// Model is the model ID (e.g. "gpt-4o", "claude-3-5-sonnet-20241022").
	Model string `mapstructure:"model"`

	// APIKey is the provider authentication key.
	APIKey string `mapstructure:"api_key"`

	// BaseURL overrides the default API endpoint.
	BaseURL string `mapstructure:"base_url"`
}

// BridgeConfig holds OpenClaw bridge integration settings.
type BridgeConfig struct {
	// MaxConcurrentTasks limits how many bridge tasks may run simultaneously.
	// Defaults to 10 when zero.
	MaxConcurrentTasks int `mapstructure:"max_concurrent_tasks"`

	// TaskTimeoutSeconds is the default per-task timeout.
	// Defaults to 120 when zero.
	TaskTimeoutSeconds int `mapstructure:"task_timeout_seconds"`
}

// Config is the root application configuration struct.
// It is validated on startup and passed to constructors via dependency injection.
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	Browser BrowserConfig `mapstructure:"browser"`
	Redis   RedisConfig   `mapstructure:"redis"`
	SQLite  SQLiteConfig  `mapstructure:"sqlite"`
	Log     LogConfig     `mapstructure:"log"`
	API     APIConfig     `mapstructure:"api"`
	LLM     LLMConfig     `mapstructure:"llm"`
	Bridge  BridgeConfig  `mapstructure:"bridge"`
}

// Load reads configuration from aperture.yaml and APERTURE_* environment variables.
// Environment variables take precedence over YAML values.
// Returns an error if required fields are missing or invalid.
func Load() (*Config, error) {
	return LoadFromFile("aperture.yaml")
}

// LoadFromFile reads configuration from the given YAML file path and APERTURE_* env vars.
func LoadFromFile(path string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.SetEnvPrefix("APERTURE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Ignore file-not-found; env vars alone can supply all values.
	if err := v.ReadInConfig(); err != nil {
		var notFoundErr viper.ConfigFileNotFoundError
		if !errors.As(err, &notFoundErr) && !isFileNotFoundError(err) {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// setDefaults registers non-security-sensitive defaults.
func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("browser.pool_size", 5)
	v.SetDefault("browser.skip_pre_warm", false)
	v.SetDefault("log.level", "info")
	v.SetDefault("api.key_prefix", "apt_")
	v.SetDefault("sqlite.path", "aperture.db")
	v.SetDefault("redis.url", "redis://localhost:6379")
	v.SetDefault("llm.provider", "openai")
	v.SetDefault("llm.model", "gpt-4o")
	v.SetDefault("bridge.max_concurrent_tasks", 10)
	v.SetDefault("bridge.task_timeout_seconds", 120)
}

// validate checks that required fields are present.
func validate(cfg *Config) error {
	if cfg.Browser.ChromiumPath == "" {
		return errors.New("browser.chromium_path is required")
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", cfg.Server.Port)
	}
	return nil
}

// isFileNotFoundError returns true if the error is a "no such file" OS error.
func isFileNotFoundError(err error) bool {
	return strings.Contains(err.Error(), "no such file") ||
		strings.Contains(err.Error(), "cannot find the file")
}
