// Package api provides HTTP handlers and middleware for Aperture.
// This file validates URLs for the Screenshot and similar handler-level endpoints.
package api

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// validateScreenshotURL checks that a URL is safe for screenshot capture.
// Same rules as the executor-level navigate validator: blocks non-http(s),
// private/loopback IPs, cloud metadata, and embedded credentials.
func validateScreenshotURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("blocked scheme %q: only http/https allowed", scheme)
	}

	if u.User != nil {
		return fmt.Errorf("blocked: URLs with embedded credentials are not allowed")
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("blocked: empty hostname")
	}

	metadataHosts := []string{
		"169.254.169.254", "metadata.google.internal", "metadata.google.com",
		"100.100.100.200", "169.254.170.2", "fd00:ec2::254",
	}
	hostLower := strings.ToLower(host)
	for _, mh := range metadataHosts {
		if hostLower == mh {
			return fmt.Errorf("blocked: cloud metadata endpoint %q", host)
		}
	}

	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("blocked: private/loopback address %s", ip)
		}
	}

	return nil
}
