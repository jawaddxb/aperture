// Package domain defines core types for Aperture.
// This file provides CAPTCHA-related types and interfaces.
package domain

import "context"

// CaptchaType identifies the kind of CAPTCHA challenge.
type CaptchaType string

const (
	CaptchaRecaptchaV2 CaptchaType = "recaptcha_v2"
	CaptchaRecaptchaV3 CaptchaType = "recaptcha_v3"
	CaptchaHCaptcha    CaptchaType = "hcaptcha"
	CaptchaTurnstile   CaptchaType = "turnstile"
	CaptchaImageText   CaptchaType = "image_text"
)

// CaptchaChallenge describes a detected CAPTCHA on a page.
type CaptchaChallenge struct {
	Type    CaptchaType `json:"type"`
	SiteKey string      `json:"site_key,omitempty"`
	PageURL string      `json:"page_url"`
	Action  string      `json:"action,omitempty"` // reCAPTCHA v3 action
	Image   []byte      `json:"-"`                // For image-based CAPTCHAs
}

// CaptchaSolution is the result of solving a CAPTCHA.
type CaptchaSolution struct {
	Token    string `json:"token,omitempty"` // g-recaptcha-response / h-captcha-response / cf-turnstile-response
	Text     string `json:"text,omitempty"`  // For image text CAPTCHAs
	SolvedBy string `json:"solved_by"`       // "capsolver", "2captcha", "human"
}

// CaptchaSolver solves CAPTCHA challenges.
type CaptchaSolver interface {
	// Solve attempts to solve the given CAPTCHA challenge.
	Solve(ctx context.Context, challenge CaptchaChallenge) (*CaptchaSolution, error)
	// Name returns the solver's identifier.
	Name() string
}

// CaptchaDetector detects CAPTCHAs on a page.
type CaptchaDetector interface {
	// Detect checks the current page for CAPTCHA challenges.
	Detect(ctx context.Context, inst BrowserInstance) (*CaptchaChallenge, error)
}

// CaptchaInjector injects CAPTCHA solutions into the page.
type CaptchaInjector interface {
	// Inject applies the solution to the page and optionally submits.
	Inject(ctx context.Context, inst BrowserInstance, solution *CaptchaSolution) error
}
