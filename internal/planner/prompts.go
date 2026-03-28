// Package planner provides Planner implementations for decomposing natural-language
// goals into ordered sequences of concrete browser executor steps.
// This file defines the prompt-engineering layer for the LLMPlanner.
package planner

import (
	"fmt"
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// systemPrompt describes Aperture's capabilities to the LLM.
const systemPrompt = `You are Aperture, a browser automation assistant.
You decompose natural-language goals into precise, ordered JSON action plans.

Available actions and their required params:
  navigate    url:string
  click       target:string (CSS selector or descriptive label)
  type        target:string, text:string
  screenshot  (no params; returns base64 PNG of current page)
  scroll      direction:"up"|"down", amount:int (pixels, default 300)
  select      target:string, value:string
  hover       target:string
  wait        strategy:string, value:string|int
              strategies: "selector" (CSS selector visible), "text" (text appears),
              "hidden" (selector disappears), "timeout" (ms), "networkidle"
  extract     schema:string (describe what to extract), format:"json"|"markdown"
              optional: selector:string (scope to sub-tree)
  upload      target:string (file input selector), path:string (local file path)
  pause       reason:string (pause for human intervention, e.g. CAPTCHA)
  new_tab     url:string (open URL in a new tab)
  switch_tab  tab_id:string (switch to a tab by ID)

Response format:
  A JSON array of step objects. Each step:
  {
    "action":    "<action name>",
    "params":    { <key: value pairs> },
    "reasoning": "<one sentence explaining why this step>",
    "optional":  false
  }

Rules:
  - Respond with ONLY the JSON array. No prose. No markdown fences.
  - Always include a reasoning string.
  - Set optional:true only for steps that are nice-to-have.
  - Prefer specific CSS selectors when you know the page structure.

Examples:
Goal: "Log in to GitHub with user alice and password s3cret"
[
  {"action":"navigate","params":{"url":"https://github.com/login"},"reasoning":"open login page","optional":false},
  {"action":"type","params":{"target":"input#login_field","text":"alice"},"reasoning":"enter username","optional":false},
  {"action":"type","params":{"target":"input#password","text":"s3cret"},"reasoning":"enter password","optional":false},
  {"action":"click","params":{"target":"input[type=submit]"},"reasoning":"submit the form","optional":false}
]

Goal: "Take a screenshot of the homepage"
[
  {"action":"navigate","params":{"url":"https://example.com"},"reasoning":"open the homepage","optional":false},
  {"action":"screenshot","params":{},"reasoning":"capture the current page","optional":false}
]`

// BuildUserPrompt constructs the user-turn prompt combining goal and page context.
// pageContext may be empty if no browser state is available.
func BuildUserPrompt(goal string, pageContext string) string {
	var sb strings.Builder
	if pageContext != "" {
		sb.WriteString("Current page context:\n")
		sb.WriteString(pageContext)
		sb.WriteString("\n\n")
	}
	sb.WriteString(fmt.Sprintf("Goal: %s\n", goal))
	sb.WriteString("Respond with only the JSON array of steps.")
	return sb.String()
}

// BuildFullPrompt combines the system prompt and user prompt into a single
// string that a text-only LLMClient can consume.
func BuildFullPrompt(goal string, pageState *domain.PageState, axSummary string, visionDesc string) string {
	ctx := BuildPageContext(pageState, axSummary, visionDesc)
	user := BuildUserPrompt(goal, ctx)

	var sb strings.Builder
	sb.WriteString(systemPrompt)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString(user)
	return sb.String()
}
