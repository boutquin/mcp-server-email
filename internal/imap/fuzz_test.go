package imap

import (
	"strings"
	"testing"

	goimap "github.com/emersion/go-imap/v2"
)

func FuzzBuildSearchCriteria(f *testing.F) {
	// Seed corpus: various search parameter combinations.
	// f.Add(query, from, to, since, before)
	f.Add("hello world", "", "", "", "")                                   // simple query
	f.Add("", "", "", "", "")                                              // all empty
	f.Add("test", "sender@example.com", "", "", "")                        // query + from
	f.Add("", "", "recipient@example.com", "", "")                         // to only
	f.Add("", "", "", "2026-01-01", "2026-12-31")                          // date range
	f.Add("search term", "a@b.com", "c@d.com", "2026-01-01", "2026-06-30") // all fields
	f.Add(strings.Repeat("x", 10000), "", "", "", "")                      // very long query
	f.Add("🎉 emoji search", "", "", "", "")                                // unicode query
	f.Add("", "", "", "invalid-date", "")                                  // invalid since
	f.Add("", "", "", "", "not-a-date")                                    // invalid before
	f.Add("", "", "", "2026-13-45", "")                                    // impossible date
	f.Add("special chars: <>&\"'", "", "", "", "")                         // special chars
	f.Add("", "\"Name\" <user@example.com>", "", "", "")                   // display name in from

	f.Fuzz(func(t *testing.T, query, from, to, since, before string) {
		// Must not panic.
		criteria := buildSearchCriteria(query, from, to, since, before)

		// Invariant: criteria should never be nil.
		if criteria == nil {
			t.Error("buildSearchCriteria returned nil")
		}
	})
}

func FuzzExtractContentType(f *testing.F) {
	// Seed corpus: various type/subtype combinations.
	f.Add("text", "plain")
	f.Add("text", "html")
	f.Add("application", "pdf")
	f.Add("multipart", "mixed")
	f.Add("image", "png")
	f.Add("", "")
	f.Add("TEXT", "PLAIN")
	f.Add("Application", "Octet-Stream")
	f.Add(strings.Repeat("x", 1000), strings.Repeat("y", 1000)) // long values
	f.Add("text/plain", "charset=utf-8")                        // malformed
	f.Add("invalid\x00type", "sub\x00type")                     // null bytes
	f.Add("🎉", "📎")                                             // unicode

	f.Fuzz(func(t *testing.T, typ, subtype string) {
		// Test single-part: must not panic.
		sp := &goimap.BodyStructureSinglePart{
			Type:    typ,
			Subtype: subtype,
		}
		_ = extractContentType(sp)

		// Test multipart with fuzzed children: must not panic.
		mp := &goimap.BodyStructureMultiPart{
			Children: []goimap.BodyStructure{
				&goimap.BodyStructureSinglePart{Type: typ, Subtype: subtype},
			},
		}
		_ = extractContentType(mp)

		// Test nested multipart: must not panic.
		nested := &goimap.BodyStructureMultiPart{
			Children: []goimap.BodyStructure{
				&goimap.BodyStructureMultiPart{
					Children: []goimap.BodyStructure{
						&goimap.BodyStructureSinglePart{Type: typ, Subtype: subtype},
					},
				},
			},
		}
		_ = extractContentType(nested)
	})
}

func FuzzExtractAttachmentByIndex(f *testing.F) {
	// Valid multipart message with one attachment.
	validMsg := "From: sender@example.com\r\n" +
		"To: recipient@example.com\r\n" +
		"Subject: Test\r\n" +
		"Content-Type: multipart/mixed; boundary=\"boundary123\"\r\n" +
		"\r\n" +
		"--boundary123\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Hello world\r\n" +
		"--boundary123\r\n" +
		"Content-Type: application/octet-stream\r\n" +
		"Content-Disposition: attachment; filename=\"test.bin\"\r\n" +
		"\r\n" +
		"binary data here\r\n" +
		"--boundary123--\r\n"

	// Minimal message (non-multipart).
	plainMsg := "From: a@b.com\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"body\r\n"

	// Base64-encoded attachment.
	base64Msg := "From: a@b.com\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b\"\r\n" +
		"\r\n" +
		"--b\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"text\r\n" +
		"--b\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=\"doc.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"SGVsbG8gV29ybGQ=\r\n" +
		"--b--\r\n"

	f.Add([]byte(validMsg), 0)
	f.Add([]byte(plainMsg), 0)
	f.Add([]byte(base64Msg), 0)
	f.Add([]byte(""), 0)                          // empty
	f.Add([]byte("not a valid email"), 0)         // invalid
	f.Add([]byte(validMsg), 99)                   // out of range index
	f.Add([]byte(strings.Repeat("x", 10000)), 0)  // large garbage
	f.Add([]byte("From: a@b.com\r\n\r\nbody"), 0) // minimal, missing content-type

	f.Fuzz(func(t *testing.T, data []byte, index int) {
		// Must not panic — errors are expected for malformed input.
		_, _ = extractAttachmentByIndex(data, index)
	})
}
