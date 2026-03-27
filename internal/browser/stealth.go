package browser

import (
	"context"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// stealthJS is a snippet that mocks navigator properties and WebGL to avoid detection.
const stealthJS = `
(function() {
    Object.defineProperty(navigator, 'webdriver', { get: () => false });
    Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });
    Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });

    const originalQuery = window.navigator.permissions.query;
    window.navigator.permissions.query = (parameters) => (
        parameters.name === 'notifications' ?
            Promise.resolve({ state: Notification.permission }) :
            originalQuery(parameters)
    );

    const getParameter = WebGLRenderingContext.prototype.getParameter;
    WebGLRenderingContext.prototype.getParameter = function(parameter) {
        if (parameter === 37445) return 'Intel Inc.';
        if (parameter === 37446) return 'Intel(R) Iris(TM) Graphics 6100';
        return getParameter.apply(this, arguments);
    };
})();
`

// ApplyStealth injects scripts and configures the browser to avoid bot detection.
func ApplyStealth(ctx context.Context, cfg domain.StealthConfig) error {
	if !cfg.Enabled {
		return nil
	}

	actions := []chromedp.Action{}

	if cfg.HideWebDriver {
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(ctx)
			return err
		}))
	}

	if cfg.UserAgent != "" {
		actions = append(actions, emulation.SetUserAgentOverride(cfg.UserAgent))
	}

	return chromedp.Run(ctx, actions...)
}
