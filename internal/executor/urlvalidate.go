// Package executor provides action executors for Aperture.
// This file validates URLs before navigation to prevent SSRF,
// local file access, and other URL-based attacks.
package executor

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// allowTestDataURLs is a package-level flag to permit data: URLs in tests.
// Set to true in test init() functions. Never true in production.
var allowTestDataURLs = false

// validateNavigateURL checks that a URL is safe to navigate to.
// Blocks: file://, javascript:, data:, private/loopback IPs, cloud metadata endpoints.
func validateNavigateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)

	// Allow data: URLs in test mode only.
	if scheme == "data" && allowTestDataURLs {
		return nil
	}

	// Only allow http and https schemes.
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("blocked scheme %q: only http/https allowed", u.Scheme)
	}

	// Block URLs with embedded credentials.
	if u.User != nil {
		return fmt.Errorf("blocked: URLs with embedded credentials are not allowed")
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("blocked: empty hostname")
	}

	// Block cloud metadata endpoints by hostname.
	metadataHosts := []string{
		"169.254.169.254",                // AWS/Azure IMDS
		"metadata.google.internal",       // GCP
		"metadata.google.com",            // GCP alt
		"100.100.100.200",                // Alibaba Cloud
		"169.254.170.2",                  // AWS ECS task metadata
		"fd00:ec2::254",                  // AWS IPv6 IMDS
	}
	hostLower := strings.ToLower(host)
	for _, mh := range metadataHosts {
		if hostLower == mh {
			return fmt.Errorf("blocked: cloud metadata endpoint %q", host)
		}
	}

	// Resolve hostname and check for private/loopback IPs.
	if ip := net.ParseIP(host); ip != nil {
		if err := checkPrivateIP(ip); err != nil {
			return err
		}
	} else {
		// Resolve DNS to check for SSRF via DNS rebinding to private IPs.
		addrs, err := net.LookupHost(host)
		if err == nil {
			for _, addr := range addrs {
				if ip := net.ParseIP(addr); ip != nil {
					if err := checkPrivateIP(ip); err != nil {
						return fmt.Errorf("blocked: %s resolves to private IP %s", host, addr)
					}
				}
			}
		}
		// DNS resolution failure is NOT blocked — the browser will handle it.
	}

	return nil
}

// checkPrivateIP returns an error if the IP is loopback, private, or link-local.
func checkPrivateIP(ip net.IP) error {
	if ip.IsLoopback() {
		return fmt.Errorf("blocked: loopback address %s", ip)
	}
	if ip.IsPrivate() {
		return fmt.Errorf("blocked: private network address %s", ip)
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("blocked: link-local address %s", ip)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("blocked: unspecified address %s", ip)
	}
	return nil
}
