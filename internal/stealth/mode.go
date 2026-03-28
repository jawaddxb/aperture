package stealth

import (
	"net/url"
	"strings"
)

// Mode represents a stealth operating mode.
type Mode string

const (
	// ModeResearch is the default mode: Chrome + SwiftShader + uTLS, fast (3-7s).
	ModeResearch Mode = "research"
	// ModeHardened is for sites with elevated bot detection (e.g. LinkedIn).
	ModeHardened Mode = "hardened"
	// ModeMax is for Cloudflare-heavy or adversarial sites; Camoufox primary (20-22s).
	ModeMax Mode = "max"
)

// cloudflareHeavyDomains is the set of domains that benefit from ModeMax.
// These are known to deploy Cloudflare Bot Management or equivalent.
var cloudflareHeavyDomains = map[string]bool{
	"cloudflare.com":   true,
	"discord.com":      true,
	"discord.gg":       true,
	"shopify.com":      true,
	"medium.com":       true,
	"doordash.com":     true,
	"instacart.com":    true,
	"ticketmaster.com": true,
	"stubhub.com":      true,
	"nike.com":         true,
	"supreme.com":      true,
	"footlocker.com":   true,
}

// hardenedDomains is the set of domains requiring ModeHardened.
var hardenedDomains = map[string]bool{
	"linkedin.com":    true,
	"www.linkedin.com": true,
}

// ModeRouter detects the appropriate stealth mode for a URL.
// An agent-supplied override always takes precedence over auto-detection.
type ModeRouter struct{}

// NewModeRouter returns a new ModeRouter.
func NewModeRouter() *ModeRouter {
	return &ModeRouter{}
}

// Detect returns the stealth Mode for the given rawURL.
// If agentOverride is non-empty and a valid Mode, it is returned immediately.
// Otherwise auto-detection logic runs.
func (r *ModeRouter) Detect(rawURL string, agentOverride string) Mode {
	if agentOverride != "" {
		m := Mode(agentOverride)
		if m == ModeResearch || m == ModeHardened || m == ModeMax {
			return m
		}
	}
	return r.autoDetect(rawURL)
}

// autoDetect infers the stealth mode from the URL's host.
func (r *ModeRouter) autoDetect(rawURL string) Mode {
	host := extractHost(rawURL)
	if host == "" {
		return ModeResearch
	}
	host = strings.ToLower(host)
	// Strip leading "www." for lookup.
	bare := strings.TrimPrefix(host, "www.")

	if hardenedDomains[host] || hardenedDomains[bare] {
		return ModeHardened
	}
	if cloudflareHeavyDomains[host] || cloudflareHeavyDomains[bare] {
		return ModeMax
	}
	return ModeResearch
}

// extractHost parses a URL and returns its host, or the input itself if
// parsing fails (handles bare hostnames passed without a scheme).
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.Host != "" {
		return u.Host
	}
	// Bare hostname with no scheme: url.Parse puts it in Path.
	return u.Path
}
