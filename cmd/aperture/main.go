// Package main is the CLI entry point for the Aperture browser automation tool.
// Usage:
//
//	aperture run "goal" --url https://example.com
//	aperture screenshot --url https://example.com --output screenshot.png
//	aperture version
package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

// version is the current CLI version, injected at build time via -ldflags.
var version = "dev"

// globalFlags holds flags shared across all subcommands.
type globalFlags struct {
	model    string
	apiKey   string
	headless bool
	timeout  time.Duration
	verbose  bool
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	sub := os.Args[1]
	args := os.Args[2:]

	switch sub {
	case "run":
		if err := runSubcommand(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "screenshot":
		if err := screenshotSubcommand(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Printf("aperture %s\n", version)
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %q\n", sub)
		printUsage()
		os.Exit(1)
	}
}

// parseGlobalFlags parses flags shared by all subcommands from fs.
func parseGlobalFlags(fs *flag.FlagSet) *globalFlags {
	g := &globalFlags{}
	fs.StringVar(&g.model, "model", "openai", "LLM provider (openai|anthropic)")
	fs.StringVar(&g.apiKey, "api-key", os.Getenv("APERTURE_API_KEY"), "API key (or set APERTURE_API_KEY env var)")
	fs.BoolVar(&g.headless, "headless", true, "run browser in headless mode")
	fs.DurationVar(&g.timeout, "timeout", 60*time.Second, "overall operation timeout")
	fs.BoolVar(&g.verbose, "verbose", false, "enable debug logging")
	return g
}

// printUsage prints CLI usage to stderr.
func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: aperture <subcommand> [flags]

Subcommands:
  run        Execute a natural-language goal against a URL
  screenshot Take a screenshot of a URL
  version    Print the CLI version

Global flags (available on all subcommands):
  --model     LLM provider: openai (default) or anthropic
  --api-key   API key (defaults to APERTURE_API_KEY env var)
  --headless  Run browser headless (default true)
  --timeout   Overall operation timeout (default 60s)
  --verbose   Enable debug logging

Examples:
  aperture run "click the login button" --url https://example.com
  aperture screenshot --url https://example.com --output out.png
  aperture screenshot --url https://example.com --full-page --output full.png
  aperture version`)
}
