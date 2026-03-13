package imap

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/retry"
	goimap "github.com/emersion/go-imap/v2"
)

func TestNewPool(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com"},
			{ID: "acc2", Email: "two@example.com"},
		},
		DefaultAccount:   "acc1",
		IMAPRateLimitRPM: 60,
		IMAPTimeoutMS:    30000,
	}

	pool := NewPool(cfg)

	if pool == nil {
		t.Fatal("NewPool returned nil")
	}

	if len(pool.accounts) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(pool.accounts))
	}

	if pool.cfg != cfg {
		t.Error("pool.cfg not set correctly")
	}
}

func TestPool_AccountStatus_NoConnections(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com"},
			{ID: "acc2", Email: "two@example.com"},
		},
		DefaultAccount: "acc1",
	}

	pool := NewPool(cfg)
	statuses := pool.AccountStatus()

	if len(statuses) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if s.Connected {
			t.Errorf("expected %s to not be connected", s.ID)
		}
	}

	// Check default account flag
	foundDefault := false

	for _, s := range statuses {
		if s.ID == "acc1" && s.IsDefault {
			foundDefault = true
		}
	}

	if !foundDefault {
		t.Error("expected acc1 to be marked as default")
	}
}

func TestPool_Get_AccountNotFound(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com"},
		},
		DefaultAccount:   "acc1",
		IMAPRateLimitRPM: 60,
		IMAPTimeoutMS:    1000,
	}

	pool := NewPool(cfg)

	_, err := pool.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent account")
	}

	if !strings.Contains(err.Error(), "account not found") {
		t.Errorf("expected 'account not found' error, got: %v", err)
	}
}

func TestPool_Close(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com"},
		},
		DefaultAccount: "acc1",
	}

	pool := NewPool(cfg)
	pool.Close(context.Background()) // Should not panic even with no connections

	if len(pool.clients) != 0 {
		t.Error("expected clients map to be empty after Close")
	}
}

func TestIsRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errMsg string
		want   bool
	}{
		{"connection reset by peer", true},
		{"timeout waiting for response", true},
		{"operation timed out", true},
		{"temporary failure", true},
		{"try again later", true},
		{"broken pipe", true},
		{"connection refused", true},
		{"authentication failed", false},
		{"mailbox not found", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			t.Parallel()

			var err error
			if tt.errMsg != "" {
				err = &testError{msg: tt.errMsg}
			}

			if got := retry.IsRetryable(err); got != tt.want {
				t.Errorf("isRetryable(%q) = %v, want %v", tt.errMsg, got, tt.want)
			}
		})
	}

	// Test nil error
	if retry.IsRetryable(nil) {
		t.Error("retry.IsRetryable(nil) should return false")
	}

	// Test context.DeadlineExceeded
	if !retry.IsRetryable(context.DeadlineExceeded) {
		t.Error("retry.IsRetryable(context.DeadlineExceeded) should return true")
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestClient_RateLimitInfo(t *testing.T) {
	t.Parallel()

	lim := retry.NewLimiter(60, time.Minute)

	// Consume 15 tokens
	for range 15 {
		_ = retry.WaitForToken(t.Context(), lim)
	}

	client := &Client{
		limiter: lim,
	}

	remaining, limit, resetAt := client.RateLimitInfo()

	if remaining != 45 {
		t.Errorf("expected remaining 45, got %d", remaining)
	}

	if limit != 60 {
		t.Errorf("expected limit 60, got %d", limit)
	}

	if resetAt.Before(time.Now()) {
		t.Error("resetAt should be in the future")
	}
}

func TestClient_RateLimitInfo_TokenReset(t *testing.T) {
	t.Parallel()

	// Use very short window so it expires immediately
	lim := retry.NewLimiter(60, 1*time.Millisecond)

	// Drain some tokens
	for range 50 {
		_ = retry.WaitForToken(t.Context(), lim)
	}

	time.Sleep(5 * time.Millisecond) // window expires

	client := &Client{
		limiter: lim,
	}

	remaining, limit, _ := client.RateLimitInfo()

	// Tokens should be reset to limit
	if remaining != 60 {
		t.Errorf("expected remaining to be reset to 60, got %d", remaining)
	}

	if limit != 60 {
		t.Errorf("expected limit 60, got %d", limit)
	}
}

func TestClient_IsConnected_Nil(t *testing.T) {
	t.Parallel()

	client := &Client{
		conn: nil,
	}

	if client.IsConnected() {
		t.Error("expected IsConnected to return false when conn is nil")
	}
}

func TestClient_Close_Nil(t *testing.T) {
	t.Parallel()

	client := &Client{
		conn: nil,
	}

	err := client.Close()
	if err != nil {
		t.Errorf("Close() on nil conn should return nil error, got: %v", err)
	}
}

func TestClient_AccountID(t *testing.T) {
	t.Parallel()

	client := &Client{
		account: &config.Account{ID: "test-account"},
	}

	if client.AccountID() != "test-account" {
		t.Errorf("AccountID() = %q, want %q", client.AccountID(), "test-account")
	}
}

func TestClient_LastActivity(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &Client{
		lastActive: now,
	}

	if !client.LastActivity().Equal(now) {
		t.Errorf("LastActivity() = %v, want %v", client.LastActivity(), now)
	}
}

func TestClient_ForceClose(t *testing.T) {
	t.Parallel()

	client := &Client{}

	// forceClose should not panic on nil rawConn
	client.forceClose()

	if !client.connInvalid.Load() {
		t.Error("connInvalid should be true after forceClose")
	}
}

func TestBuildSearchCriteria_BodySearch(t *testing.T) {
	t.Parallel()

	const query = "hello"

	criteria := buildSearchCriteria(query, "", "", "", "")

	// Must use OR criteria: SUBJECT "hello" OR BODY "hello"
	// Or is [][2]SearchCriteria — one pair: [subject criteria, body criteria]
	if len(criteria.Or) != 1 {
		t.Fatalf("expected 1 OR pair (subject, body), got %d", len(criteria.Or))
	}

	pair := criteria.Or[0]

	// First element: subject header search
	if len(pair[0].Header) != 1 || pair[0].Header[0].Key != "Subject" || pair[0].Header[0].Value != query {
		t.Errorf("OR[0] should search Subject for %q, got %+v", query, pair[0].Header)
	}

	// Second element: body search
	if len(pair[1].Body) != 1 || pair[1].Body[0] != query {
		t.Errorf("OR[1] should search Body for %q, got %+v", query, pair[1].Body)
	}

	// Top-level Header should NOT contain the query (it goes in Or branches)
	for _, h := range criteria.Header {
		if h.Key == "Subject" && h.Value == query {
			t.Error("query should be in Or branches, not top-level Header")
		}
	}
}

func TestBuildSearchCriteria_BodySearchWithFilters(t *testing.T) {
	t.Parallel()

	criteria := buildSearchCriteria("hello", "sender@example.com", "recip@example.com", "2024-01-01", "2024-12-31")

	// OR pair for query
	if len(criteria.Or) != 1 {
		t.Fatalf("expected 1 OR pair, got %d", len(criteria.Or))
	}

	// From and To filters should still be in top-level Header (not in Or branches)
	foundFrom := false
	foundTo := false

	for _, h := range criteria.Header {
		if h.Key == "From" && h.Value == "sender@example.com" {
			foundFrom = true
		}

		if h.Key == "To" && h.Value == "recip@example.com" {
			foundTo = true
		}
	}

	if !foundFrom {
		t.Error("expected From filter in top-level Header")
	}

	if !foundTo {
		t.Error("expected To filter in top-level Header")
	}

	// Since and Before should be set
	if criteria.Since.IsZero() {
		t.Error("expected Since to be set")
	}

	if criteria.Before.IsZero() {
		t.Error("expected Before to be set")
	}
}

func TestBuildSearchCriteria_EmptyQuery(t *testing.T) {
	t.Parallel()

	criteria := buildSearchCriteria("", "sender@example.com", "", "", "")

	// No OR branches when query is empty
	if len(criteria.Or) != 0 {
		t.Errorf("expected 0 OR branches for empty query, got %d", len(criteria.Or))
	}

	// From filter should still work
	if len(criteria.Header) != 1 || criteria.Header[0].Key != "From" {
		t.Errorf("expected From header filter, got %+v", criteria.Header)
	}
}

func TestClient_WaitForToken_ContextCanceled(t *testing.T) {
	t.Parallel()

	lim := retry.NewLimiter(1, time.Hour)

	// Drain the only token
	_ = retry.WaitForToken(t.Context(), lim)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := retry.WaitForToken(ctx, lim)
	if err == nil {
		t.Error("expected error when context is canceled")
	}
}

func TestWrapIMAPError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		op          string
		err         error
		wantNil     bool
		wantContain string
		wantAbsent  string
	}{
		{
			name:    "nil error returns nil",
			op:      "list folders",
			err:     nil,
			wantNil: true,
		},
		{
			name:        "auth failure maps to sentinel",
			op:          "login",
			err:         &testError{msg: "NO [AUTHENTICATIONFAILED] Invalid credentials"},
			wantContain: "authentication failed",
			wantAbsent:  "AUTHENTICATIONFAILED",
		},
		{
			name:        "nonexistent folder maps to sentinel",
			op:          "select",
			err:         &testError{msg: "NO [NONEXISTENT] Unknown Mailbox: BadFolder"},
			wantContain: "folder not found",
			wantAbsent:  "NONEXISTENT",
		},
		{
			name:        "connection refused maps to sentinel",
			op:          "connect",
			err:         &testError{msg: "dial tcp: connection refused"},
			wantContain: "connection failed",
		},
		{
			name:        "unknown error preserves message with op",
			op:          "search",
			err:         &testError{msg: "some unexpected error"},
			wantContain: "search",
		},
		{
			name:        "op name included in wrapped error",
			op:          "list folders",
			err:         &testError{msg: "NO [AUTHENTICATIONFAILED] bad creds"},
			wantContain: "list folders",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := wrapIMAPError(tt.op, tt.err)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}

				return
			}

			if result == nil {
				t.Fatal("expected non-nil error")
			}

			errMsg := result.Error()

			if tt.wantContain != "" && !strings.Contains(errMsg, tt.wantContain) {
				t.Errorf("expected error to contain %q, got %q", tt.wantContain, errMsg)
			}

			if tt.wantAbsent != "" && strings.Contains(errMsg, tt.wantAbsent) {
				t.Errorf("expected error to NOT contain %q, got %q", tt.wantAbsent, errMsg)
			}
		})
	}
}

