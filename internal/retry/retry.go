// Package retry provides shared retry, backoff, and rate-limiting logic.
package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultMaxRetries is the default number of retry attempts.
	DefaultMaxRetries = 3

	// DefaultBaseDelay is the default initial backoff delay.
	DefaultBaseDelay = time.Second

	// DefaultMaxDelay is the default maximum backoff delay.
	DefaultMaxDelay = 30 * time.Second

	backoffExponent = 2
	jitterDivisor   = 4
)

// Config controls retry behavior.
type Config struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	RateLimit  int           // tokens per window
	RateWindow time.Duration // window duration
}

// IsRetryableFunc classifies whether an error is transient.
type IsRetryableFunc func(error) bool

// PermanentError wraps an error to signal that it should not be retried,
// regardless of what the IsRetryableFunc says. Use this for errors like
// failed reconnections that should immediately abort the retry loop.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// Limiter implements a token-bucket rate limiter with a sliding window.
type Limiter struct {
	mu        sync.Mutex
	tokens    int
	limit     int
	window    time.Duration
	lastReset time.Time
}

// NewLimiter creates a rate limiter with the given token limit and window duration.
func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{
		tokens:    limit,
		limit:     limit,
		window:    window,
		lastReset: time.Now(),
	}
}

// WaitForToken blocks until a rate limit token is available or ctx is canceled.
func WaitForToken(ctx context.Context, lim *Limiter) error {
	if lim == nil {
		return nil
	}

	for {
		lim.mu.Lock()

		now := time.Now()
		if now.Sub(lim.lastReset) >= lim.window {
			lim.tokens = lim.limit
			lim.lastReset = now
		}

		if lim.tokens > 0 {
			lim.tokens--
			lim.mu.Unlock()

			return nil
		}

		resetAt := lim.lastReset.Add(lim.window)
		lim.mu.Unlock()

		wait := time.Until(resetAt)
		if wait <= 0 {
			continue
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled waiting for rate limit token: %w", ctx.Err())
		case <-time.After(wait):
		}
	}
}

// Info returns current rate limit state: remaining tokens, total limit, and next reset time.
func (l *Limiter) Info() (int, int, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if now.Sub(l.lastReset) >= l.window {
		l.tokens = l.limit
		l.lastReset = now
	}

	return l.tokens, l.limit, l.lastReset.Add(l.window)
}

// WithRetry executes op with rate limiting, exponential backoff, and jitter.
// The isRetryable function determines whether a failed attempt should be retried.
// If lim is non-nil, a rate limit token is acquired before each attempt.
func WithRetry(
	ctx context.Context,
	cfg Config,
	lim *Limiter,
	isRetryable IsRetryableFunc,
	op func() error,
) error {
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		err := WaitForToken(ctx, lim)
		if err != nil {
			return err
		}

		lastErr = op()
		if lastErr == nil {
			return nil
		}

		// PermanentError short-circuits all retries regardless of classifier.
		var permErr *PermanentError
		if errors.As(lastErr, &permErr) {
			return permErr.Err
		}

		if !isRetryable(lastErr) {
			return lastErr
		}

		if attempt < cfg.MaxRetries {
			backoff := time.Duration(
				float64(cfg.BaseDelay) * math.Pow(backoffExponent, float64(attempt)),
			)
			backoff = min(backoff, cfg.MaxDelay)
			jitter := time.Duration(rand.Int64N(int64(backoff / jitterDivisor)))

			select {
			case <-ctx.Done():
				return fmt.Errorf("context canceled during retry backoff: %w", ctx.Err())
			case <-time.After(backoff + jitter):
			}
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// IsRetryable returns true for common transient network errors.
// Protocol-specific checks (SMTP 4xx, IMAP auth) should be combined with this
// in a wrapper that also checks protocol-specific conditions.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	errStr := err.Error()

	return strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "timed out") ||
		strings.Contains(errStr, "temporary") ||
		strings.Contains(errStr, "try again") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection refused")
}
