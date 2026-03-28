package browser

import (
	"context"
	"math/rand"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// coreStealthJS patches navigator.webdriver, languages, and permissions.
// WebGL vendor/renderer is only faked when NOT using SwiftShader, because
// SwiftShader has its own consistent "Google Inc. (Google)" / "ANGLE (Google,
// SwiftShader...)" strings that blend into the crowd of all SwiftShader users.
const coreStealthJS = `
(function() {
    Object.defineProperty(navigator, 'webdriver', { get: () => false });
    Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });

    const origQuery = window.navigator.permissions.query;
    window.navigator.permissions.query = (p) => (
        p.name === 'notifications' ?
            Promise.resolve({ state: Notification.permission }) :
            origQuery(p)
    );
})();`

// webglSpoofJS fakes WebGL vendor/renderer for non-SwiftShader modes.
// When SwiftShader is active, this is NOT injected — SwiftShader's native
// values ("Google Inc." / "ANGLE SwiftShader") are consistent across all
// instances, which is the entire point of crowd-blending.
const webglSpoofJS = `
(function(){
    const gp = WebGLRenderingContext.prototype.getParameter;
    WebGLRenderingContext.prototype.getParameter = function(param) {
        if (param === 37445) return 'Intel Inc.';
        if (param === 37446) return 'Intel(R) Iris(TM) Graphics 6100';
        return gp.apply(this, arguments);
    };
})();`

// commonViewports contains real-world screen resolutions for randomization.
var commonViewports = [][2]int64{
	{1920, 1080}, {1440, 900}, {1536, 864},
	{1366, 768}, {2560, 1440}, {1680, 1050},
}

// ApplyStealth injects scripts and configures the browser to evade bot detection.
// It is called once per instance spawn and once per reset.
func ApplyStealth(ctx context.Context, cfg domain.StealthConfig) error {
	if !cfg.Enabled {
		return nil
	}

	// Phase 1: Collect all JS to inject as a single script.
	js := coreStealthJS

	// WebGL fingerprint strategy:
	// - SwiftShader: no JS spoofing needed — hardware-level crowd-blending.
	// - Noise: inject canvas noise (legacy, ML-detectable).
	// - Native/other: spoof WebGL vendor/renderer to look like common Intel GPU.
	switch cfg.WebGL {
	case "swiftshader":
		// No canvas noise, no WebGL spoof. SwiftShader handles everything at GPU level.
	case "noise":
		js += webglSpoofJS
		js += canvasNoiseJS
	default:
		// native or unset: still spoof vendor/renderer for basic protection
		js += webglSpoofJS
	}

	if cfg.BlockWebRTC {
		js += blockWebRTCJS
	}
	if cfg.MockPlugins {
		js += mockPluginsJS
	}

	// Phase 2: Build CDP action list.
	actions := []chromedp.Action{
		injectScript(js),
	}

	if cfg.UserAgent != "" {
		actions = append(actions, emulation.SetUserAgentOverride(cfg.UserAgent))
	}

	if cfg.RandomView {
		w, h := randomViewport()
		actions = append(actions, setViewport(w, h))
	}

	if cfg.Timezone != "" {
		actions = append(actions, setTimezone(cfg.Timezone))
	}

	if cfg.GeoLatitude != 0 || cfg.GeoLongitude != 0 {
		actions = append(actions, setGeolocation(cfg.GeoLatitude, cfg.GeoLongitude))
	}

	return chromedp.Run(ctx, actions...)
}

// injectScript returns an action that registers JS to run on every new document.
func injectScript(js string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(js).Do(ctx)
		return err
	}
}

// randomViewport picks a random resolution from commonViewports.
func randomViewport() (int64, int64) {
	v := commonViewports[rand.Intn(len(commonViewports))]
	return v[0], v[1]
}

// setViewport returns an action that overrides device metrics.
func setViewport(w, h int64) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		return emulation.SetDeviceMetricsOverride(w, h, 1.0, false).Do(ctx)
	}
}

// setTimezone returns an action that overrides the browser timezone.
func setTimezone(tz string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		return emulation.SetTimezoneOverride(tz).Do(ctx)
	}
}

// setGeolocation returns an action that overrides the browser geolocation.
func setGeolocation(lat, lon float64) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		return emulation.SetGeolocationOverride().
			WithLatitude(lat).WithLongitude(lon).WithAccuracy(100).Do(ctx)
	}
}
