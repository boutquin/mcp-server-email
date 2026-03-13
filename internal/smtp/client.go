// Package smtp provides SMTP client for sending emails.
package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"

	"github.com/boutquin/mcp-server-email/internal/auth"
	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/retry"
	"github.com/wneessen/go-mail"
)

// Client wraps SMTP operations with rate limiting.
type Client struct {
	account     *config.Account
	timeout     time.Duration
	limiter     *retry.Limiter
	retryCfg    retry.Config
	debug       bool
	logger      *slog.Logger
	tokenSource oauth2.TokenSource                        // nil for password auth
	newDialer   func(opts ...mail.Option) (dialer, error) // factory for testability
}

// nopLogger is a discarding logger used when no logger is configured.
//
//nolint:gochecknoglobals // package-level singleton, read-only
var nopLogger = slog.New(slog.NewTextHandler(nopWriter{}, &slog.HandlerOptions{Level: slog.LevelError + 1}))

// nopWriter discards all writes.
type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

const authMethodOAuth2 = "oauth2"

// ErrPoolClosed is returned when Get is called on a closed pool.
var ErrPoolClosed = errors.New("pool is closed")

// Pool manages SMTP clients for multiple accounts.
type Pool struct {
	mu       sync.Mutex
	clients  map[string]*Client
	cfg      *config.Config
	accounts map[string]*config.Account
	closing  atomic.Bool // set when Close is called; Get rejects new requests

	// buildTokenSourceFunc overrides the default token source builder for testing.
	// When nil after construction, buildTokenSource is used.
	buildTokenSourceFunc func(account *config.Account) (oauth2.TokenSource, error)
}

// NewPool creates a new SMTP client pool.
func NewPool(cfg *config.Config) *Pool {
	accounts := make(map[string]*config.Account)
	for i := range cfg.Accounts {
		accounts[cfg.Accounts[i].ID] = &cfg.Accounts[i]
	}

	p := &Pool{
		clients:  make(map[string]*Client),
		cfg:      cfg,
		accounts: accounts,
	}
	p.buildTokenSourceFunc = p.buildTokenSource

	return p
}

// Get returns an SMTP client for the given account.
// Unlike IMAP Pool.Get(), this method does NOT need to release the mutex during
// client creation because SMTP clients are lightweight structs — no network I/O
// happens here. The actual TCP+TLS connection is deferred to Send() via
// DialAndSend(). Holding the lock for the duration is safe and correct.
// Returns ErrPoolClosed if the pool is shutting down.
func (p *Pool) Get(accountID string) (*Client, error) {
	if p.closing.Load() {
		return nil, ErrPoolClosed
	}

	if accountID == "" {
		accountID = p.cfg.DefaultAccount
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if client, ok := p.clients[accountID]; ok {
		return client, nil
	}

	account, ok := p.accounts[accountID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", models.ErrAccountNotFound, accountID)
	}

	client := &Client{
		account: account,
		timeout: time.Duration(p.cfg.SMTPTimeoutMS) * time.Millisecond,
		limiter: retry.NewLimiter(p.cfg.SMTPRateLimitRPH, time.Hour),
		retryCfg: retry.Config{
			MaxRetries: retry.DefaultMaxRetries,
			BaseDelay:  retry.DefaultBaseDelay,
			MaxDelay:   retry.DefaultMaxDelay,
		},
		debug:  p.cfg.Debug,
		logger: slog.Default().With(slog.String("component", "smtp"), slog.String("account_id", account.ID)),
		newDialer: func(opts ...mail.Option) (dialer, error) {
			return mail.NewClient(account.SMTPHost, opts...)
		},
	}

	if account.AuthMethod == authMethodOAuth2 {
		ts, tsErr := p.buildTokenSourceFunc(account)
		if tsErr != nil {
			return nil, tsErr
		}

		client.tokenSource = ts
	}

	p.clients[accountID] = client

	return client, nil
}

// Send sends an email for the given account.
func (p *Pool) Send(ctx context.Context, accountID string, req *SendRequest) error {
	client, err := p.Get(accountID)
	if err != nil {
		return err
	}

	return client.Send(ctx, req)
}

// AccountEmail returns the email address for the given account.
func (p *Pool) AccountEmail(accountID string) (string, error) {
	client, err := p.Get(accountID)
	if err != nil {
		return "", err
	}

	return client.Email(), nil
}

// DefaultAccountID returns the configured default account ID.
func (p *Pool) DefaultAccountID() string {
	return p.cfg.DefaultAccount
}

// Close rejects new operations. SMTP connections are short-lived (connect per
// Send), so there are no persistent connections to drain.
func (p *Pool) Close() {
	p.closing.Store(true)
}

// SendAttachment represents a file attachment for sending.
// Set Path for file-based attachments (email_send), or Data for in-memory
// attachments (email_forward). If both are set, Data takes precedence.
type SendAttachment struct {
	Path     string // absolute file path (file-based)
	Filename string // display name
	Data     []byte // raw content (in-memory, e.g. from IMAP fetch)
}

// SendRequest contains the parameters for sending an email.
type SendRequest struct {
	To          []string
	CC          []string
	BCC         []string
	Subject     string
	Body        string
	IsHTML      bool
	ReplyTo     string
	InReplyTo   string
	References  []string
	Attachments []SendAttachment
}

// Send sends an email.
func (c *Client) Send(ctx context.Context, req *SendRequest) error {
	start := time.Now()

	c.log().Info("sending",
		slog.Int("recipients", len(req.To)),
		slog.String("subject", req.Subject),
	)

	err := retry.WithRetry(ctx, c.retryCfg, c.limiter, smtpIsRetryable, func() error {
		return c.withTimeout(ctx, func() error {
			m, buildErr := c.buildMessage(req)
			if buildErr != nil {
				return buildErr
			}

			return c.dialAndSend(m)
		})
	})
	if err != nil {
		c.log().Error("send failed",
			slog.Duration("duration", time.Since(start)),
			slog.String("error", err.Error()),
		)

		return fmt.Errorf("send: %w", err)
	}

	c.log().Info("sent",
		slog.Duration("duration", time.Since(start)),
		slog.Int("recipients", len(req.To)),
	)

	return nil
}

// RateLimitInfo returns current rate limit state.
func (c *Client) RateLimitInfo() (int, int, time.Time) {
	return c.limiter.Info()
}

// AccountID returns the account ID.
func (c *Client) AccountID() string {
	return c.account.ID
}

// Email returns the account email.
func (c *Client) Email() string {
	return c.account.Email
}

// log returns the client's logger, falling back to nopLogger if nil.
func (c *Client) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}

	return nopLogger
}

