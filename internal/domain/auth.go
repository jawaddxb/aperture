// Package domain defines core interfaces for Aperture.
// This file defines AuthPersistence, CookieJar, and Cookie types.
package domain

import (
	"context"
	"time"
)

// Cookie represents a single browser cookie with JSON-serializable fields.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Expires  int64  `json:"expires"`
	HTTPOnly bool   `json:"http_only"`
	Secure   bool   `json:"secure"`
}

// CookieJar holds a snapshot of all cookies for a session.
type CookieJar struct {
	// SessionID is the session this jar belongs to.
	SessionID string `json:"session_id"`
	// Cookies is the full list of cookies captured at SavedAt.
	Cookies []Cookie `json:"cookies"`
	// SavedAt is the UTC time when the jar was persisted.
	SavedAt time.Time `json:"saved_at"`
}

// AuthPersistence saves and restores browser cookie state across sessions.
// Implementations must be safe for concurrent use.
type AuthPersistence interface {
	// SaveCookies exports all cookies from inst and persists them under sessionID.
	SaveCookies(ctx context.Context, sessionID string, inst BrowserInstance) error

	// LoadCookies imports previously persisted cookies for sessionID into inst.
	// Returns no error and loads nothing if sessionID has no saved cookies.
	LoadCookies(ctx context.Context, sessionID string, inst BrowserInstance) error

	// ClearCookies removes persisted cookies for sessionID.
	// Returns no error if no persisted cookies exist for sessionID.
	ClearCookies(ctx context.Context, sessionID string) error
}
