// Package api provides HTTP handlers and middleware for Aperture.
// This file implements a simple token-bucket rate limiter per client IP.
package api

import (
	"net/http"
	"sync"
	"time"
)

// RateLimitConfig holds settings for the rate limiter.
type RateLimitConfig struct {
	// RequestsPerMinute is the max requests per IP per minute. 0 = unlimited.
	RequestsPerMinute int
	// BurstSize is the max burst above the steady rate. Defaults to RPM/2.
	BurstSize int
}

type bucket struct {
	tokens    float64
	lastFill  time.Time
	ratePerMs float64
	max       float64
}

func (b *bucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(b.lastFill).Milliseconds()
	b.tokens += float64(elapsed) * b.ratePerMs
	if b.tokens > b.max {
		b.tokens = b.max
	}
	b.lastFill = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// RateLimit returns middleware that limits requests per IP.
// When cfg.RequestsPerMinute is 0, no limiting is applied.
func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	if cfg.RequestsPerMinute <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	burst := cfg.BurstSize
	if burst <= 0 {
		burst = cfg.RequestsPerMinute / 2
		if burst < 1 {
			burst = 1
		}
	}
	ratePerMs := float64(cfg.RequestsPerMinute) / 60000.0

	var mu sync.Mutex
	buckets := make(map[string]*bucket)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			mu.Lock()
			b, ok := buckets[ip]
			if !ok {
				b = &bucket{
					tokens:    float64(burst),
					lastFill:  time.Now(),
					ratePerMs: ratePerMs,
					max:       float64(burst),
				}
				buckets[ip] = b
			}
			allowed := b.allow()
			mu.Unlock()

			if !allowed {
				w.Header().Set("Retry-After", "60")
				writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
