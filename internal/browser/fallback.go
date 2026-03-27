package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// ScraplingResult is the JSON output from the Python fallback script.
type ScraplingResult struct {
	OK           bool   `json:"ok"`
	HTML         string `json:"html"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	ScreenshotB64 string `json:"screenshot_b64"`
	Error        string `json:"error"`
}

// FallbackFetch calls Scrapling's StealthyFetcher via subprocess for
// sites that Aperture's native stealth cannot bypass (e.g., Cloudflare Turnstile).
// toolsDir should point to the directory containing scrapling_fallback.py.
func FallbackFetch(ctx context.Context, toolsDir, url string, screenshot bool, timeout time.Duration) (*ScraplingResult, error) {
	script := filepath.Join(toolsDir, "scrapling_fallback.py")

	args := []string{script, "--url", url, "--timeout", strconv.Itoa(int(timeout.Seconds()))}
	if screenshot {
		args = append(args, "--screenshot")
	}

	cmd := exec.CommandContext(ctx, "python3", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("scrapling fallback failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("scrapling fallback exec: %w", err)
	}

	var result ScraplingResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("scrapling fallback JSON parse: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("scrapling fallback error: %s", result.Error)
	}

	return &result, nil
}
