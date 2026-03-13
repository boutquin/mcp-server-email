package retry_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/boutquin/mcp-server-email/internal/retry"
)

// --- Test sentinel errors (err113) ---

var (
	errConnReset = errors.New("connection reset by peer")
	errAuthFail  = errors.New("authentication failed")
	errTimeout   = errors.New("operation timed out")
	errTemporary = errors.New("temporary failure")
	errTryAgain  = errors.New("try again later")
	errBroken    = errors.New("broken pipe")
	errRefused   = errors.New("connection refused")
)

// testCfg returns a Config suitable for fast tests (no real delays).
func testCfg() retry.Config {
	return retry.Config{
		MaxRetries: 3,
		BaseDelay:  time.Millisecond, // fast for tests
		MaxDelay:   10 * time.Millisecond,
	}
}

// --- WithRetry tests ---

func TestWithRetry_SuccessFirstAttempt(t *testing.T) {
	t.Parallel()

	var attempts int

	err := retry.WithRetry(t.Context(), testCfg(), nil, retry.IsRetryable, func() error {
		attempts++

		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestWithRetry_TransientThenSuccess(t *testing.T) {
	t.Parallel()

	var attempts int

	err := retry.WithRetry(t.Context(), testCfg(), nil, retry.IsRetryable, func() error {
		attempts++
		if attempts == 1 {
			return errConnReset
		}

		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error after retry, got: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestWithRetry_PermanentError(t *testing.T) {
	t.Parallel()

	var attempts int

	err := retry.WithRetry(t.Context(), testCfg(), nil, retry.IsRetryable, func() error {
		attempts++

		return errAuthFail
	})
	if err == nil {
		t.Fatal("expected error for permanent failure")
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry for permanent error), got %d", attempts)
	}

	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("expected original error, got: %v", err)
	}
}

func TestWithRetry_MaxRetriesExceeded(t *testing.T) {
	t.Parallel()

	var attempts int

	err := retry.WithRetry(t.Context(), testCfg(), nil, retry.IsRetryable, func() error {
		attempts++

		return errConnReset
	})
	if err == nil {
		t.Fatal("expected error after max retries")
	}

	// maxRetries=3: attempts = initial(0) + 3 retries = 4 total
	if attempts != 4 {
		t.Errorf("expected 4 attempts (initial + 3 retries), got %d", attempts)
	}

	if !strings.Contains(err.Error(), "max retries exceeded") {
		t.Errorf("expected 'max retries exceeded', got: %v", err)
	}

	if !strings.Contains(err.Error(), "connection reset") {
		t.Errorf("expected original error wrapped, got: %v", err)
	}
}

func TestWithRetry_ContextCancelled(t *testing.T) {
	t.Parallel()

	cfg := retry.Config{
		MaxRetries: 3,
		BaseDelay:  time.Second, // long enough for cancel to fire during backoff
		MaxDelay:   30 * time.Second,
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var attempts int

	err := retry.WithRetry(ctx, cfg, nil, retry.IsRetryable, func() error {
		attempts++
		if attempts == 1 {
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()

			return errConnReset
		}

		return nil
	})
	if err == nil {
		t.Fatal("expected error when context cancelled during backoff")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt before cancel, got %d", attempts)
	}
}

func TestWithRetry_BackoffGrowsExponentially(t *testing.T) {
	t.Parallel()

	cfg := retry.Config{
		MaxRetries: 3,
		BaseDelay:  50 * time.Millisecond,
		MaxDelay:   5 * time.Second,
	}

	var timestamps []time.Time

	_ = retry.WithRetry(t.Context(), cfg, nil, retry.IsRetryable, func() error {
		timestamps = append(timestamps, time.Now())

		return errConnReset
	})

	if len(timestamps) < 3 {
		t.Fatalf("expected at least 3 timestamps, got %d", len(timestamps))
	}

	// Verify delays grow: gap2 > gap1 (approximately, allowing for jitter)
	gap1 := timestamps[1].Sub(timestamps[0])
	gap2 := timestamps[2].Sub(timestamps[1])

	// gap2 should be roughly 2x gap1 (exponential backoff)
	// Allow tolerance for jitter and scheduling
	if gap2 < gap1 {
		t.Errorf("expected exponential growth: gap1=%v, gap2=%v", gap1, gap2)
	}
}

func TestWithRetry_JitterApplied(t *testing.T) {
	t.Parallel()

	cfg := retry.Config{
		MaxRetries: 2,
		BaseDelay:  50 * time.Millisecond,
		MaxDelay:   5 * time.Second,
	}

	// Run multiple times and collect first-retry delays
	var delays []time.Duration

	for range 10 {
		var timestamps []time.Time

		_ = retry.WithRetry(t.Context(), cfg, nil, retry.IsRetryable, func() error {
			timestamps = append(timestamps, time.Now())

			return errConnReset
		})

		if len(timestamps) >= 2 {
			delays = append(delays, timestamps[1].Sub(timestamps[0]))
		}
	}

	if len(delays) < 5 {
		t.Fatalf("expected at least 5 delay samples, got %d", len(delays))
	}

	// Check that not all delays are identical (jitter makes them vary)
	allSame := true

	for i := 1; i < len(delays); i++ {
		// Allow 1ms tolerance for timer resolution
		if absDuration(delays[i]-delays[0]) > time.Millisecond {
			allSame = false

			break
		}
	}

	if allSame {
		t.Error("all retry delays were identical — jitter not applied")
	}
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}

	return d
}

// --- WaitForToken tests ---

func TestWaitForToken_Available(t *testing.T) {
	t.Parallel()

	lim := retry.NewLimiter(10, time.Minute)

	err := retry.WaitForToken(t.Context(), lim)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	remaining, limit, _ := lim.Info()

	if remaining != 9 {
		t.Errorf("expected 9 remaining tokens, got %d", remaining)
	}

	if limit != 10 {
		t.Errorf("expected limit 10, got %d", limit)
	}
}

func TestWaitForToken_Exhausted(t *testing.T) {
	t.Parallel()

	lim := retry.NewLimiter(5, 50*time.Millisecond) // short window for fast test

	// Drain all tokens
	for range 5 {
		err := retry.WaitForToken(t.Context(), lim)
		if err != nil {
			t.Fatalf("drain: unexpected error: %v", err)
		}
	}

	// Next call should block until window resets, then succeed
	start := time.Now()

	err := retry.WaitForToken(t.Context(), lim)
	if err != nil {
		t.Fatalf("expected nil error after refill, got: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 25*time.Millisecond {
		t.Errorf("expected to block at least 25ms for refill, blocked %v", elapsed)
	}

	remaining, _, _ := lim.Info()
	if remaining != 4 {
		t.Errorf("expected 4 remaining tokens after refill+consume, got %d", remaining)
	}
}

func TestWaitForToken_ContextCancelled(t *testing.T) {
	t.Parallel()

	lim := retry.NewLimiter(1, time.Hour)

	// Drain the only token
	err := retry.WaitForToken(t.Context(), lim)
	if err != nil {
		t.Fatalf("drain: unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately

	err = retry.WaitForToken(ctx, lim)
	if err == nil {
		t.Fatal("expected error when context cancelled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestWaitForToken_NilLimiter(t *testing.T) {
	t.Parallel()

	err := retry.WaitForToken(t.Context(), nil)
	if err != nil {
		t.Fatalf("expected nil error for nil limiter, got: %v", err)
	}
}

// --- IsRetryable tests ---

func TestIsRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"connection reset", errConnReset, true},
		{"timeout", errTimeout, true},
		{"temporary", errTemporary, true},
		{"try again", errTryAgain, true},
		{"broken pipe", errBroken, true},
		{"connection refused", errRefused, true},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"auth failure", errAuthFail, false},
		{"generic error", errors.New("something else"), false}, //nolint:err113 // test-only
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := retry.IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// --- Limiter.Info tests ---

func TestLimiter_Info(t *testing.T) {
	t.Parallel()

	lim := retry.NewLimiter(100, time.Hour)

	remaining, limit, resetAt := lim.Info()

	if remaining != 100 {
		t.Errorf("remaining = %d, want 100", remaining)
	}

	if limit != 100 {
		t.Errorf("limit = %d, want 100", limit)
	}

	if resetAt.Before(time.Now()) {
		t.Error("resetAt should be in the future")
	}
}

func TestLimiter_Info_TokenReset(t *testing.T) {
	t.Parallel()

	// Use very short window so it expires immediately
	lim := retry.NewLimiter(50, 1*time.Millisecond)

	// Drain all tokens
	for range 50 {
		_ = retry.WaitForToken(t.Context(), lim)
	}

	time.Sleep(5 * time.Millisecond) // window expires

	remaining, limit, _ := lim.Info()

	if remaining != 50 {
		t.Errorf("remaining = %d, want 50 (should be reset)", remaining)
	}

	if limit != 50 {
		t.Errorf("limit = %d, want 50", limit)
	}
}

// --- PermanentError tests ---

func TestPermanentError_Error(t *testing.T) {
	t.Parallel()

	inner := errConnReset

	pe := &retry.PermanentError{Err: inner}

	got := pe.Error()
	if got != "connection reset by peer" {
		t.Errorf("Error() = %q, want %q", got, "connection reset by peer")
	}
}

func TestPermanentError_Unwrap(t *testing.T) {
	t.Parallel()

	inner := errAuthFail

	pe := &retry.PermanentError{Err: inner}

	unwrapped := pe.Unwrap()
	if !errors.Is(unwrapped, inner) {
		t.Error("Unwrap() did not return inner error")
	}
}

func TestPermanentError_ErrorsAs(t *testing.T) {
	t.Parallel()

	inner := errConnReset

	pe := &retry.PermanentError{Err: inner}

	wrapped := fmt.Errorf("operation failed: %w", pe)

	var target *retry.PermanentError
	if !errors.As(wrapped, &target) {
		t.Error("errors.As should find PermanentError")
	}
}

func TestWithRetry_PermanentErrorShortCircuit(t *testing.T) {
	t.Parallel()

	var attempts int

	inner := errConnReset

	err := retry.WithRetry(t.Context(), testCfg(), nil, retry.IsRetryable, func() error {
		attempts++

		return &retry.PermanentError{Err: inner}
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}

	if !errors.Is(err, inner) {
		t.Errorf("expected inner error, got: %v", err)
	}
}

func TestWithRetry_RateLimitExhausted(t *testing.T) {
	t.Parallel()

	lim := retry.NewLimiter(1, time.Hour)

	// Drain the only token.
	_ = retry.WaitForToken(t.Context(), lim)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately so WaitForToken fails

	err := retry.WithRetry(ctx, testCfg(), lim, retry.IsRetryable, func() error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error when rate limit token unavailable")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// --- WithRetry + Limiter integration ---

func TestWithRetry_WithLimiter(t *testing.T) {
	t.Parallel()

	lim := retry.NewLimiter(100, time.Minute)

	var attempts int

	err := retry.WithRetry(t.Context(), testCfg(), lim, retry.IsRetryable, func() error {
		attempts++
		if attempts == 1 {
			return errConnReset
		}

		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}

	// Should have consumed 2 tokens (one per attempt)
	remaining, _, _ := lim.Info()
	if remaining != 98 {
		t.Errorf("expected 98 remaining tokens, got %d", remaining)
	}
}
