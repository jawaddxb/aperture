// Package domain defines core interfaces for Aperture.
// This file defines the ExtractionRequest and ExtractionResult types used by
// the data extraction engine.
package domain

// ExtractionRequest is the input to an LLM-backed extraction call.
type ExtractionRequest struct {
	// Content is the raw page text or AX tree content to extract from.
	Content string

	// Schema is the JSON schema or descriptive prompt that describes the desired output.
	Schema string

	// Format is the desired output format: "json" or "markdown".
	Format string
}

// ExtractionResult holds the output of a successful extraction call.
type ExtractionResult struct {
	// Data is the extracted content, serialised as the requested format.
	Data string

	// Format echoes the requested format ("json" or "markdown").
	Format string
}
