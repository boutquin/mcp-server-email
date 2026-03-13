package smtp

import "github.com/wneessen/go-mail"

// dialer abstracts the SMTP dial-and-send operation for testability.
type dialer interface {
	DialAndSend(msgs ...*mail.Msg) error
}
