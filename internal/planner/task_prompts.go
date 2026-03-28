// Package planner provides Planner implementations for Aperture.
// This file defines multi-step task planning prompts for the StatefulTaskPlanner.
package planner

import (
	"fmt"
	"strings"
)

// taskSystemPrompt describes multi-step goal decomposition to the LLM.
const taskSystemPrompt = `You are Aperture, a browser automation assistant.
You decompose complex natural-language goals into ordered multi-step execution plans.

Available step actions:
  navigate  - target: URL string
  click     - target: CSS selector or descriptive label
  type      - target: CSS selector or label, text: string to type
  extract   - selector: CSS selector to scope extraction, fields: array of field names
  scroll    - direction: "up"|"down", amount: pixels (default 300)
  wait      - strategy: "selector"|"text"|"hidden"|"timeout"|"networkidle", value: string|int
  screenshot - no params

Your response MUST be a single JSON object (no prose, no markdown fences):
{
  "steps": [
    {
      "action": "<action>",
      "target": "<target if applicable>",
      "selector": "<CSS selector if applicable>",
      "fields": ["<field1>", "<field2>"],
      "text": "<text if type action>",
      "reasoning": "<one sentence>",
      "completion": "<condition that signals this step succeeded>"
    }
  ],
  "pagination_strategy": "click_next|url_param|scroll_load|none",
  "estimated_pages": 1
}

Rules:
  - Respond with ONLY the JSON object. No prose. No markdown.
  - Each step must have action and reasoning.
  - completion is a human-readable condition string (e.g. "URL contains /search", "data.length > 0").
  - For pagination_strategy: click_next = find/click Next button; url_param = increment page param; scroll_load = infinite scroll; none = single page.
  - estimated_pages: number of pages to process (1 if not paginating).`

// replanSystemPrompt is used when the page state is unexpected and we need to revise.
const replanSystemPrompt = `You are Aperture, a browser automation assistant.
The current browser state is unexpected (CAPTCHA, login wall, different layout, or error page).
You must produce a revised plan for the remaining steps to recover and continue toward the goal.

Your response MUST be a JSON object (no prose, no markdown fences):
{
  "steps": [
    {
      "action": "<action>",
      "target": "<target if applicable>",
      "selector": "<CSS selector if applicable>",
      "fields": ["<field1>", "<field2>"],
      "text": "<text if type action>",
      "reasoning": "<one sentence>",
      "completion": "<condition string>"
    }
  ],
  "pagination_strategy": "click_next|url_param|scroll_load|none",
  "estimated_pages": 1
}

Rules:
  - Respond with ONLY the JSON object. No prose. No markdown.
  - Focus on recovering from the unexpected state first, then continuing the goal.`

// BuildTaskPlanPrompt builds the initial planning prompt.
func BuildTaskPlanPrompt(goal string, pageContext string) string {
	var sb strings.Builder
	sb.WriteString(taskSystemPrompt)
	sb.WriteString("\n\n---\n\n")
	if pageContext != "" {
		sb.WriteString("Current page context:\n")
		sb.WriteString(pageContext)
		sb.WriteString("\n\n")
	}
	sb.WriteString(fmt.Sprintf("Goal: %s\n", goal))
	sb.WriteString("Produce the JSON plan.")
	return sb.String()
}

// BuildReplanPrompt builds a re-planning prompt given current progress and unexpected state.
func BuildReplanPrompt(goal string, completedSteps int, totalSteps int, pageContext string, unexpectedReason string) string {
	var sb strings.Builder
	sb.WriteString(replanSystemPrompt)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString(fmt.Sprintf("Original goal: %s\n", goal))
	sb.WriteString(fmt.Sprintf("Progress: completed %d of %d planned steps\n", completedSteps, totalSteps))
	if unexpectedReason != "" {
		sb.WriteString(fmt.Sprintf("Unexpected situation: %s\n", unexpectedReason))
	}
	if pageContext != "" {
		sb.WriteString("\nCurrent page context:\n")
		sb.WriteString(pageContext)
		sb.WriteString("\n")
	}
	sb.WriteString("\nProduce the revised JSON plan for remaining steps.")
	return sb.String()
}
