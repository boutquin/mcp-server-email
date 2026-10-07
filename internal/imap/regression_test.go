package imap

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/mail"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

const (
	testDisplayDomain  = "example.com"
	testDisplayMailbox = "sender"
	testDisplayAddress = "sender@example.com"
)

func TestMessageBodyMIME(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, raw, body, contentType string
	}{
		{"quoted printable", "Content-Type: text/plain; charset=utf-8\r\n" +
			"Content-Transfer-Encoding: quoted-printable\r\n\r\nSalut =C8=99i bine!", "Salut și bine!", testContentPlain},
		{"base64", "Content-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
			base64.StdEncoding.EncodeToString([]byte("<p>Țară</p>")), "<p>Țară</p>", testContentHTML},
		{"charset", "Content-Type: text/plain; charset=iso-8859-1\r\n\r\nCaf\xe9", "Café", testContentPlain},
		{"nested", "Content-Type: multipart/mixed; boundary=m\r\n\r\n--m\r\nContent-Type: text/plain\r\n" +
			"Content-Disposition: attachment; filename=ignore.txt\r\n\r\nattachment text\r\n--m\r\n" +
			"Content-Type: multipart/alternative; boundary=a\r\n\r\n--a\r\nContent-Type: text/plain\r\n\r\n" +
			"Visible body\r\n--a\r\nContent-Type: text/html\r\n\r\n<p>Visible body</p>\r\n--a--\r\n--m--\r\n",
			"Visible body", testContentPlain},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := mimeFixtureClient(tt.raw)

			email, err := client.GetMessage(t.Context(), testMailboxINBOX, 1)
			if err != nil {
				t.Fatal(err)
			}

			if email.Body != tt.body || email.ContentType != tt.contentType {
				t.Fatalf("body/type = %q/%q, want %q/%q", email.Body, email.ContentType, tt.body, tt.contentType)
			}
		})
	}
}

func TestMessageBodyMIMEError(t *testing.T) {
	t.Parallel()

	client := mimeFixtureClient("Content-Type: text/plain\r\nContent-Transfer-Encoding: base64\r\n\r\n%%%invalid")

	_, err := client.GetMessage(t.Context(), testMailboxINBOX, 1)
	if err == nil {
		t.Fatal("expected MIME decoding error")
	}
}

func mimeFixtureClient(raw string) *Client {
	return newMockClient(&mockConnector{
		fetchFn: func(_ imap.NumSet, opts *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{{
				UID:         1,
				BodySection: []imapclient.FetchBodySectionBuffer{{Section: opts.BodySection[0], Bytes: []byte(raw)}},
			}}, nil
		},
	})
}

func TestTransportFailureReplacesClient(t *testing.T) {
	t.Parallel()

	for _, cause := range []error{net.ErrClosed, io.EOF, io.ErrUnexpectedEOF} {
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()
			testTransportReplacement(t, cause)
		})
	}
}

func testTransportReplacement(t *testing.T, cause error) {
	t.Helper()

	stale := newMockClient(&mockConnector{})

	err := stale.withTimeout(t.Context(), func() error { return fmt.Errorf("fetch: %w", cause) })
	if err == nil || stale.IsConnected() {
		t.Fatal("closed transport must invalidate the connection")
	}

	healthy := newMockClient(&mockConnector{})
	pool := NewPool(&config.Config{Accounts: []config.Account{*stale.account}})
	pool.clients[stale.account.ID] = stale
	pool.newClientFunc = func(_ context.Context, _ *config.Account, _ *config.Config) (*Client, error) {
		return healthy, nil
	}

	got, err := pool.Get(t.Context(), stale.account.ID)
	if err != nil || got != healthy {
		t.Fatalf("pool did not replace closed connection: %v", err)
	}

	_, err = got.ListFolders(t.Context())
	if err != nil {
		t.Fatal(err)
	}
}

func TestCommandRejectionKeepsConnection(t *testing.T) {
	t.Parallel()

	client := newMockClient(&mockConnector{})

	err := client.withTimeout(t.Context(), func() error { return errTestMailbox })
	if err == nil || !client.IsConnected() {
		t.Fatal("an IMAP command rejection should retain a usable connection")
	}
}

func TestDisplayNameAddressRoundTrip(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"sender@example.com (Test User)", "Last, First", `A "quote"`, "Utilizator ș ț"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			formatted := formatAddress(imap.Address{Name: name, Mailbox: testDisplayMailbox, Host: testDisplayDomain})

			parsed, err := mail.ParseAddress(formatted)
			if err != nil {
				t.Fatalf("reply recipient cannot be parsed: %q: %v", formatted, err)
			}

			if parsed.Address != testDisplayAddress || parsed.Name != name {
				t.Fatalf("address/name changed: %+v", parsed)
			}
		})
	}
}
