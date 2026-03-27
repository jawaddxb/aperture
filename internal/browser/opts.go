// Package browser provides Chromium lifecycle management for Aperture.
// It implements the domain.BrowserPool and domain.BrowserInstance interfaces.
package browser

import (
	"github.com/chromedp/chromedp"
)

// DefaultLaunchFlags returns hardened headless Chromium launch flags.
// These flags optimise for stealth, security, and stability.
func DefaultLaunchFlags() []chromedp.ExecAllocatorOption {
	return []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", "new"), // new headless = identical TLS to headed Chrome
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-translate", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("safebrowsing-disable-auto-update", true),
		// Anti-detection flags
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-webrtc-hw-decoding", true),
		chromedp.Flag("enforce-webrtc-ip-permission-check", true),
		chromedp.Flag("disable-features", "WebRtcHideLocalIpsWithMdns"),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	}
}

// BuildAllocatorOptions assembles the full set of ExecAllocatorOptions for a
// Chromium instance, merging defaults with any caller-supplied overrides.
func BuildAllocatorOptions(chromiumPath string, extra ...chromedp.ExecAllocatorOption) []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(chromiumPath),
	}
	opts = append(opts, DefaultLaunchFlags()...)
	opts = append(opts, extra...)
	return opts
}
