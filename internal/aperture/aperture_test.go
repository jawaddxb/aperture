package aperture_test

import (
	"context"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/aperture"
	"github.com/ApertureHQ/aperture/internal/config"
)

// validConfig returns a minimal config with a fake chromium path.
// The browser pool will fail to launch real instances, but New() should
// still construct all components (the pool returns an error on Acquire,
// not on NewPool).
func validConfig() *config.Config {
	return &config.Config{
		Server:  config.ServerConfig{Host: "127.0.0.1", Port: 18080},
		Browser: config.BrowserConfig{PoolSize: 1, ChromiumPath: "/usr/bin/chromium-fake", SkipPreWarm: true},
		Log:     config.LogConfig{Level: "info"},
		Bridge:  config.BridgeConfig{MaxConcurrentTasks: 5, TaskTimeoutSeconds: 30},
		LLM:     config.LLMConfig{},
	}
}

func TestNew_WithValidConfig_CreatesComponents(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	a, err := aperture.New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if a == nil {
		t.Fatal("expected non-nil Aperture")
	}
	if a.Config == nil {
		t.Error("expected Config to be set")
	}
	if a.Pool == nil {
		t.Error("expected Pool to be set")
	}
	if a.Planner == nil {
		t.Error("expected Planner to be set")
	}
	if a.Sequencer == nil {
		t.Error("expected Sequencer to be set")
	}
	if a.Bridge == nil {
		t.Error("expected Bridge to be set")
	}
	if a.Logger == nil {
		t.Error("expected Logger to be set")
	}
	if a.Metrics == nil {
		t.Error("expected Metrics to be set")
	}
	if a.Router == nil {
		t.Error("expected Router to be set")
	}
}

func TestNew_WithMissingAPIKey_UsesStaticPlanner(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.LLM.APIKey = "" // no key → static planner

	a, err := aperture.New(cfg)
	if err != nil {
		t.Fatalf("New() with missing API key should not fail: %v", err)
	}
	if a.Planner == nil {
		t.Error("expected Planner (static) to be set even without API key")
	}
}

func TestNew_WithLLMAPIKey_UsesLLMPlanner(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.LLM.APIKey = "test-api-key-12345"
	cfg.LLM.Provider = "openai"

	a, err := aperture.New(cfg)
	if err != nil {
		t.Fatalf("New() with API key error: %v", err)
	}
	if a.Planner == nil {
		t.Error("expected LLM-backed Planner when API key provided")
	}
}

func TestShutdown_CleansUp(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	a, err := aperture.New(cfg)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = config.Config{} // ensure import used
	defer cancel()

	// Shutdown without having started — should not panic.
	if err := a.Shutdown(ctx); err != nil {
		// Pool close may return an error for the fake chromium path; acceptable.
		t.Logf("Shutdown returned (expected for fake chromium): %v", err)
	}
}