func TestPool_ConcurrentDifferentAccounts(t *testing.T) {
	t.Parallel()

	// Track how many concurrent newClient calls are in progress.
	// If the pool lock is held during newClient, only one goroutine can be
	// inside newClient at a time, so maxConcurrent will be 1.
	// After the fix, both goroutines should enter newClient concurrently.
	var (
		mu            sync.Mutex
		concurrent    int
		maxConcurrent int
	)

	// barrier ensures both goroutines enter Get() before either completes newClient
	barrier := make(chan struct{})

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com", IMAPHost: "localhost", IMAPPort: 993},
			{ID: "acc2", Email: "two@example.com", IMAPHost: "localhost", IMAPPort: 993},
		},
		DefaultAccount:   "acc1",
		IMAPRateLimitRPM: 60,
		IMAPTimeoutMS:    30000,
	}

	pool := NewPool(cfg)
	pool.newClientFunc = func(
		ctx context.Context, account *config.Account, cfg *config.Config,
	) (*Client, error) {
		mu.Lock()

		concurrent++

		if concurrent > maxConcurrent {
			maxConcurrent = concurrent
		}

		mu.Unlock()

		// Signal that we've entered newClient, then wait for the other goroutine
		select {
		case barrier <- struct{}{}:
		default:
		}

		// Wait until the other goroutine also enters newClient (or a timeout)
		select {
		case <-barrier:
		case <-time.After(2 * time.Second):
			// If the other goroutine can't enter because the lock is held,
			// this timeout will fire — that's the bug we're testing for
		}

		// Simulate network I/O delay
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		concurrent--
		mu.Unlock()

		return &Client{
			account: account,
			timeout: time.Duration(cfg.IMAPTimeoutMS) * time.Millisecond,
			limiter: retry.NewLimiter(cfg.IMAPRateLimitRPM, time.Minute),
			retryCfg: retry.Config{
				MaxRetries: retry.DefaultMaxRetries,
				BaseDelay:  retry.DefaultBaseDelay,
				MaxDelay:   retry.DefaultMaxDelay,
			},
		}, nil
	}

	var wg sync.WaitGroup

	errs := make([]error, 2)

	wg.Add(2)

	go func() {
		defer wg.Done()

		_, errs[0] = pool.Get(context.Background(), "acc1")
	}()

	go func() {
		defer wg.Done()

		_, errs[1] = pool.Get(context.Background(), "acc2")
	}()

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Get() error = %v", i, err)
		}
	}

	mu.Lock()
	mc := maxConcurrent
	mu.Unlock()

	if mc < 2 {
		t.Errorf(
			"expected concurrent newClient calls for different accounts "+
				"(maxConcurrent=%d), pool lock likely held during I/O", mc,
		)
	}
}

