// Package llm provides LLM client implementations for OpenAI and Anthropic.
// Use NewClient to construct a provider-specific client from Config.
package llm

import (
	"fmt"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// Provider constants identify supported LLM providers.
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
)

// Config holds the configuration required to construct an LLM client.
type Config struct {
	// Provider selects the backend: ProviderOpenAI or ProviderAnthropic.
	Provider string

	// Model is the model ID to use, e.g. "gpt-4o" or "claude-3-5-sonnet-20241022".
	Model string

	// APIKey is the authentication key for the chosen provider.
	APIKey string

	// BaseURL overrides the default API endpoint (e.g. for OpenRouter proxies).
	// Leave empty to use the provider default.
	BaseURL string

	// MaxTokens caps the number of tokens in each completion response.
	// Defaults to 4096 when zero.
	MaxTokens int

	// Temperature controls response randomness in [0.0, 2.0].
	// Defaults to 0.2 when zero.
	Temperature float64
}

// NewClient constructs a domain.LLMClient for the provider specified in cfg.
// Returns an error when Provider is unrecognised or APIKey is empty.
func NewClient(cfg Config) (domain.LLMClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("llm: APIKey must not be empty")
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.2
	}

	switch cfg.Provider {
	case ProviderOpenAI:
		return newOpenAIClient(cfg), nil
	case ProviderAnthropic:
		return newAnthropicClient(cfg), nil
	default:
		return nil, fmt.Errorf("llm: unsupported provider %q", cfg.Provider)
	}
}
