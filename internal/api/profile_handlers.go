// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file contains handlers for site profile endpoints.
package api

import (
	"net/http"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// ProfileHandlers groups HTTP handlers for site profile endpoints.
type ProfileHandlers struct {
	manager domain.SiteProfileManager
}

// NewProfileHandlers constructs ProfileHandlers.
func NewProfileHandlers(manager domain.SiteProfileManager) *ProfileHandlers {
	return &ProfileHandlers{manager: manager}
}

// profileSummary is the JSON response for a single loaded profile.
type profileSummary struct {
	Domain    string   `json:"domain"`
	Version   string   `json:"version"`
	PageTypes []string `json:"page_types"`
}

// List handles GET /api/v1/profiles.
func (h *ProfileHandlers) List(w http.ResponseWriter, r *http.Request) {
	profiles := h.manager.Profiles()
	out := make([]profileSummary, 0, len(profiles))
	for _, sp := range profiles {
		pageTypes := make([]string, 0, len(sp.Pages))
		for pt := range sp.Pages {
			pageTypes = append(pageTypes, pt)
		}
		out = append(out, profileSummary{
			Domain:    sp.Domain,
			Version:   sp.Version,
			PageTypes: pageTypes,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
