package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	browserpool "github.com/ApertureHQ/aperture/internal/browser"
	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/executor"
	"github.com/ApertureHQ/aperture/internal/llm"
	"github.com/ApertureHQ/aperture/internal/planner"
	"github.com/ApertureHQ/aperture/internal/resolver"
	"github.com/ApertureHQ/aperture/internal/sequencer"
	"github.com/ApertureHQ/aperture/internal/session"
)

// runSubcommand parses flags and executes a natural-language goal.
// Exit code 0 on success, 1 on failure.
func runSubcommand(args []string) error {
	g, goal, err := parseRunFlags(args)
	if err != nil {
		return err
	}
	configureLogger(g.verbose)

	pool, mgr, err := buildSessionManager(g)
	if err != nil {
		return err
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	return executeGoal(ctx, mgr, goal)
}

// parseRunFlags parses run subcommand flags and returns globalFlags + resolved goal string.
func parseRunFlags(args []string) (*globalFlags, string, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	g := parseGlobalFlags(fs)
	urlFlag := fs.String("url", "", "URL to navigate to before executing the goal (optional)")

	if err := fs.Parse(args); err != nil {
		return nil, "", err
	}
	if fs.NArg() < 1 {
		return nil, "", fmt.Errorf("usage: aperture run \"<goal>\" [--url <url>]")
	}

	goal := fs.Arg(0)
	if *urlFlag != "" {
		goal = "navigate to " + *urlFlag + " then " + goal
	}
	return g, goal, nil
}

// buildSessionManager wires up the browser pool and session manager from global flags.
func buildSessionManager(g *globalFlags) (*browserpool.Pool, *session.DefaultSessionManager, error) {
	chromePath, err := findChromium()
	if err != nil {
		return nil, nil, fmt.Errorf("chromium not found: %w", err)
	}
	pool, err := browserpool.NewPool(browserpool.Config{PoolSize: 1, ChromiumPath: chromePath})
	if err != nil {
		return nil, nil, fmt.Errorf("browser pool: %w", err)
	}
	llmClient, err := buildLLMClient(g)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("llm client: %w", err)
	}
	reg := buildRegistry(pool)
	seq := sequencer.NewDefaultSequencer(sequencer.Config{Registry: reg, Progress: printProgress})
	p := planner.NewLLMPlanner(llmClient)
	mgr := session.NewDefaultSessionManager(session.Config{Pool: pool, Planner: p, Sequencer: seq})
	return pool, mgr, nil
}

// executeGoal creates a session, runs it, prints the result and returns any error.
func executeGoal(ctx context.Context, mgr *session.DefaultSessionManager, goal string) error {
	sess, err := mgr.Create(ctx, goal)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	fmt.Printf("session %s created — goal: %s\n", sess.ID, goal)

	result, err := mgr.Execute(ctx, sess.ID)
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}
	printResult(result)

	if !result.Success {
		return fmt.Errorf("goal failed at step %d", result.FailedStep)
	}
	return nil
}

// printProgress prints a step result to stdout as it completes.
func printProgress(sr domain.StepResult) {
	status := "✓"
	if sr.Result != nil && !sr.Result.Success {
		status = "✗"
	}
	fmt.Printf("  [%d] %s %s (%s)\n", sr.Index+1, status, sr.Step.Action, sr.Duration)
}

// printResult prints a RunResult summary to stdout.
func printResult(r *domain.RunResult) {
	fmt.Printf("\nResult: ")
	if r.Success {
		fmt.Printf("SUCCESS (%d steps, %s)\n", len(r.Steps), r.Duration)
	} else {
		fmt.Printf("FAILED at step %d (%s)\n", r.FailedStep+1, r.Duration)
	}
}

// buildLLMClient constructs a domain.LLMClient from global flags.
func buildLLMClient(g *globalFlags) (domain.LLMClient, error) {
	if g.apiKey == "" {
		return nil, fmt.Errorf("API key required: set --api-key or APERTURE_API_KEY")
	}
	return llm.NewClient(llm.Config{
		Provider: g.model,
		APIKey:   g.apiKey,
	})
}

// buildRegistry constructs the default executor registry.
func buildRegistry(pool domain.BrowserPool) map[string]domain.Executor {
	res := resolver.NewUnifiedResolver()
	return map[string]domain.Executor{
		"navigate":   executor.NewNavigateExecutor(),
		"click":      executor.NewClickExecutor(res),
		"type":       executor.NewTypeExecutor(res),
		"screenshot": executor.NewScreenshotExecutor(),
		"scroll":     executor.NewScrollExecutor(),
		"hover":      executor.NewHoverExecutor(res),
		"select":     executor.NewSelectExecutor(res),
		"wait":       executor.NewWaitExecutor(),
	}
}

// configureLogger sets the default slog level based on verbose flag.
func configureLogger(verbose bool) {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}

// findChromium locates the Chrome/Chromium binary on the system.
func findChromium() (string, error) {
	candidates := []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium",
		"chromium-browser",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
		// check absolute path directly
		if strings.HasPrefix(c, "/") {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}
	return "", fmt.Errorf("none of %v found in PATH", candidates)
}
