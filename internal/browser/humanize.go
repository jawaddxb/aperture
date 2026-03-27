package browser

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// HumanMove simulates a human-like mouse movement from (sx,sy) to (ex,ey)
// using a cubic Bézier curve with random control points and micro-jitter.
// It dispatches ~20 intermediate mouseMoved events via CDP.
func HumanMove(ctx context.Context, sx, sy, ex, ey float64) error {
	steps := 18 + rand.Intn(8) // 18–25 steps
	cx1, cy1 := bezierControl(sx, sy, ex, ey)
	cx2, cy2 := bezierControl(ex, ey, sx, sy)

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x, y := cubicBezier(t, sx, sy, cx1, cy1, cx2, cy2, ex, ey)

		// Add micro-jitter (humans aren't pixel-perfect).
		x += rand.NormFloat64() * 1.2
		y += rand.NormFloat64() * 1.2

		if err := dispatchMove(ctx, x, y); err != nil {
			return err
		}

		// Variable delay — faster in the middle, slower at endpoints.
		delay := baseDelay(t)
		time.Sleep(delay)
	}

	return nil
}

// bezierControl generates a random control point offset from (ax,ay) toward (bx,by).
func bezierControl(ax, ay, bx, by float64) (float64, float64) {
	mx := (ax + bx) / 2
	my := (ay + by) / 2
	return mx + rand.NormFloat64()*40, my + rand.NormFloat64()*40
}

// cubicBezier evaluates the cubic Bézier at parameter t.
func cubicBezier(t, sx, sy, cx1, cy1, cx2, cy2, ex, ey float64) (float64, float64) {
	u := 1 - t
	x := u*u*u*sx + 3*u*u*t*cx1 + 3*u*t*t*cx2 + t*t*t*ex
	y := u*u*u*sy + 3*u*u*t*cy1 + 3*u*t*t*cy2 + t*t*t*ey
	return x, y
}

// baseDelay returns a sleep duration that simulates human timing:
// slower at start/end, faster in the middle.
func baseDelay(t float64) time.Duration {
	// Parabola: peaks at edges (t=0, t=1), lowest at t=0.5
	edge := 4 * math.Pow(t-0.5, 2) // 0..1, highest at edges
	ms := 6 + edge*18 + rand.NormFloat64()*2
	if ms < 2 {
		ms = 2
	}
	return time.Duration(ms) * time.Millisecond
}

// dispatchMove sends a single mouseMoved event via CDP.
func dispatchMove(ctx context.Context, x, y float64) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseMoved, x, y).Do(ctx)
	}))
}