func TestParseMessageIDList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single ID", "<msg1@example.com>", []string{"<msg1@example.com>"}},
		{"multiple IDs", "<msg1@example.com> <msg2@example.com>", []string{"<msg1@example.com>", "<msg2@example.com>"}},
		{"empty string", "", nil},
		{"extra spaces", "  <a@b>   <c@d>  ", []string{"<a@b>", "<c@d>"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseMessageIDList(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseMessageIDList(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.want, len(tt.want))
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseMessageIDList(%q)[%d] = %q, want %q",
						tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestApplyHeaderFields(t *testing.T) {
	t.Parallel()

	t.Run("nil reader", func(t *testing.T) {
		t.Parallel()

		e := &models.Email{}
		applyHeaderFields(e, nil)

		if e.References != nil {
			t.Errorf("expected nil References, got %v", e.References)
		}
	})

	t.Run("empty reader", func(t *testing.T) {
		t.Parallel()

		e := &models.Email{}
		applyHeaderFields(e, strings.NewReader(""))

		if e.References != nil {
			t.Errorf("expected nil References, got %v", e.References)
		}
	})

	t.Run("references header", func(t *testing.T) {
		t.Parallel()

		e := &models.Email{}
		header := "References: <msg1@example.com> <msg2@example.com>\r\n"
		applyHeaderFields(e, strings.NewReader(header))

		if len(e.References) != 2 {
			t.Fatalf("expected 2 references, got %d: %v", len(e.References), e.References)
		}

		if e.References[0] != "<msg1@example.com>" {
			t.Errorf("References[0] = %q", e.References[0])
		}
	})

	t.Run("no references header", func(t *testing.T) {
		t.Parallel()

		e := &models.Email{}
		header := "Subject: Test\r\nFrom: test@example.com\r\n"
		applyHeaderFields(e, strings.NewReader(header))

		if e.References != nil {
			t.Errorf("expected nil References, got %v", e.References)
		}
	})
}

func TestExtractContentType(t *testing.T) {
	t.Parallel()

	t.Run("single part text/plain", func(t *testing.T) {
		t.Parallel()

		bs := &goimap.BodyStructureSinglePart{
			Type:    "TEXT",
			Subtype: "PLAIN",
		}

		ct := extractContentType(bs)
		if ct != "text/plain" {
			t.Errorf("extractContentType() = %q, want %q", ct, "text/plain")
		}
	})

	t.Run("single part text/html", func(t *testing.T) {
		t.Parallel()

		bs := &goimap.BodyStructureSinglePart{
			Type:    "TEXT",
			Subtype: "HTML",
		}

		ct := extractContentType(bs)
		if ct != "text/html" {
			t.Errorf("extractContentType() = %q, want %q", ct, "text/html")
		}
	})

	t.Run("multipart with text child", func(t *testing.T) {
		t.Parallel()

		bs := &goimap.BodyStructureMultiPart{
			Children: []goimap.BodyStructure{
				&goimap.BodyStructureSinglePart{Type: "TEXT", Subtype: "PLAIN"},
				&goimap.BodyStructureSinglePart{Type: "APPLICATION", Subtype: "PDF"},
			},
		}

		ct := extractContentType(bs)
		if ct != "text/plain" {
			t.Errorf("extractContentType() = %q, want %q", ct, "text/plain")
		}
	})

	t.Run("nested multipart", func(t *testing.T) {
		t.Parallel()

		bs := &goimap.BodyStructureMultiPart{
			Children: []goimap.BodyStructure{
				&goimap.BodyStructureMultiPart{
					Children: []goimap.BodyStructure{
						&goimap.BodyStructureSinglePart{Type: "TEXT", Subtype: "HTML"},
					},
				},
			},
		}

		ct := extractContentType(bs)
		if ct != "text/html" {
			t.Errorf("extractContentType() = %q, want %q", ct, "text/html")
		}
	})

	t.Run("multipart with no text falls through to nested", func(t *testing.T) {
		t.Parallel()

		bs := &goimap.BodyStructureMultiPart{
			Children: []goimap.BodyStructure{
				&goimap.BodyStructureSinglePart{Type: "APPLICATION", Subtype: "PDF"},
			},
		}

		// Non-text single parts are still returned via recursion
		ct := extractContentType(bs)
		if ct != "application/pdf" {
			t.Errorf("extractContentType() = %q, want %q", ct, "application/pdf")
		}
	})

	t.Run("empty multipart returns empty", func(t *testing.T) {
		t.Parallel()

		bs := &goimap.BodyStructureMultiPart{
			Children: []goimap.BodyStructure{},
		}

		ct := extractContentType(bs)
		if ct != "" {
			t.Errorf("extractContentType() = %q, want empty", ct)
		}
	})
}

func TestBuildFetchOptions(t *testing.T) {
	t.Parallel()

	t.Run("without body", func(t *testing.T) {
		t.Parallel()

		opts := buildFetchOptions(false)

		if !opts.UID {
			t.Error("expected UID to be true")
		}

		if !opts.Flags {
			t.Error("expected Flags to be true")
		}

		if !opts.Envelope {
			t.Error("expected Envelope to be true")
		}

		if opts.BodyStructure == nil {
			t.Error("expected BodyStructure to be set")
		}

		// Should have 1 BodySection (the HEADER.FIELDS[References] peek)
		if len(opts.BodySection) != 1 {
			t.Errorf("expected 1 BodySection for no-body fetch, got %d", len(opts.BodySection))
		}
	})

	t.Run("with body", func(t *testing.T) {
		t.Parallel()

		opts := buildFetchOptions(true)

		// Should have 2 BodySections: TEXT + HEADER.FIELDS[References]
		if len(opts.BodySection) != 2 {
			t.Errorf("expected 2 BodySections for body fetch, got %d", len(opts.BodySection))
		}
	})
}

func TestPool_ConcurrentSameAccount(t *testing.T) {
	t.Parallel()

	// Two goroutines request the same account concurrently.
	// Only one connection should be created (first-writer-wins).
	var callCount int

	var mu sync.Mutex

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com", IMAPHost: "localhost", IMAPPort: 993},
		},
		DefaultAccount:   "acc1",
		IMAPRateLimitRPM: 60,
		IMAPTimeoutMS:    30000,
	}

	pool := NewPool(cfg)
	pool.newClientFunc = func(
		ctx context.Context, account *config.Account, cfg *config.Config,
	) (*Client, error) {
		mu.Lock()
		callCount++
		mu.Unlock()

		// Simulate slow connection
		time.Sleep(100 * time.Millisecond)

		return &Client{
			account: account,
			timeout: time.Duration(cfg.IMAPTimeoutMS) * time.Millisecond,
			limiter: retry.NewLimiter(cfg.IMAPRateLimitRPM, time.Minute),
			retryCfg: retry.Config{
				MaxRetries: retry.DefaultMaxRetries,
				BaseDelay:  retry.DefaultBaseDelay,
				MaxDelay:   retry.DefaultMaxDelay,
			},
		}, nil
	}

	var wg sync.WaitGroup

	errs := make([]error, 2)
	clients := make([]*Client, 2)

	wg.Add(2)

	go func() {
		defer wg.Done()

		clients[0], errs[0] = pool.Get(context.Background(), "acc1")
	}()

	go func() {
		defer wg.Done()

		clients[1], errs[1] = pool.Get(context.Background(), "acc1")
	}()

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Get() error = %v", i, err)
		}
	}

	// Both goroutines should return the same client (first-writer-wins)
	if clients[0] != clients[1] {
		t.Error("expected both goroutines to return the same client for the same account")
	}
}

