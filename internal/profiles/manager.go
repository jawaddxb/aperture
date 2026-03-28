// Package profiles implements the site profile system for domain intelligence.
// It loads YAML profiles at init and matches URLs to extract structured data.
package profiles

import (
	"context"
	"embed"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/chromedp"
	"gopkg.in/yaml.v3"
)

//go:embed data/*.yaml
var profileFS embed.FS

// YAMLProfileManager loads profiles from embedded YAML and matches URLs.
type YAMLProfileManager struct {
	profiles []domain.SiteProfile
}

// NewYAMLProfileManager reads all embedded YAML profiles and returns a manager.
func NewYAMLProfileManager() (*YAMLProfileManager, error) {
	entries, err := profileFS.ReadDir("data")
	if err != nil {
		return nil, fmt.Errorf("read profile dir: %w", err)
	}

	var profiles []domain.SiteProfile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := profileFS.ReadFile("data/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read profile %s: %w", entry.Name(), err)
		}
		var sp domain.SiteProfile
		if err := yaml.Unmarshal(data, &sp); err != nil {
			return nil, fmt.Errorf("parse profile %s: %w", entry.Name(), err)
		}
		profiles = append(profiles, sp)
	}

	return &YAMLProfileManager{profiles: profiles}, nil
}

// Match returns the best profile for a given URL, or nil if no match.
func (m *YAMLProfileManager) Match(rawURL string) *domain.ProfileMatch {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	host := strings.ToLower(u.Hostname())
	path := u.Path
	if u.RawQuery != "" {
		path = path + "?" + u.RawQuery
	}

	for _, sp := range m.profiles {
		if !matchProfileDomain(sp.Domain, host) {
			continue
		}
		for pageType, page := range sp.Pages {
			for _, pattern := range page.URLPatterns {
				if strings.Contains(path, pattern) {
					p := page
					return &domain.ProfileMatch{
						ProfileDomain: sp.Domain,
						PageType:      pageType,
						Profile:       &p,
					}
				}
			}
		}
	}
	return nil
}

// Extract uses the matched profile to extract structured data from the page.
func (m *YAMLProfileManager) Extract(ctx context.Context, match *domain.ProfileMatch, inst domain.BrowserInstance) (map[string]interface{}, error) {
	if match == nil || match.Profile == nil {
		return nil, nil
	}

	result := make(map[string]interface{})
	browserCtx := inst.Context()

	for fieldName, schema := range match.Profile.Schema {
		if schema.Selector == "" {
			continue
		}

		var rawVal string
		var runCtx context.Context
		var cancel context.CancelFunc
		if deadline, ok := ctx.Deadline(); ok {
			runCtx, cancel = context.WithDeadline(browserCtx, deadline)
		} else {
			runCtx, cancel = context.WithCancel(browserCtx)
		}

		// Try to extract text from the first matching selector.
		err := chromedp.Run(runCtx,
			chromedp.Text(schema.Selector, &rawVal, chromedp.ByQuery, chromedp.AtLeast(0)),
		)
		cancel()

		if err != nil || rawVal == "" {
			// Try Value (for input fields).
			runCtx2, cancel2 := context.WithCancel(browserCtx)
			_ = chromedp.Run(runCtx2,
				chromedp.Value(schema.Selector, &rawVal, chromedp.ByQuery),
			)
			cancel2()
		}

		if rawVal == "" {
			continue
		}

		result[fieldName] = convertValue(rawVal, schema.Type)
	}

	return result, nil
}

// AvailableActions returns the sorted semantic action names for the matched page.
func (m *YAMLProfileManager) AvailableActions(match *domain.ProfileMatch) []string {
	if match == nil || match.Profile == nil {
		return nil
	}
	actions := make([]string, 0, len(match.Profile.Actions))
	for name := range match.Profile.Actions {
		actions = append(actions, name)
	}
	sort.Strings(actions)
	return actions
}

// Profiles returns all loaded site profiles.
func (m *YAMLProfileManager) Profiles() []domain.SiteProfile {
	return m.profiles
}

// NoopProfileManager is a no-op implementation for when profiles fail to load.
type NoopProfileManager struct{}

// NewNoopProfileManager returns a no-op profile manager.
func NewNoopProfileManager() *NoopProfileManager {
	return &NoopProfileManager{}
}

func (n *NoopProfileManager) Match(string) *domain.ProfileMatch                     { return nil }
func (n *NoopProfileManager) Extract(context.Context, *domain.ProfileMatch, domain.BrowserInstance) (map[string]interface{}, error) {
	return nil, nil
}
func (n *NoopProfileManager) AvailableActions(*domain.ProfileMatch) []string { return nil }
func (n *NoopProfileManager) Profiles() []domain.SiteProfile                 { return nil }

// ─── helpers ────────────────────────────────────────────────────────────────────

// matchProfileDomain matches a pattern like "*.example.com" against a hostname.
func matchProfileDomain(pattern, host string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	host = strings.ToLower(strings.TrimSpace(host))

	if pattern == host {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		if strings.HasSuffix(host, suffix) {
			return true
		}
		if host == pattern[2:] {
			return true
		}
	}
	return false
}

var (
	currencyRe = regexp.MustCompile(`[\$€£¥₹,]`)
	ratingRe   = regexp.MustCompile(`([\d.]+)\s*out of`)
	numberRe   = regexp.MustCompile(`[,\s]`)
)

// convertValue converts a raw extracted string to the appropriate Go type.
func convertValue(raw, typ string) interface{} {
	raw = strings.TrimSpace(raw)
	switch typ {
	case "currency":
		cleaned := currencyRe.ReplaceAllString(raw, "")
		cleaned = strings.TrimSpace(cleaned)
		if f, err := strconv.ParseFloat(cleaned, 64); err == nil {
			return f
		}
		return raw
	case "rating":
		if m := ratingRe.FindStringSubmatch(raw); len(m) > 1 {
			if f, err := strconv.ParseFloat(m[1], 64); err == nil {
				return f
			}
		}
		return raw
	case "number":
		cleaned := numberRe.ReplaceAllString(raw, "")
		if i, err := strconv.ParseInt(cleaned, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(cleaned, 64); err == nil {
			return f
		}
		return raw
	case "boolean":
		lower := strings.ToLower(raw)
		return lower == "true" || lower == "yes" || lower == "1"
	default: // "string"
		return raw
	}
}

// compile-time interface assertions.
var _ domain.SiteProfileManager = (*YAMLProfileManager)(nil)
var _ domain.SiteProfileManager = (*NoopProfileManager)(nil)