// buildTokenSource creates an OAuth2 token source for the given account.
func (p *Pool) buildTokenSource(account *config.Account) (oauth2.TokenSource, error) {
	tokenDir, err := auth.DefaultTokenDir()
	if err != nil {
		return nil, fmt.Errorf("token dir: %w", err)
	}

	store, err := auth.NewTokenStore(tokenDir)
	if err != nil {
		return nil, fmt.Errorf("token store: %w", err)
	}

	oauthCfg, _, err := auth.OAuthConfig(account)
	if err != nil {
		return nil, fmt.Errorf("oauth config: %w", err)
	}

	ts, err := auth.NewTokenSource(store, account.ID, oauthCfg)
	if err != nil {
		return nil, fmt.Errorf("token source for %s: %w", account.ID, err)
	}

	return ts, nil
}

// dialAndSend creates an SMTP client, connects, and sends the message.
func (c *Client) dialAndSend(m *mail.Msg) error {
	opts := []mail.Option{
		mail.WithPort(c.account.SMTPPort),
		mail.WithTimeout(c.timeout),
	}

	if c.account.AuthMethod == authMethodOAuth2 && c.tokenSource != nil {
		token, err := c.tokenSource.Token()
		if err != nil {
			return fmt.Errorf("get OAuth2 token: %w", err)
		}

		smtpAuth := &xoauth2Auth{email: c.account.Email, token: token.AccessToken}
		opts = append(opts, mail.WithSMTPAuthCustom(smtpAuth))
	} else {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(c.account.Username),
			mail.WithPassword(c.account.Password),
		)
	}

	opts = append(opts, c.tlsOptions()...)

	d, err := c.newDialer(opts...)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}

	err = d.DialAndSend(m)
	if err == nil {
		return nil
	}

	// For OAuth2: retry once on 535 auth failure with refreshed token.
	if c.account.AuthMethod == authMethodOAuth2 && isSMTPAuthFailure(err) {
		c.log().Warn("SMTP OAuth2 auth failed, refreshing token and retrying",
			slog.String("error", err.Error()))

		return c.retryWithRefreshedToken(m)
	}

	return fmt.Errorf("send email: %w", err)
}

