package retry_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/boutquin/mcp-server-email/internal/retry"
)

func BenchmarkLimiterAllow(b *testing.B) {
	lim := retry.NewLimiter(math.MaxInt32, time.Hour)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = retry.WaitForToken(context.Background(), lim)
	}
}

func BenchmarkLimiterAllow_Parallel(b *testing.B) {
	lim := retry.NewLimiter(math.MaxInt32, time.Hour)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = retry.WaitForToken(context.Background(), lim)
		}
	})
}
