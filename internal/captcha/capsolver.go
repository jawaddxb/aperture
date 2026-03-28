package captcha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
)

const capsolverBaseURL = "https://api.capsolver.com"

// CapSolverClient solves CAPTCHAs using the CapSolver API.
type CapSolverClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewCapSolverClient creates a new CapSolver client.
func NewCapSolverClient(apiKey string) *CapSolverClient {
	return &CapSolverClient{
		apiKey:  apiKey,
		baseURL: capsolverBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *CapSolverClient) Name() string { return "capsolver" }

// Solve sends a CAPTCHA challenge to CapSolver and polls for the result.
func (c *CapSolverClient) Solve(ctx context.Context, ch domain.CaptchaChallenge) (*domain.CaptchaSolution, error) {
	taskType, err := c.taskType(ch)
	if err != nil {
		return nil, err
	}

	// Create task
	taskReq := map[string]interface{}{
		"clientKey": c.apiKey,
		"task": map[string]interface{}{
			"type":       taskType,
			"websiteURL": ch.PageURL,
			"websiteKey": ch.SiteKey,
		},
	}
	if ch.Action != "" {
		taskReq["task"].(map[string]interface{})["pageAction"] = ch.Action
	}

	body, err := c.post(ctx, "/createTask", taskReq)
	if err != nil {
		return nil, fmt.Errorf("capsolver: create task: %w", err)
	}

	taskID, ok := body["taskId"].(string)
	if !ok {
		return nil, fmt.Errorf("capsolver: no taskId in response: %v", body)
	}

	// Poll for result
	pollReq := map[string]interface{}{
		"clientKey": c.apiKey,
		"taskId":    taskID,
	}

	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}

		result, err := c.post(ctx, "/getTaskResult", pollReq)
		if err != nil {
			continue
		}

		status, _ := result["status"].(string)
		if status == "ready" {
			solution, _ := result["solution"].(map[string]interface{})
			token := ""
			if v, ok := solution["gRecaptchaResponse"].(string); ok {
				token = v
			} else if v, ok := solution["token"].(string); ok {
				token = v
			} else if v, ok := solution["text"].(string); ok {
				return &domain.CaptchaSolution{Text: v, SolvedBy: "capsolver"}, nil
			}
			return &domain.CaptchaSolution{Token: token, SolvedBy: "capsolver"}, nil
		}
		if status == "failed" {
			errMsg, _ := result["errorDescription"].(string)
			return nil, fmt.Errorf("capsolver: task failed: %s", errMsg)
		}
	}

	return nil, fmt.Errorf("capsolver: timeout after 120s")
}

func (c *CapSolverClient) taskType(ch domain.CaptchaChallenge) (string, error) {
	switch ch.Type {
	case domain.CaptchaRecaptchaV2:
		return "ReCaptchaV2TaskProxyLess", nil
	case domain.CaptchaRecaptchaV3:
		return "ReCaptchaV3TaskProxyLess", nil
	case domain.CaptchaHCaptcha:
		return "HCaptchaTaskProxyLess", nil
	case domain.CaptchaTurnstile:
		return "AntiTurnstileTaskProxyLess", nil
	case domain.CaptchaImageText:
		return "ImageToTextTask", nil
	default:
		return "", fmt.Errorf("unsupported captcha type: %s", ch.Type)
	}
}

func (c *CapSolverClient) post(ctx context.Context, path string, payload interface{}) (map[string]interface{}, error) {
	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("capsolver: parse response: %w", err)
	}

	errorID, _ := result["errorId"].(float64)
	if errorID != 0 {
		errMsg, _ := result["errorDescription"].(string)
		return nil, fmt.Errorf("capsolver API error: %s", errMsg)
	}

	return result, nil
}
