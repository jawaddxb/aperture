// Package domain defines core interfaces for Aperture.
// This file defines the Proxy struct and ProxyProvider interface.
package domain

import "context"

// Proxy holds the connection details for an HTTP/HTTPS/SOCKS proxy server.
type Proxy struct {
	// URL is the proxy address, e.g. "http://proxy.example.com:8080".
	URL string

	// Username is the optional proxy authentication username.
	Username string

	// Password is the optional proxy authentication password.
	Password string
}

// ProxyProvider resolves which proxy to use for a given session.
// Implementations must be safe for concurrent use.
type ProxyProvider interface {
	// GetProxy returns the proxy to use for sessionID.
	// Returns nil, nil when no proxy should be used for the session.
	GetProxy(ctx context.Context, sessionID string) (*Proxy, error)
}
