package executor

import (
	"testing"
)

func init() {
	// Allow data: URLs in tests since many test cases use them for HTML test pages.
	allowTestDataURLs = true
}

func TestValidateNavigateURL_BlockedSchemes(t *testing.T) {
	tests := []struct {
		url     string
		blocked bool
	}{
		{"https://example.com", false},
		{"http://example.com", false},
		{"file:///etc/passwd", true},
		{"javascript:alert(1)", true},
		{"ftp://example.com/file.txt", true},
		{"gopher://evil.com", true},
	}
	// Temporarily disable test mode for this specific test.
	allowTestDataURLs = false
	defer func() { allowTestDataURLs = true }()

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := validateNavigateURL(tt.url)
			if tt.blocked && err == nil {
				t.Errorf("expected %q to be blocked, but it was allowed", tt.url)
			}
			if !tt.blocked && err != nil {
				t.Errorf("expected %q to be allowed, but got: %v", tt.url, err)
			}
		})
	}
}

func TestValidateNavigateURL_DataBlocked(t *testing.T) {
	allowTestDataURLs = false
	defer func() { allowTestDataURLs = true }()

	err := validateNavigateURL("data:text/html,<script>alert(1)</script>")
	if err == nil {
		t.Error("expected data: URL to be blocked in production mode")
	}
}

func TestValidateNavigateURL_SSRF(t *testing.T) {
	tests := []struct {
		url     string
		blocked bool
	}{
		{"http://169.254.169.254/latest/meta-data", true},       // AWS IMDS
		{"http://metadata.google.internal/", true},               // GCP
		{"http://127.0.0.1:8080/admin", true},                   // Loopback
		{"http://0.0.0.0/", true},                                // Unspecified
		{"http://[::1]/", true},                                  // IPv6 loopback
		{"http://192.168.1.1/", true},                            // Private
		{"http://10.0.0.1/", true},                               // Private
		{"http://172.16.0.1/", true},                             // Private
		{"https://example.com", false},                           // Public
		{"https://www.google.com", false},                        // Public
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := validateNavigateURL(tt.url)
			if tt.blocked && err == nil {
				t.Errorf("expected %q to be blocked (SSRF), but allowed", tt.url)
			}
			if !tt.blocked && err != nil {
				t.Errorf("expected %q to be allowed, but got: %v", tt.url, err)
			}
		})
	}
}

func TestValidateNavigateURL_EmbeddedCredentials(t *testing.T) {
	err := validateNavigateURL("http://admin:password@evil.com/")
	if err == nil {
		t.Error("expected URL with embedded credentials to be blocked")
	}
}

func TestValidateNavigateURL_PathTraversal(t *testing.T) {
	// These should be allowed (path traversal in URL path is harmless — it's the scheme/host that matters)
	err := validateNavigateURL("https://example.com/../../../etc/passwd")
	if err != nil {
		t.Errorf("URL path traversal should be allowed (browser handles it): %v", err)
	}
}
