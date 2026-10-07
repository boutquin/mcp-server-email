package smtp

import (
	"net/mail"
	"strings"
	"testing"
)

const (
	testParentHeader    = "<parent@example.com>"
	testHeaderRecipient = "recipient@example.com"
)

func TestReplyMessageIDHeaders(t *testing.T) {
	t.Parallel()

	for _, replyID := range []string{"parent@example.com", testParentHeader} {
		t.Run(replyID, func(t *testing.T) {
			t.Parallel()

			raw := buildAndCapture(t, newTestClient(), &SendRequest{
				To: []string{testHeaderRecipient}, Subject: "Reply", Body: "Body",
				InReplyTo: replyID, References: []string{"older@example.com", testParentHeader},
			})

			message, err := mail.ReadMessage(strings.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}

			if message.Header.Get("In-Reply-To") != testParentHeader ||
				message.Header.Get("References") != "<older@example.com> <parent@example.com>" {
				t.Fatalf("reply identifiers: In-Reply-To=%q References=%q",
					message.Header.Get("In-Reply-To"), message.Header.Get("References"))
			}
		})
	}
}