func TestPool_GetAfterClose(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com"},
		},
		DefaultAccount:   "acc1",
		IMAPRateLimitRPM: 60,
		IMAPTimeoutMS:    1000,
	}

	pool := NewPool(cfg)
	pool.Close(context.Background())

	_, err := pool.Get(context.Background(), "acc1")
	if err == nil {
		t.Fatal("expected error from Get after Close")
	}

	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("expected ErrPoolClosed, got: %v", err)
	}
}

func TestPool_CloseWaitsInflight(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com", IMAPHost: "localhost", IMAPPort: 993},
		},
		DefaultAccount:   "acc1",
		IMAPRateLimitRPM: 60,
		IMAPTimeoutMS:    30000,
	}

	pool := NewPool(cfg)
	pool.newClientFunc = func(
		ctx context.Context, account *config.Account, cfg *config.Config,
	) (*Client, error) {
		return &Client{
			account: account,
			conn:    &mockConnector{},
			timeout: time.Duration(cfg.IMAPTimeoutMS) * time.Millisecond,
			limiter: retry.NewLimiter(cfg.IMAPRateLimitRPM, time.Minute),
			retryCfg: retry.Config{
				MaxRetries: 0, // no retries for this test
				BaseDelay:  retry.DefaultBaseDelay,
				MaxDelay:   retry.DefaultMaxDelay,
			},
		}, nil
	}

	// Start a simulated in-flight operation via a Pool method.
	// We use ListFolders with a mock that blocks until we release it.
	opStarted := make(chan struct{})
	opRelease := make(chan struct{})

	// Pre-populate the pool with a client that has a blocking ListFolders.
	client, _ := pool.Get(context.Background(), "acc1")
	client.conn = &mockConnector{
		listFn: func(_, _ string, _ *goimap.ListOptions) ([]*goimap.ListData, error) {
			close(opStarted)
			<-opRelease

			return nil, nil
		},
	}

	// Start in-flight operation in background
	go func() {
		_, _ = pool.ListFolders(context.Background(), "acc1")
	}()

	// Wait until the operation has started
	<-opStarted

	// Close with a generous timeout — it should block until the operation completes
	closeDone := make(chan struct{})

	go func() {
		pool.Close(context.Background())
		close(closeDone)
	}()

	// Verify Close is blocking (hasn't returned yet)
	select {
	case <-closeDone:
		t.Fatal("Close returned before in-flight operation completed")
	case <-time.After(50 * time.Millisecond):
		// Expected: Close is still waiting
	}

	// Release the in-flight operation
	close(opRelease)

	// Now Close should complete
	select {
	case <-closeDone:
		// Expected
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after in-flight operation completed")
	}
}

