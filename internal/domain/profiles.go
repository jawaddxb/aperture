// Package domain defines core interfaces for Aperture.
// This file defines the ProfileManager interface and site profile types.
package domain

import "context"

// SiteProfile defines structured intelligence for a domain.
type SiteProfile struct {
	Domain  string                 `json:"domain" yaml:"domain"`
	Version string                 `json:"version" yaml:"version"`
	Pages   map[string]PageProfile `json:"pages" yaml:"pages"`
}

// PageProfile defines a single page type within a domain.
type PageProfile struct {
	URLPatterns []string                 `json:"url_patterns" yaml:"url_patterns"`
	Schema      map[string]FieldSchema   `json:"schema" yaml:"schema"`
	Actions     map[string]ActionMapping `json:"actions" yaml:"actions"`
}

// FieldSchema describes how to extract a field from the page.
type FieldSchema struct {
	Selector string `json:"selector,omitempty" yaml:"selector,omitempty"`
	AXRole   string `json:"ax_role,omitempty" yaml:"ax_role,omitempty"`
	AXName   string `json:"ax_name,omitempty" yaml:"ax_name,omitempty"`
	Type     string `json:"type" yaml:"type"`
	Required bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

// ActionMapping maps a semantic action name to page elements.
type ActionMapping struct {
	Primary  ElementHint `json:"primary" yaml:"primary"`
	Fallback ElementHint `json:"fallback,omitempty" yaml:"fallback,omitempty"`
}

// ElementHint describes how to find an element.
type ElementHint struct {
	AXRole   string `json:"ax_role,omitempty" yaml:"ax_role,omitempty"`
	AXName   string `json:"ax_name,omitempty" yaml:"ax_name,omitempty"`
	Selector string `json:"selector,omitempty" yaml:"selector,omitempty"`
}

// ProfileMatch is returned when a URL matches a site profile.
type ProfileMatch struct {
	ProfileDomain string       `json:"profile_domain"`
	PageType      string       `json:"page_type"`
	Profile       *PageProfile `json:"profile"`
}

// SiteProfileManager loads and matches site profiles.
type SiteProfileManager interface {
	// Match returns the best profile for a given URL, or nil if no match.
	Match(rawURL string) *ProfileMatch

	// Extract uses the matched profile to extract structured data from the page.
	// Returns map of field name -> extracted value.
	Extract(ctx context.Context, match *ProfileMatch, inst BrowserInstance) (map[string]interface{}, error)

	// AvailableActions returns the semantic action names available on the matched page.
	AvailableActions(match *ProfileMatch) []string

	// Profiles returns all loaded site profiles.
	Profiles() []SiteProfile
}
