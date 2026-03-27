// Package planner provides Planner implementations for decomposing natural-language
// goals into ordered sequences of concrete browser executor steps.
// This file provides the PageContext builder that assembles page metadata for prompts.
package planner

import (
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
)

const (
	// axSummaryMaxChars is the maximum characters allowed for the AX tree summary
	// in a prompt. Content beyond this limit is truncated.
	axSummaryMaxChars = 2000
)

// BuildPageContext assembles a human-readable context block from the current
// page state, accessibility tree summary, and optional vision description.
// Any component may be empty; callers receive whatever is available.
func BuildPageContext(pageState *domain.PageState, axSummary string, visionDesc string) string {
	var sb strings.Builder

	if pageState != nil {
		if pageState.URL != "" {
			sb.WriteString("URL: ")
			sb.WriteString(pageState.URL)
			sb.WriteString("\n")
		}
		if pageState.Title != "" {
			sb.WriteString("Title: ")
			sb.WriteString(pageState.Title)
			sb.WriteString("\n")
		}
	}

	truncated := truncateAXSummary(axSummary)
	if truncated != "" {
		sb.WriteString("Accessibility tree (interactive elements):\n")
		sb.WriteString(truncated)
		sb.WriteString("\n")
	}

	if visionDesc != "" {
		sb.WriteString("Visual description:\n")
		sb.WriteString(visionDesc)
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}

// truncateAXSummary returns the AX tree summary capped at axSummaryMaxChars.
// When truncation occurs it appends a note so the LLM knows context is incomplete.
func truncateAXSummary(summary string) string {
	if len(summary) <= axSummaryMaxChars {
		return summary
	}
	return summary[:axSummaryMaxChars] + "\n[...truncated]"
}
