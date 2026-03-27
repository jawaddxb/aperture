package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// turnstileSelector matches the Cloudflare Turnstile challenge iframe.
const turnstileSelector = `iframe[src*="challenges.cloudflare.com"]`

// interstitialText is the text shown on Cloudflare's "checking your browser" page.
const interstitialText = "Checking your browser"

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
	js := fmt.Sprintf(
		`!!document.querySelector('%s') || document.body.innerText.includes('%s')`,
		turnstileSelector, interstitialText,
	)
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &found)); err != nil {
		return false, err
	}
	return found, nil
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
