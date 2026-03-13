package imap

import (
	"context"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/config"
)

// benchPool creates a Pool with a mock newClientFunc for benchmarking.
func benchPool(b *testing.B, accounts []config.Account) *Pool {
	b.Helper()

	cfg := &config.Config{
		DefaultAccount: accounts[0].ID,
		Accounts:       accounts,
	}

	pool := NewPool(cfg)
	pool.newClientFunc = func(
		_ context.Context, _ *config.Account, _ *config.Config,
	) (*Client, error) {
		return newMockClient(&mockConnector{}), nil
	}

	return pool
}

func BenchmarkPoolGetRelease(b *testing.B) {
	b.Run("single-account", func(b *testing.B) {
		accounts := []config.Account{{
			ID:       "bench-account",
			IMAPHost: "localhost",
			IMAPPort: 993,
			Username: "bench",
			Password: "bench",
			Email:    "bench@example.com",
		}}

		pool := benchPool(b, accounts)

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = pool.Get(context.Background(), "bench-account")
		}
	})

	b.Run("multi-account", func(b *testing.B) {
		accounts := []config.Account{
			{ID: "acct-0", IMAPHost: "localhost", IMAPPort: 993, Username: "u0", Password: "p0", Email: "u0@example.com"},
			{ID: "acct-1", IMAPHost: "localhost", IMAPPort: 993, Username: "u1", Password: "p1", Email: "u1@example.com"},
			{ID: "acct-2", IMAPHost: "localhost", IMAPPort: 993, Username: "u2", Password: "p2", Email: "u2@example.com"},
		}

		pool := benchPool(b, accounts)
		ids := []string{"acct-0", "acct-1", "acct-2"}

		b.ReportAllocs()
		b.ResetTimer()

		for i := range b.N {
			_, _ = pool.Get(context.Background(), ids[i%len(ids)])
		}
	})
}
