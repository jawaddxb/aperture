package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// challengeSelectors match known anti-bot challenge iframes.
var challengeSelectors = []string{
	`iframe[src*="challenges.cloudflare.com"]`,  // Cloudflare Turnstile
	`iframe[src*="geo.captcha-delivery.com"]`,   // DataDome
	`iframe[src*="captcha.px-cdn.net"]`,          // PerimeterX
	`#px-captcha`,                                 // PerimeterX press-and-hold
}

// interstitialTexts are texts shown on various challenge pages.
var interstitialTexts = []string{
	"Checking your browser",
	"Performing security verification",
	"Verify you are human",
	"challenges.cloudflare.com",
	"Press & Hold",            // PerimeterX
	"Please verify you are a human", // DataDome
	"Pardon Our Interruption", // Akamai
}

// WaitForTurnstile detects and waits for a Cloudflare Turnstile challenge to resolve.
// It returns nil if no challenge is detected or if the challenge resolves within timeout.
// Returns an error only if the challenge is detected but fails to resolve.
func WaitForTurnstile(ctx context.Context, timeout time.Duration) error {
	detected, err := isTurnstilePresent(ctx)
	if err != nil || !detected {
		return err
	}

	deadline := time.After(timeout)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("turnstile: challenge did not resolve within %s", timeout)
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			gone, err := isTurnstileGone(ctx)
			if err != nil {
				return fmt.Errorf("turnstile: poll error: %w", err)
			}
			if gone {
				return nil
			}
		}
	}
}

// isTurnstilePresent checks if a Turnstile iframe or interstitial page exists.
func isTurnstilePresent(ctx context.Context) (bool, error) {
	var found bool
	// Build a JS check that looks for any challenge iframe OR known interstitial text.
	js := fmt.Sprintf(`(function(){
		%s
		var t = document.body ? document.body.innerText : '';
		return %s;
	})()`, buildSelectorChecks(), buildTextChecks())
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &found)); err != nil {
		return false, err
	}
	return found, nil
}

// buildSelectorChecks returns JS that checks for any challenge iframe/element.
func buildSelectorChecks() string {
	s := ""
	for _, sel := range challengeSelectors {
		s += fmt.Sprintf("if(document.querySelector('%s')) return true;\n\t\t", sel)
	}
	return s
}

// buildTextChecks returns a JS expression that checks for any interstitial text.
func buildTextChecks() string {
	s := ""
	for i, text := range interstitialTexts {
		if i > 0 {
			s += " || "
		}
		s += fmt.Sprintf("t.includes('%s')", text)
	}
	return s
}

// isTurnstileGone checks if the Turnstile challenge has resolved (iframe removed
// or page content changed away from the interstitial).
func isTurnstileGone(ctx context.Context) (bool, error) {
	present, err := isTurnstilePresent(ctx)
	if err != nil {
		return false, err
	}
	return !present, nil
}