// retryWithRefreshedToken attempts to send the message with a refreshed OAuth2 token.
func (c *Client) retryWithRefreshedToken(m *mail.Msg) error {
	newToken, err := c.tokenSource.Token()
	if err != nil {
		return fmt.Errorf("SMTP token refresh: %w", err)
	}

	refreshedAuth := &xoauth2Auth{email: c.account.Email, token: newToken.AccessToken}
	tlsOpts := c.tlsOptions()

	retryOpts := make([]mail.Option, 0, 3+len(tlsOpts)) //nolint:mnd // 3 base options + TLS
	retryOpts = append(retryOpts,
		mail.WithPort(c.account.SMTPPort),
		mail.WithTimeout(c.timeout),
		mail.WithSMTPAuthCustom(refreshedAuth),
	)
	retryOpts = append(retryOpts, tlsOpts...)

	d, err := c.newDialer(retryOpts...)
	if err != nil {
		return fmt.Errorf("SMTP retry client: %w", err)
	}

	retryErr := d.DialAndSend(m)
	if retryErr != nil {
		return fmt.Errorf("SMTP retry send: %w", retryErr)
	}

	return nil
}

// tlsOptions returns the TLS-related mail options for the client.
func (c *Client) tlsOptions() []mail.Option {
	var opts []mail.Option

	if c.account.SMTPUseTLS() {
		opts = append(opts, mail.WithSSL())
	} else {
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSMandatory))
	}

	if c.account.InsecureSkipVerify {
		opts = append(opts, mail.WithTLSConfig(&tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // configurable for testing/dev
		}))
	}

	return opts
}

// buildMessage constructs a go-mail message from the send request.
func (c *Client) buildMessage(req *SendRequest) (*mail.Msg, error) {
	m := mail.NewMsg()

	err := m.From(c.account.Email)
	if err != nil {
		return nil, fmt.Errorf("set from: %w", err)
	}

	err = m.To(req.To...)
	if err != nil {
		return nil, fmt.Errorf("set to: %w", err)
	}

	if len(req.CC) > 0 {
		err = m.Cc(req.CC...)
		if err != nil {
			return nil, fmt.Errorf("set cc: %w", err)
		}
	}

	if len(req.BCC) > 0 {
		err = m.Bcc(req.BCC...)
		if err != nil {
			return nil, fmt.Errorf("set bcc: %w", err)
		}
	}

	if req.ReplyTo != "" {
		err = m.ReplyTo(req.ReplyTo)
		if err != nil {
			return nil, fmt.Errorf("set reply-to: %w", err)
		}
	}

	if req.InReplyTo != "" {
		m.SetGenHeader(mail.HeaderInReplyTo, req.InReplyTo)
	}

	if len(req.References) > 0 {
		m.SetGenHeader(mail.HeaderReferences, strings.Join(req.References, " "))
	}

	m.Subject(req.Subject)

	if req.IsHTML {
		m.SetBodyString(mail.TypeTextHTML, req.Body)
	} else {
		m.SetBodyString(mail.TypeTextPlain, req.Body)
	}

	err = attachFiles(m, req.Attachments)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// attachFiles adds all attachments to the mail message.
func attachFiles(m *mail.Msg, attachments []SendAttachment) error {
	for _, att := range attachments {
		if len(att.Data) > 0 {
			err := m.AttachReader(att.Filename, bytes.NewReader(att.Data))
			if err != nil {
				return fmt.Errorf("attach %s: %w", att.Filename, err)
			}
		} else {
			m.AttachFile(att.Path, mail.WithFileName(att.Filename))
		}
	}

	return nil
}

// withTimeout runs an operation with the client's configured timeout.
// If the operation doesn't complete within the timeout, it returns context.DeadlineExceeded.
func (c *Client) withTimeout(ctx context.Context, op func() error) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		done <- op()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("SMTP operation timed out after %v: %w", c.timeout, ctx.Err())
	}
}

// smtpIsRetryable extends retry.IsRetryable with SMTP-specific 4xx pattern matching.
func smtpIsRetryable(err error) bool {
	if retry.IsRetryable(err) {
		return true
	}

	if err == nil {
		return false
	}

	return smtp4xxPattern.MatchString(err.Error())
}

// smtp4xxPattern matches 4xx SMTP status codes (400-499) at the start of the
// error string, which is where SMTP response codes appear (e.g. "421 service
// unavailable"). This avoids false positives like port numbers ("port 443") or
// casual mentions of the digit 4.
var smtp4xxPattern = regexp.MustCompile(`^4[0-9]{2}\s`)
