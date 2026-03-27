package browser_test

import (
	"os"
	"runtime"
	"testing"
)

// chromiumPath returns the path to a Chromium/Chrome binary for tests.
// Override via APERTURE_CHROMIUM_PATH environment variable.
func chromiumPath(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("APERTURE_CHROMIUM_PATH"); p != "" {
		return p
	}
	switch runtime.GOOS {
	case "darwin":
		return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	case "linux":
		for _, p := range []string{"/usr/bin/chromium", "/usr/bin/chromium-browser", "/usr/bin/google-chrome"} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	t.Skip("no Chromium binary found; set APERTURE_CHROMIUM_PATH to enable")
	return ""
}
