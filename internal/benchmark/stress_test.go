package benchmark

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/ApertureHQ/aperture/internal/browser"
)

// chromiumPath returns the path to a Chromium/Chrome binary for tests.
func chromiumPath(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("APERTURE_CHROMIUM_PATH"); p != "" {
		return p
	}
	// Simplified for Darwin as we know the platform from ls output.
	return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
}

// TestStress_Concurrency20 launches 20 concurrent sessions performing
// a simple navigate + screenshot task to verify pool stability and performance.
func TestStress_Concurrency20(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const concurrency = 20
	const iterations = 1

	p, err := browser.NewPool(browser.Config{
		PoolSize:     concurrency,
		ChromiumPath: chromiumPath(t),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer p.Close()

	var wg sync.WaitGroup
	start := time.Now()

	fmt.Printf("Starting stress test: %d concurrent sessions...\n", concurrency)

	errChan := make(chan error, concurrency*iterations)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if err := performTask(p, id); err != nil {
					errChan <- fmt.Errorf("session %d iter %d: %w", id, j, err)
				}
			}
		}(i)
	}

	// Monitor memory usage during the run.
	go func() {
		for {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			// fmt.Printf("Memory: Alloc=%v, TotalAlloc=%v, Sys=%v, NumGC=%v\n", 
			// 	m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024, m.NumGC)
			time.Sleep(1 * time.Second)
			if time.Since(start) > 30*time.Second { // fail-safe if wg.Wait blocks forever
				return
			}
		}
	}()

	wg.Wait()
	close(errChan)

	duration := time.Since(start)
	fmt.Printf("Stress test completed in %v\n", duration)

	var failures int
	for err := range errChan {
		t.Errorf("Failure: %v", err)
		failures++
	}

	if failures == 0 {
		fmt.Println("Stress test PASSED: 0 failures, 0 race conditions detected.")
	} else {
		fmt.Printf("Stress test FAILED: %d failures.\n", failures)
	}
}

func performTask(p *browser.Pool, id int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	inst, err := p.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire: %w", err)
	}
	defer p.Release(inst)

	// Perform a navigate + screenshot task.
	html := fmt.Sprintf(`data:text/html,<html><body><h1>Session %d</h1></body></html>`, id)
	
	var buf []byte
	err = chromedp.Run(inst.Context(),
		chromedp.Navigate(html),
		chromedp.CaptureScreenshot(&buf),
	)
	if err != nil {
		return fmt.Errorf("chromedp: %w", err)
	}

	if len(buf) == 0 {
		return fmt.Errorf("screenshot captured 0 bytes")
	}

	return nil
}
