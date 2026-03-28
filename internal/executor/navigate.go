// Package executor implements browser action executors for Aperture.
// Each executor handles a single action type and implements domain.Executor.
package executor

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// WaitStrategy controls how the navigate executor waits for page readiness.
type WaitStrategy string

const (
	// WaitLoad waits for DOMContentLoaded to fire.
	WaitLoad WaitStrategy = "load"

	// WaitNetworkIdle waits until there are no pending requests for 500 ms.
	WaitNetworkIdle WaitStrategy = "networkidle"

	// WaitSelector waits until a CSS selector is present in the DOM.
	WaitSelector WaitStrategy = "selector"
)

// defaultNavigateTimeout is used when no explicit timeout is set in params.
const defaultNavigateTimeout = 30 * time.Second

// NavigateExecutor navigates a browser instance to a URL and waits for
// page readiness according to a configurable wait strategy.
// It implements domain.Executor.
type NavigateExecutor struct {
	profileMgr domain.SiteProfileManager // optional; nil = no profile matching
	credVault  domain.CredentialVault    // optional; nil = no auto-login
}

// NewNavigateExecutor constructs a NavigateExecutor.
func NewNavigateExecutor(opts ...NavigateOption) *NavigateExecutor {
	e := &NavigateExecutor{}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// NavigateOption configures the NavigateExecutor.
type NavigateOption func(*NavigateExecutor)

// WithProfileManager sets the profile manager for site intelligence.
func WithProfileManager(pm domain.SiteProfileManager) NavigateOption {
	return func(e *NavigateExecutor) { e.profileMgr = pm }
}

// WithCredentialVault sets the credential vault for auto-login.
func WithCredentialVault(cv domain.CredentialVault) NavigateOption {
	return func(e *NavigateExecutor) { e.credVault = cv }
}

// Execute navigates to the URL in params["url"] and waits using params["wait"].
//
// Supported params:
//   - "url"      string (required) — target URL
//   - "wait"     string — wait strategy: "load" (default), "networkidle", "selector"
//   - "selector" string — CSS selector, required when wait="selector"
//   - "timeout"  time.Duration — override default 30 s timeout
//
// Returns a non-nil *ActionResult on both success and failure.
// Implements domain.Executor.
func (e *NavigateExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "navigate"}

	rawURL, err := stringParam(params, "url")
	if err != nil {
		return failResult(result, start, err), nil
	}

	timeout := defaultNavigateTimeout
	if v, ok := params["timeout"]; ok {
		if d, ok := v.(time.Duration); ok {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	wait := WaitLoad
	if v, ok := params["wait"]; ok {
		if s, ok := v.(string); ok {
			wait = WaitStrategy(s)
		}
	}

	selector, _ := params["selector"].(string)

	pageState, err := navigate(ctx, inst.Context(), rawURL, wait, selector)
	if err != nil {
		return failResult(result, start, fmt.Errorf("navigate: %w", err)), nil
	}

	// Auto-login: if credential vault has credentials for this domain with auto_login,
	// attempt to fill login form and submit.
	if e.credVault != nil && pageState != nil {
		agentID, _ := params["agent_id"].(string)
		if agentID == "" {
			agentID, _ = params["session_id"].(string)
		}
		if agentID != "" {
			e.tryAutoLogin(ctx, inst, agentID, pageState)
		}
	}

	// Site profile matching: enrich response with structured data if a profile matches.
	if e.profileMgr != nil && pageState != nil {
		if match := e.profileMgr.Match(pageState.URL); match != nil {
			pageState.ProfileMatched = match.ProfileDomain
			pageState.AvailableActions = e.profileMgr.AvailableActions(match)
			if extracted, err := e.profileMgr.Extract(ctx, match, inst); err == nil && len(extracted) > 0 {
				pageState.StructuredData = extracted
			}
		}
	}

	result.Success = true
	result.PageState = pageState
	result.Duration = time.Since(start)
	return result, nil
}

// makeRunContext creates a child of browserCtx that inherits the deadline from
// ctx, ensuring chromedp operations are cancelled when the caller's timeout fires.
func makeRunContext(ctx, browserCtx context.Context) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(browserCtx, deadline)
	}
	return context.WithCancel(browserCtx)
}

// navigate performs the actual CDP navigation and waits for the chosen strategy.
func navigate(
	ctx context.Context,
	browserCtx context.Context,
	rawURL string,
	wait WaitStrategy,
	selector string,
) (*domain.PageState, error) {
	var (
		finalURL   string
		title      string
		statusCode int64
	)

	runCtx, cancelRun := makeRunContext(ctx, browserCtx)
	defer cancelRun()

	listenCtx, stopListen := context.WithCancel(browserCtx)
	defer stopListen()

	chromedp.ListenTarget(listenCtx, func(ev interface{}) {
		if e, ok := ev.(*network.EventResponseReceived); ok {
			if e.Type == "Document" {
				statusCode = int64(e.Response.Status)
			}
		}
	})

	actions := buildNavigateActions(rawURL, wait, selector, &finalURL, &title)

	// Execute navigation with hard timeout enforcement.
	// chromedp.Run may not always respect context cancellation for certain
	// page load states (e.g., infinite JS loading, CAPTCHA walls).
	// Use a goroutine + select to enforce the deadline.
	type navResult struct {
		err error
	}
	navCh := make(chan navResult, 1)
	go func() {
		navCh <- navResult{err: chromedp.Run(runCtx, actions...)}
	}()

	select {
	case nr := <-navCh:
		if nr.err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("timeout waiting for page: %w", ctx.Err())
			}
			return nil, nr.err
		}
	case <-ctx.Done():
		cancelRun() // Force cancel the chromedp context
		return nil, fmt.Errorf("page load error net::ERR_TIMED_OUT (hard timeout)")
	}

	return &domain.PageState{
		URL:        finalURL,
		Title:      title,
		StatusCode: int(statusCode),
	}, nil
}

