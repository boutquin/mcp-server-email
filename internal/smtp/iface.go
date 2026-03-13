package smtp

import (
	"context"
)

// Operations defines the SMTP operations interface.
// Tool handlers accept this interface instead of concrete *Pool,
// enabling mock injection for unit tests.
type Operations interface {
	Send(ctx context.Context, accountID string, req *SendRequest) error
	AccountEmail(accountID string) (string, error)
	DefaultAccountID() string
}

// Compile-time check: Pool implements Operations.
var _ Operations = (*Pool)(nil)
