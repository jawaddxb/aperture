package captcha

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// Injector implements domain.CaptchaInjector by running JavaScript in the page.
type Injector struct{}

// NewInjector creates a new CAPTCHA solution injector.
func NewInjector() *Injector {
	return &Injector{}
}

// Inject applies the CAPTCHA solution to the page.
// For token-based CAPTCHAs (reCAPTCHA, hCaptcha, Turnstile), it sets the
// hidden response input and triggers the callback.
func (i *Injector) Inject(ctx context.Context, inst domain.BrowserInstance, sol *domain.CaptchaSolution) error {
	bctx := inst.Context()
	if bctx == nil {
		return fmt.Errorf("captcha injector: no browser context")
	}

	if sol.Token != "" {
		script := fmt.Sprintf(`
		(() => {
			const token = %q;

			// reCAPTCHA v2/v3
			const gResponse = document.getElementById('g-recaptcha-response');
			if (gResponse) {
				gResponse.style.display = 'block';
				gResponse.value = token;
				// Trigger callback if available
				if (typeof ___grecaptcha_cfg !== 'undefined') {
					const clients = ___grecaptcha_cfg.clients || {};
					for (const key of Object.keys(clients)) {
						const client = clients[key];
						if (client && typeof client === 'object') {
							for (const prop of Object.keys(client)) {
								const val = client[prop];
								if (val && typeof val === 'object' && typeof val.callback === 'function') {
									val.callback(token);
									return 'injected_recaptcha';
								}
							}
						}
					}
				}
				// Fallback: try window.grecaptcha callback
				if (window.grecaptcha && window.grecaptcha.getResponse) {
					return 'injected_recaptcha_no_callback';
				}
				return 'injected_recaptcha_input';
			}

			// hCaptcha
			const hResponse = document.querySelector('[name="h-captcha-response"]');
			if (hResponse) {
				hResponse.value = token;
				// Trigger hCaptcha callback
				if (window.hcaptcha) {
					const iframes = document.querySelectorAll('iframe[data-hcaptcha-widget-id]');
					if (iframes.length > 0) {
						const widgetId = iframes[0].getAttribute('data-hcaptcha-widget-id');
						// hCaptcha doesn't expose a direct callback, but form submit should work
					}
				}
				return 'injected_hcaptcha';
			}

			// Cloudflare Turnstile
			const cfResponse = document.querySelector('input[name="cf-turnstile-response"]');
			if (cfResponse) {
				cfResponse.value = token;
				return 'injected_turnstile';
			}

			return 'no_target_found';
		})()
		`, sol.Token)

		var result string
		if err := chromedp.Run(bctx, chromedp.Evaluate(script, &result)); err != nil {
			return fmt.Errorf("captcha injector: evaluate: %w", err)
		}
	}

	return nil
}
