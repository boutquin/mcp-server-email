package models

import (
	"strings"
	"testing"
)

func FuzzParseMessageID(f *testing.F) {
	// Seed corpus: known good and edge cases.
	f.Add("default:INBOX:123")           // valid
	f.Add("")                            // empty
	f.Add("no-colons")                   // no separators
	f.Add("a:b:0")                       // zero uid
	f.Add("a:b:4294967295")              // max uint32
	f.Add("a:b:c:d:e")                   // extra colons (mailbox with colons)
	f.Add("account:INBOX.With.Dots:999") // dotted mailbox
	f.Add(strings.Repeat("x", 10000))    // very long input
	f.Add("🎉:📧:42")                      // unicode
	f.Add("a:b:4294967296")              // overflow uint32
	f.Add("a:b:-1")                      // negative uid
	f.Add("::1")                         // empty account and mailbox
	f.Add("a::1")                        // empty mailbox
	f.Add(":b:1")                        // empty account

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic.
		account, folder, uid, err := ParseMessageID(input)
		if err != nil {
			return // errors are fine
		}

		// If parse succeeded, round-trip should work.
		formatted := FormatMessageID(account, folder, uid)

		a2, f2, u2, err2 := ParseMessageID(formatted)
		if err2 != nil {
			t.Errorf("round-trip failed: %q → %q → error: %v", input, formatted, err2)
		}

		if a2 != account || f2 != folder || u2 != uid {
			t.Errorf(
				"round-trip mismatch: got (%s,%s,%d), want (%s,%s,%d)",
				a2, f2, u2, account, folder, uid,
			)
		}
	})
}
