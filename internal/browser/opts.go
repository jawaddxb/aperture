// Package browser provides Chromium lifecycle management for Aperture.
// It implements the domain.BrowserPool and domain.BrowserInstance interfaces.
package browser

import (
	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// DefaultLaunchFlags returns hardened headless Chromium launch flags.
// These flags optimise for stealth, security, and stability.
// GPU handling depends on WebGL mode — see SwiftShaderFlags.
func DefaultLaunchFlags(webglMode string) []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", "new"), // new headless = identical TLS to headed Chrome
		chromedp.Flag("no-sandbox", true),
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

	// GPU + WebGL fingerprint strategy:
	// - "swiftshader": Software GPU rendering. Every instance produces identical WebGL
	//   output — crowd-blends into thousands of other SwiftShader users.
	//   Key insight: randomisation is detectable, homogeneity is not.
	// - "noise": Legacy mode. GPU disabled, canvas noise injected via JS (ML-detectable).
	// - "native": Real GPU. Unique fingerprint per machine. Use only for trusted/local.
	switch webglMode {
	case "swiftshader":
		opts = append(opts,
			chromedp.Flag("use-gl", "swiftshader"),        // SwiftShader software GPU
			chromedp.Flag("use-angle", "swiftshader"),      // Force ANGLE to use SwiftShader backend
			chromedp.Flag("disable-gpu-shader-disk-cache", true), // No persistent GPU cache artifacts
		)
		// Note: disable-gpu is deliberately NOT set — SwiftShader needs GPU subsystem active.
	case "native":
		// Real GPU — no disable-gpu, no SwiftShader. Unique fingerprint.
	default: // "noise" or unknown — legacy behaviour
		opts = append(opts, chromedp.Flag("disable-gpu", true))
	}

	return opts
}

// BuildAllocatorOptions assembles the full set of ExecAllocatorOptions for a
// Chromium instance, merging defaults with any caller-supplied overrides.
// If stealth.UTLSProxyAddr is set, a proxy server flag is added so Chromium
// routes HTTPS CONNECT tunnels through the uTLS proxy.
func BuildAllocatorOptions(chromiumPath string, stealth domain.StealthConfig, extra ...chromedp.ExecAllocatorOption) []chromedp.ExecAllocatorOption {
	webgl := stealth.WebGL
	if webgl == "" {
		webgl = "swiftshader"
	}
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(chromiumPath),
	}
	opts = append(opts, DefaultLaunchFlags(webgl)...)
	if stealth.UTLSProxyAddr != "" {
		opts = append(opts, chromedp.ProxyServer(stealth.UTLSProxyAddr))
	}
	opts = append(opts, extra...)
	return opts
}
