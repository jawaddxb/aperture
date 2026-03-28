// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file implements POST /api/v1/sessions/import for cookie-based session import.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ApertureHQ/aperture/internal/auth"
	"github.com/ApertureHQ/aperture/internal/domain"
)

// SessionImportHandlers groups HTTP handlers for session import endpoints.
type SessionImportHandlers struct {
	sessionManager domain.SessionManager
	profileManager domain.SiteProfileManager
}

// NewSessionImportHandlers constructs a SessionImportHandlers.
func NewSessionImportHandlers(sm domain.SessionManager, pm domain.SiteProfileManager) *SessionImportHandlers {
	return &SessionImportHandlers{
		sessionManager: sm,
		profileManager: pm,
	}
}

// ImportSessionResponse is the JSON body returned for a successful import.
type ImportSessionResponse struct {
	SessionID       string    `json:"session_id"`
	ProfileID       string    `json:"profile_id"`
	TrustMode       string    `json:"trust_mode"`
	CookiesImported int       `json:"cookies_imported"`
	Domains         []string  `json:"domains"`
}

// Import handles POST /api/v1/sessions/import.
// It accepts a multipart form with fields:
//   - cookies:     Netscape cookie file text
//   - trust_mode:  "preserve" or "standard" (default: preserve)
//   - domain_hint: e.g. "linkedin.com" (used for profile naming)
func (h *SessionImportHandlers) Import(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_FORM", "failed to parse multipart form: "+err.Error())
		return
	}

	cookiesText := r.FormValue("cookies")
	if strings.TrimSpace(cookiesText) == "" {
		writeError(w, http.StatusBadRequest, "MISSING_COOKIES", "field 'cookies' is required")
		return
	}

	trustModeStr := r.FormValue("trust_mode")
	if trustModeStr == "" {
		trustModeStr = string(domain.TrustPreserve)
	}
	trustMode := domain.TrustMode(trustModeStr)
	if trustMode != domain.TrustStandard && trustMode != domain.TrustPreserve {
		writeError(w, http.StatusBadRequest, "INVALID_TRUST_MODE", "trust_mode must be 'standard' or 'preserve'")
		return
	}

	domainHint := strings.TrimSpace(r.FormValue("domain_hint"))
	if domainHint == "" {
		domainHint = "unknown"
	}

	// Parse Netscape cookie file.
	cookies, domains, err := auth.ParseNetscapeCookies(strings.NewReader(cookiesText))
	if err != nil {
		writeError(w, http.StatusBadRequest, "COOKIE_PARSE_ERROR", err.Error())
		return
	}

	profileID := buildProfileID(domainHint)

	// Attempt to register the profile with the profile manager if available.
	if h.profileManager != nil {
		if pm, ok := h.profileManager.(interface {
			CreateProfile(ctx context.Context, id string) (*domain.Profile, error)
		}); ok {
			if _, createErr := pm.CreateProfile(r.Context(), profileID); createErr != nil {
				slog.Warn("session import: profile creation failed (non-fatal)",
					"profile_id", profileID, "error", createErr)
			}
		}
	}

	// Build import goal and metadata.
	goal := fmt.Sprintf("imported session for %s (%d cookies)", domainHint, len(cookies))
	meta := map[string]string{
		"import_source":   "cookie_import",
		"domain_hint":     domainHint,
		"trust_mode":      string(trustMode),
		"imported_profile": profileID,
	}

	sess, err := h.sessionManager.Create(r.Context(), goal, meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SESSION_CREATE_FAILED", err.Error())
		return
	}

	// Decorate the session with import metadata.
	sess.TrustMode = trustMode
	sess.ImportedProfile = profileID
	sess.ImportedDomains = domains

	if updateErr := h.sessionManager.Update(r.Context(), sess); updateErr != nil {
		slog.Warn("session import: failed to update session with import metadata",
			"session_id", sess.ID, "error", updateErr)
	}

	slog.Info("session import completed",
		"session_id", sess.ID,
		"profile_id", profileID,
		"trust_mode", trustMode,
		"cookies_imported", len(cookies),
		"domains", domains,
	)

	writeJSON(w, http.StatusCreated, ImportSessionResponse{
		SessionID:       sess.ID,
		ProfileID:       profileID,
		TrustMode:       string(trustMode),
		CookiesImported: len(cookies),
		Domains:         domains,
	})
}

// buildProfileID generates a deterministic profile ID from a domain hint and timestamp.
func buildProfileID(domainHint string) string {
	safe := strings.ReplaceAll(domainHint, ".", "-")
	safe = strings.ReplaceAll(safe, "/", "-")
	return fmt.Sprintf("imported-%s-%d", safe, time.Now().Unix())
}