func TestPool_CloseTimeout(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com", IMAPHost: "localhost", IMAPPort: 993},
		},
		DefaultAccount:   "acc1",
		IMAPRateLimitRPM: 60,
		IMAPTimeoutMS:    30000,
	}

	pool := NewPool(cfg)
	pool.newClientFunc = func(
		ctx context.Context, account *config.Account, cfg *config.Config,
	) (*Client, error) {
		return &Client{
			account: account,
			conn:    &mockConnector{},
			timeout: time.Duration(cfg.IMAPTimeoutMS) * time.Millisecond,
			limiter: retry.NewLimiter(cfg.IMAPRateLimitRPM, time.Minute),
			retryCfg: retry.Config{
				MaxRetries: 0,
				BaseDelay:  retry.DefaultBaseDelay,
				MaxDelay:   retry.DefaultMaxDelay,
			},
		}, nil
	}

	// Pre-populate the pool
	client, _ := pool.Get(context.Background(), "acc1")

	// Set up a mock that blocks forever (simulates a hung operation)
	opStarted := make(chan struct{})
	client.conn = &mockConnector{
		listFn: func(_, _ string, _ *goimap.ListOptions) ([]*goimap.ListData, error) {
			close(opStarted)

			select {} // block forever
		},
	}

	// Start an in-flight operation that will never complete
	go func() {
		_, _ = pool.ListFolders(context.Background(), "acc1")
	}()

	<-opStarted

	// Close with a short timeout — should return after timeout, not hang
	shortCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	closeDone := make(chan struct{})

	go func() {
		pool.Close(shortCtx)
		close(closeDone)
	}()

	select {
	case <-closeDone:
		// Expected: Close returned after timeout
	case <-time.After(2 * time.Second):
		t.Fatal("Close hung despite context timeout — should have returned")
	}
}