// buildNavigateActions assembles the chromedp action sequence for a navigation.
func buildNavigateActions(
	rawURL string,
	wait WaitStrategy,
	selector string,
	outURL *string,
	outTitle *string,
) []chromedp.Action {
	actions := []chromedp.Action{
		chromedp.Navigate(rawURL),
	}

	switch wait {
	case WaitNetworkIdle:
		actions = append(actions, chromedp.WaitReady("body", chromedp.ByQuery))
		actions = append(actions, networkIdleAction())
	case WaitSelector:
		if selector != "" {
			actions = append(actions, chromedp.WaitVisible(selector, chromedp.ByQuery))
		}
	default: // WaitLoad
		actions = append(actions, chromedp.WaitReady("body", chromedp.ByQuery))
	}

	actions = append(actions,
		chromedp.Location(outURL),
		chromedp.Title(outTitle),
	)

	return actions
}

// networkIdleAction returns a chromedp.Action that sleeps briefly to approximate
// network idle (no new requests for ~500 ms). A full network idle monitor would
// require a dedicated listener; this provides a reasonable approximation.
func networkIdleAction() chromedp.Action {
	return chromedp.Sleep(500 * time.Millisecond)
}

// stringParam extracts a required string from params.
func stringParam(params map[string]interface{}, key string) (string, error) {
	v, ok := params[key]
	if !ok {
		return "", fmt.Errorf("missing required param %q", key)
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("param %q must be a non-empty string", key)
	}
	return s, nil
}

// failResult stamps Duration and Error on result then returns it.
func failResult(result *domain.ActionResult, start time.Time, err error) *domain.ActionResult {
	result.Success = false
	result.Error = err.Error()
	result.Duration = time.Since(start)
	return result
}

// tryAutoLogin checks if credentials exist for the navigated domain and
// attempts to fill and submit a login form. Best-effort: errors are logged
// but never propagated to the caller.
func (e *NavigateExecutor) tryAutoLogin(ctx context.Context, inst domain.BrowserInstance, agentID string, pageState *domain.PageState) {
	if pageState.URL == "" {
		return
	}

	// Extract domain from URL.
	pageDomain := ""
	if u, err := url.Parse(pageState.URL); err == nil {
		pageDomain = u.Hostname()
	}
	if pageDomain == "" {
		return
	}

	cred, err := e.credVault.Get(ctx, agentID, pageDomain)
	if err != nil || cred == nil || !cred.AutoLogin {
		return
	}

	browserCtx := inst.Context()

	// Check if there's a password field (login form indicator).
	var hasPassword bool
	runCtx, cancel := context.WithCancel(browserCtx)
	_ = chromedp.Run(runCtx,
		chromedp.Evaluate(`!!document.querySelector('input[type=password]')`, &hasPassword),
	)
	cancel()

	if !hasPassword {
		return
	}

	// Fill username (try common selectors).
	usernameSelectors := []string{
		"input[type=email]",
		"input[name=email]",
		"input[name=username]",
		"input[type=text]",
	}
	for _, sel := range usernameSelectors {
		runCtx2, cancel2 := context.WithCancel(browserCtx)
		err := chromedp.Run(runCtx2,
			chromedp.SendKeys(sel, cred.Username, chromedp.ByQuery),
		)
		cancel2()
		if err == nil {
			break
		}
	}

	// Fill password.
	runCtx3, cancel3 := context.WithCancel(browserCtx)
	_ = chromedp.Run(runCtx3,
		chromedp.SendKeys("input[type=password]", cred.Password, chromedp.ByQuery),
	)
	cancel3()

	// Submit: try button[type=submit], then Enter key.
	runCtx4, cancel4 := context.WithCancel(browserCtx)
	err = chromedp.Run(runCtx4,
		chromedp.Click("button[type=submit]", chromedp.ByQuery),
	)
	cancel4()
	if err != nil {
		runCtx5, cancel5 := context.WithCancel(browserCtx)
		_ = chromedp.Run(runCtx5,
			chromedp.SendKeys("input[type=password]", "\r", chromedp.ByQuery),
		)
		cancel5()
	}

	// Wait for navigation.
	runCtx6, cancel6 := context.WithCancel(browserCtx)
	_ = chromedp.Run(runCtx6, chromedp.Sleep(500*time.Millisecond))
	cancel6()

	// Update page state after login.
	runCtx7, cancel7 := context.WithCancel(browserCtx)
	var newURL, newTitle string
	_ = chromedp.Run(runCtx7,
		chromedp.Location(&newURL),
		chromedp.Title(&newTitle),
	)
	cancel7()
	if newURL != "" {
		pageState.URL = newURL
	}
	if newTitle != "" {
		pageState.Title = newTitle
	}
}

