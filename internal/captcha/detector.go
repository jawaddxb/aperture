// Package captcha provides CAPTCHA detection, solving, and injection for Aperture.
package captcha

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// Detector implements domain.CaptchaDetector using JavaScript page inspection.
type Detector struct{}

// NewDetector creates a new CAPTCHA detector.
func NewDetector() *Detector {
	return &Detector{}
}

// detectScript is injected into the page to find CAPTCHA elements.
const detectScript = `
(() => {
	// reCAPTCHA v2/v3
	const recaptchaElem = document.querySelector('.g-recaptcha, [data-sitekey]');
	if (recaptchaElem) {
		const siteKey = recaptchaElem.getAttribute('data-sitekey') || '';
		const isV3 = recaptchaElem.getAttribute('data-size') === 'invisible' || 
		             document.querySelector('script[src*="recaptcha/api.js?render="]') !== null;
		return JSON.stringify({
			type: isV3 ? 'recaptcha_v3' : 'recaptcha_v2',
			site_key: siteKey,
			page_url: window.location.href
		});
	}

	// hCaptcha
	const hcaptchaElem = document.querySelector('.h-captcha, [data-hcaptcha-widget-id]');
	if (hcaptchaElem) {
		const siteKey = hcaptchaElem.getAttribute('data-sitekey') || '';
		return JSON.stringify({
			type: 'hcaptcha',
			site_key: siteKey,
			page_url: window.location.href
		});
	}

	// Cloudflare Turnstile
	const turnstileElem = document.querySelector('.cf-turnstile, [data-turnstile-widget-id]');
	if (turnstileElem) {
		const siteKey = turnstileElem.getAttribute('data-sitekey') || '';
		return JSON.stringify({
			type: 'turnstile',
			site_key: siteKey,
			page_url: window.location.href
		});
	}

	// Check for Turnstile response element (already solved or in progress)
	const cfResponse = document.querySelector('input[name="cf-turnstile-response"]');
	if (cfResponse && cfResponse.value === '') {
		return JSON.stringify({
			type: 'turnstile',
			site_key: '',
			page_url: window.location.href
		});
	}

	// reCAPTCHA via script src (fallback detection)
	const recaptchaScript = document.querySelector('script[src*="recaptcha/api"]');
	if (recaptchaScript) {
		const src = recaptchaScript.getAttribute('src') || '';
		const renderMatch = src.match(/render=([^&]+)/);
		const siteKey = renderMatch ? renderMatch[1] : '';
		if (siteKey && siteKey !== 'explicit') {
			return JSON.stringify({
				type: 'recaptcha_v3',
				site_key: siteKey,
				page_url: window.location.href
			});
		}
	}

	return '';
})()
`

// Detect checks the current page for CAPTCHA challenges.
// Returns nil if no CAPTCHA is found.
func (d *Detector) Detect(ctx context.Context, inst domain.BrowserInstance) (*domain.CaptchaChallenge, error) {
	bctx := inst.Context()
	if bctx == nil {
		return nil, fmt.Errorf("captcha detector: no browser context")
	}

	var result string
	if err := chromedp.Run(bctx, chromedp.Evaluate(detectScript, &result)); err != nil {
		return nil, fmt.Errorf("captcha detector: evaluate: %w", err)
	}

	if result == "" {
		return nil, nil // No CAPTCHA found
	}

	var challenge domain.CaptchaChallenge
	if err := json.Unmarshal([]byte(result), &challenge); err != nil {
		return nil, fmt.Errorf("captcha detector: parse: %w", err)
	}

	return &challenge, nil
}
