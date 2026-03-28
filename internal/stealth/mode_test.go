package stealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModeRouter_Detect_AgentOverride(t *testing.T) {
	r := NewModeRouter()

	// Valid override trumps URL.
	assert.Equal(t, ModeMax, r.Detect("https://example.com", "max"))
	assert.Equal(t, ModeHardened, r.Detect("https://example.com", "hardened"))
	assert.Equal(t, ModeResearch, r.Detect("https://linkedin.com", "research"))
}

func TestModeRouter_Detect_InvalidOverrideFallsThrough(t *testing.T) {
	r := NewModeRouter()

	// Unknown override → auto-detect.
	assert.Equal(t, ModeHardened, r.Detect("https://linkedin.com", "bogus"))
}

func TestModeRouter_Detect_LinkedIn(t *testing.T) {
	r := NewModeRouter()

	cases := []string{
		"https://linkedin.com/in/someone",
		"https://www.linkedin.com/jobs",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			assert.Equal(t, ModeHardened, r.Detect(u, ""))
		})
	}
}

func TestModeRouter_Detect_CloudflareHeavy(t *testing.T) {
	r := NewModeRouter()

	cases := []string{
		"https://discord.com/channels",
		"https://www.shopify.com",
		"https://medium.com/post",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			assert.Equal(t, ModeMax, r.Detect(u, ""))
		})
	}
}

func TestModeRouter_Detect_Research(t *testing.T) {
	r := NewModeRouter()

	cases := []string{
		"https://example.com",
		"https://github.com",
		"https://news.ycombinator.com",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			assert.Equal(t, ModeResearch, r.Detect(u, ""))
		})
	}
}

func TestModeRouter_Detect_EmptyURL(t *testing.T) {
	r := NewModeRouter()
	assert.Equal(t, ModeResearch, r.Detect("", ""))
}
