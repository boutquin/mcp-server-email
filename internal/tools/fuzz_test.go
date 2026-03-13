package tools //nolint:testpackage // fuzz tests need access to unexported htmlToText

import (
	"strings"
	"testing"
)

func FuzzHtmlToText(f *testing.F) {
	// Seed corpus: valid HTML, edge cases, malformed input.
	f.Add("<html><body><p>Hello World</p></body></html>") // basic HTML
	f.Add("")                                             // empty
	f.Add("<div><div><div>nested</div></div></div>")      // nested divs
	f.Add("<script>alert('xss')</script><p>safe</p>")     // script tag
	f.Add("<style>body{color:red}</style><p>text</p>")    // style tag
	f.Add("<table><tr><td>cell</td></tr></table>")        // table
	f.Add("<p>unclosed paragraph")                        // unclosed tag
	f.Add("plain text with no HTML")                      // plain text
	f.Add("&amp;&lt;&gt;&quot;")                          // HTML entities
	f.Add(strings.Repeat("<div>", 1000))                  // deeply nested
	f.Add("<a href='http://example.com'>link</a>")        // link
	f.Add("🎉 <b>bold emoji</b>")                          // unicode
	f.Add("<br/><br/><br/>")                              // self-closing tags

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic.
		result := htmlToText(input)

		// Basic invariant: result should not be longer than input
		// (HTML tags are stripped, so output ≤ input length for valid HTML).
		// Note: entities like &amp; expand, and TrimSpace may add/remove chars,
		// so we don't enforce strict length — just no panic.
		_ = result
	})
}

func FuzzSplitAddresses(f *testing.F) {
	// Seed corpus: various address formats.
	f.Add("user@example.com")                          // single
	f.Add("a@example.com, b@example.com")              // comma-separated
	f.Add("")                                          // empty
	f.Add(",,,")                                       // only commas
	f.Add("  user@example.com  ")                      // whitespace
	f.Add("\"Display Name\" <user@example.com>")       // display name
	f.Add(strings.Repeat("a@b.com,", 1000))            // many addresses
	f.Add("🎉@example.com")                             // unicode
	f.Add("user@example.com,  ,  , other@example.com") // sparse commas
	f.Add("a,b,c,d,e")                                 // short entries

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic.
		result := SplitAddresses(input)

		// Invariant: every result element should be non-empty and trimmed.
		for i, addr := range result {
			if addr == "" {
				t.Errorf("SplitAddresses(%q)[%d] is empty", input, i)
			}

			if addr != strings.TrimSpace(addr) {
				t.Errorf("SplitAddresses(%q)[%d] = %q has untrimmed whitespace", input, i, addr)
			}
		}
	})
}
