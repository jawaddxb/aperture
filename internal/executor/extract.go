// Package executor implements browser action executors for Aperture.
// This file provides ExtractExecutor, which extracts structured data from page
// content using an LLM backend.
package executor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/chromedp"
)

// extractDefaultFormat is used when the "format" param is absent.
const extractDefaultFormat = "json"

// extractSystemPrompt is prepended to the LLM prompt for all extractions.
const extractSystemPrompt = `You are a data extraction assistant. Extract information from the provided page content according to the schema/instructions below. Return ONLY the extracted data in the requested format with no preamble or explanation.`

// ExtractExecutor extracts structured data from the current page using an LLM.
// It implements domain.Executor.
type ExtractExecutor struct {
	llm domain.LLMClient
}

// NewExtractExecutor constructs an ExtractExecutor backed by the given LLMClient.
func NewExtractExecutor(llm domain.LLMClient) *ExtractExecutor {
	return &ExtractExecutor{llm: llm}
}

// Execute extracts structured data from the current page.
//
// Supported params:
//   - "schema"   string — JSON schema or descriptive prompt for the extraction (required)
//   - "format"   string — desired output format: "json" (default) or "markdown"
//   - "selector" string — optional CSS selector to scope extraction to a sub-tree
//   - "timeout"  time.Duration — override default action timeout
//
// On success, result.Data contains the extracted string (JSON or Markdown).
// Implements domain.Executor.
func (e *ExtractExecutor) Execute(
	ctx context.Context,
	inst domain.BrowserInstance,
	params map[string]interface{},
) (*domain.ActionResult, error) {
	start := time.Now()
	result := &domain.ActionResult{Action: "extract"}

	schema, err := stringParam(params, "schema")
	if err != nil {
		return failResult(result, start, fmt.Errorf("extract: %w", err)), nil
	}

	format := extractDefaultFormat
	if v, ok := params["format"].(string); ok && v != "" {
		format = v
	}

	selector, _ := params["selector"].(string)

	ctx, cancel := context.WithTimeout(ctx, resolveTimeout(params))
	defer cancel()

	content, err := fetchPageContent(ctx, inst, selector)
	if err != nil {
		return failResult(result, start, fmt.Errorf("extract: fetch content: %w", err)), nil
	}

	prompt := buildExtractionPrompt(schema, format, content)

	extracted, err := e.llm.Complete(ctx, prompt)
	if err != nil {
		return failResult(result, start, fmt.Errorf("extract: llm: %w", err)), nil
	}

	result.Success = true
	result.Data = []byte(extracted)
	result.Duration = time.Since(start)
	return result, nil
}

// fetchPageContent retrieves text content from the page.
// When selector is non-empty, only the text within that element is returned.
// Falls back to document.body.innerText when the selector yields no content.
func fetchPageContent(ctx context.Context, inst domain.BrowserInstance, selector string) (string, error) {
	var content string
	script := buildContentScript(selector)

	runCtx, cancelRun := makeRunContext(ctx, inst.Context())
	defer cancelRun()

	if err := chromedp.Run(runCtx, chromedp.Evaluate(script, &content)); err != nil {
		return "", err
	}
	return content, nil
}

// buildContentScript returns the JS expression to extract page text.
func buildContentScript(selector string) string {
	if strings.TrimSpace(selector) == "" {
		return `document.body ? document.body.innerText : ""`
	}
	return fmt.Sprintf(
		`(function(){var el=document.querySelector(%q);return el?el.innerText:document.body.innerText;})()`,
		selector,
	)
}

// buildExtractionPrompt assembles the full LLM prompt from schema, format, and content.
func buildExtractionPrompt(schema, format, content string) string {
	return fmt.Sprintf(
		"%s\n\nSchema/Instructions:\n%s\n\nOutput format: %s\n\nPage content:\n%s",
		extractSystemPrompt,
		schema,
		format,
		content,
	)
}
