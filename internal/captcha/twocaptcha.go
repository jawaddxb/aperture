package captcha

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
)

const twoCaptchaBaseURL = "https://2captcha.com"

// TwoCaptchaClient solves CAPTCHAs using the 2Captcha API.
type TwoCaptchaClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewTwoCaptchaClient creates a new 2Captcha client.
func NewTwoCaptchaClient(apiKey string) *TwoCaptchaClient {
	return &TwoCaptchaClient{
		apiKey:  apiKey,
		baseURL: twoCaptchaBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *TwoCaptchaClient) Name() string { return "2captcha" }

// Solve submits a CAPTCHA to 2Captcha and polls for the result.
func (c *TwoCaptchaClient) Solve(ctx context.Context, ch domain.CaptchaChallenge) (*domain.CaptchaSolution, error) {
	params := url.Values{
		"key":     {c.apiKey},
		"json":    {"1"},
		"pageurl": {ch.PageURL},
	}

	switch ch.Type {
	case domain.CaptchaRecaptchaV2:
		params.Set("method", "userrecaptcha")
		params.Set("googlekey", ch.SiteKey)
	case domain.CaptchaRecaptchaV3:
		params.Set("method", "userrecaptcha")
		params.Set("googlekey", ch.SiteKey)
		params.Set("version", "v3")
		if ch.Action != "" {
			params.Set("action", ch.Action)
		}
	case domain.CaptchaHCaptcha:
		params.Set("method", "hcaptcha")
		params.Set("sitekey", ch.SiteKey)
	case domain.CaptchaTurnstile:
		params.Set("method", "turnstile")
		params.Set("sitekey", ch.SiteKey)
	case domain.CaptchaImageText:
		return nil, fmt.Errorf("2captcha: image text requires binary upload, not supported in this client")
	default:
		return nil, fmt.Errorf("2captcha: unsupported captcha type: %s", ch.Type)
	}

	// Submit task
	resp, err := c.http.PostForm(c.baseURL+"/in.php", params)
	if err != nil {
		return nil, fmt.Errorf("2captcha: submit: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	submitResult := string(body)

	// 2Captcha returns "OK|<taskId>" on success
	if !strings.HasPrefix(submitResult, "OK|") {
		return nil, fmt.Errorf("2captcha: submit error: %s", submitResult)
	}
	taskID := strings.TrimPrefix(submitResult, "OK|")

	// Poll for result (max 120s, 5s interval)
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}

		resp2, err := c.http.Get(fmt.Sprintf(
			"%s/res.php?key=%s&action=get&id=%s&json=1",
			c.baseURL, c.apiKey, taskID,
		))
		if err != nil {
			continue
		}
		defer resp2.Body.Close()
		body2, _ := io.ReadAll(resp2.Body)
		text := string(body2)

		if strings.HasPrefix(text, "OK|") {
			token := strings.TrimPrefix(text, "OK|")
			return &domain.CaptchaSolution{Token: token, SolvedBy: "2captcha"}, nil
		}
		if text == "CAPCHA_NOT_READY" || text == "ERROR_NOT_READY" {
			continue // still solving
		}
		return nil, fmt.Errorf("2captcha: error: %s", text)
	}

	return nil, fmt.Errorf("2captcha: timeout after 120s waiting for solution")
}
