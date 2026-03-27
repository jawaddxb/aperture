package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	browserpool "github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/executor"
)

// screenshotFlags holds parsed options for the screenshot subcommand.
type screenshotFlags struct {
	global   *globalFlags
	url      string
	output   string
	fullPage bool
	selector string
}

// screenshotSubcommand navigates to a URL, captures a screenshot, and saves it.
func screenshotSubcommand(args []string) error {
	sf, err := parseScreenshotFlags(args)
	if err != nil {
		return err
	}
	configureLogger(sf.global.verbose)

	chromePath, err := findChromium()
	if err != nil {
		return fmt.Errorf("chromium not found: %w", err)
	}
	pool, err := browserpool.NewPool(browserpool.Config{PoolSize: 1, ChromiumPath: chromePath})
	if err != nil {
		return fmt.Errorf("browser pool: %w", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), sf.global.timeout)
	defer cancel()

	return captureScreenshot(ctx, pool, sf)
}

// parseScreenshotFlags parses screenshot subcommand flags.
func parseScreenshotFlags(args []string) (*screenshotFlags, error) {
	fs := flag.NewFlagSet("screenshot", flag.ContinueOnError)
	g := parseGlobalFlags(fs)
	urlFlag := fs.String("url", "", "URL to screenshot (required)")
	outputFlag := fs.String("output", "screenshot.png", "output file path")
	fullPageFlag := fs.Bool("full-page", false, "capture full scrollable page")
	selectorFlag := fs.String("selector", "", "CSS selector to clip screenshot to")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *urlFlag == "" {
		return nil, fmt.Errorf("--url is required")
	}
	return &screenshotFlags{
		global:   g,
		url:      *urlFlag,
		output:   *outputFlag,
		fullPage: *fullPageFlag,
		selector: *selectorFlag,
	}, nil
}

// captureScreenshot acquires a browser, navigates, screenshots, and writes the file.
func captureScreenshot(ctx context.Context, pool *browserpool.Pool, sf *screenshotFlags) error {
	inst, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire browser: %w", err)
	}
	defer pool.Release(inst)

	navExec := executor.NewNavigateExecutor()
	navResult, err := navExec.Execute(ctx, inst, map[string]interface{}{"url": sf.url})
	if err != nil {
		return fmt.Errorf("navigate: %w", err)
	}
	if !navResult.Success {
		return fmt.Errorf("navigate failed: %s", navResult.Error)
	}

	params := buildScreenshotParams(sf.fullPage, sf.selector)
	ssExec := executor.NewScreenshotExecutor()
	result, err := ssExec.Execute(ctx, inst, params)
	if err != nil {
		return fmt.Errorf("screenshot: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("screenshot failed: %s", result.Error)
	}

	if err := os.WriteFile(sf.output, result.Data, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	fmt.Printf("screenshot saved to %s (%d bytes)\n", sf.output, len(result.Data))
	return nil
}

// buildScreenshotParams constructs executor params from CLI flags.
func buildScreenshotParams(fullPage bool, selector string) map[string]interface{} {
	params := map[string]interface{}{
		"fullPage": fullPage,
		"format":   "png",
	}
	if selector != "" {
		params["selector"] = selector
	}
	return params
}
