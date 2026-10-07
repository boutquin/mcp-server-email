package smtp

import (
	"crypto/rand"
	"strings"

	"github.com/wneessen/go-mail"
)

// SetMessageID assigns a unique RFC message identifier after From is validated.
// The random token is a single atom, avoiding the leading/trailing dots that
// can occur in the go-mail v0.6.2 default generator.
func SetMessageID(message *mail.Msg) {
	from := message.GetFrom()[0].Address
	domain := from[strings.LastIndex(from, "@")+1:]
	message.SetMessageIDWithValue(rand.Text() + "@" + domain)
}

func setThreadingHeaders(message *mail.Msg, req *SendRequest) {
	if req.InReplyTo != "" {
		message.SetGenHeader(mail.HeaderInReplyTo, formatMessageID(req.InReplyTo))
	}

	refs := make([]string, 0, len(req.References))
	for _, id := range req.References {
		if formatted := formatMessageID(id); formatted != "" {
			refs = append(refs, formatted)
		}
	}

	if len(refs) > 0 {
		message.SetGenHeader(mail.HeaderReferences, strings.Join(refs, " "))
	}
}

func formatMessageID(id string) string {
	id = strings.Trim(strings.TrimSpace(id), "<>")
	if id == "" {
		return ""
	}

	return "<" + id + ">"
}
