package imap

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// buildMultipartMsg constructs a raw RFC 5322 multipart/mixed message for testing.
func buildMultipartMsg(boundary string, parts []string) string {
	var b strings.Builder

	b.WriteString("From: test@example.com\r\n")
	b.WriteString("To: recipient@example.com\r\n")
	b.WriteString("Subject: Test\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n")
	b.WriteString("\r\n")

	for _, p := range parts {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString(p)
		b.WriteString("\r\n")
	}

	b.WriteString("--" + boundary + "--\r\n")

	return b.String()
}

func TestExtractAttachmentByIndex_SingleAttachment(t *testing.T) {
	t.Parallel()

	body := "This is the attachment content"
	msg := buildMultipartMsg("TESTBOUNDARY", []string{
		"Content-Type: text/plain\r\nContent-Disposition: inline\r\n\r\nHello body",
		"Content-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"doc.pdf\"\r\n\r\n" + body,
	})

	data, err := extractAttachmentByIndex([]byte(msg), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != body {
		t.Errorf("got %q, want %q", string(data), body)
	}
}

func TestExtractAttachmentByIndex_MultipleAttachments(t *testing.T) {
	t.Parallel()

	attachments := []string{"content-A", "content-B", "content-C"}
	parts := make([]string, 1, 1+len(attachments))
	parts[0] = "Content-Type: text/plain\r\nContent-Disposition: inline\r\n\r\nBody text"

	for i, content := range attachments {
		parts = append(parts,
			"Content-Type: application/octet-stream\r\n"+
				"Content-Disposition: attachment; filename=\"file"+string(rune('0'+i))+".dat\"\r\n\r\n"+
				content)
	}

	msg := buildMultipartMsg("MULTI", parts)

	for i, want := range attachments {
		data, err := extractAttachmentByIndex([]byte(msg), i)
		if err != nil {
			t.Fatalf("index %d: unexpected error: %v", i, err)
		}

		if string(data) != want {
			t.Errorf("index %d: got %q, want %q", i, string(data), want)
		}
	}
}

func TestExtractAttachmentByIndex_NestedMultipart(t *testing.T) {
	t.Parallel()

	// multipart/mixed containing multipart/alternative (text+html) + attachment
	innerBoundary := "INNER"
	inner := "Content-Type: multipart/alternative; boundary=\"" + innerBoundary + "\"\r\n\r\n" +
		"--" + innerBoundary + "\r\n" +
		"Content-Type: text/plain\r\n\r\nPlain text\r\n" +
		"--" + innerBoundary + "\r\n" +
		"Content-Type: text/html\r\n\r\n<p>HTML</p>\r\n" +
		"--" + innerBoundary + "--\r\n"

	msg := buildMultipartMsg("OUTER", []string{
		inner,
		"Content-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"nested.pdf\"\r\n\r\nPDF content",
	})

	data, err := extractAttachmentByIndex([]byte(msg), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != "PDF content" {
		t.Errorf("got %q, want %q", string(data), "PDF content")
	}
}

func TestExtractAttachmentByIndex_DeeplyNested(t *testing.T) {
	t.Parallel()

	// 3 levels: outer -> middle -> inner, with attachment at innermost level.
	raw := "From: a@b.com\r\nTo: c@d.com\r\nSubject: Deep\r\n" +
		"Content-Type: multipart/mixed; boundary=\"L1\"\r\n\r\n" +
		"--L1\r\n" +
		"Content-Type: multipart/mixed; boundary=\"L2\"\r\n\r\n" +
		"--L2\r\n" +
		"Content-Type: multipart/mixed; boundary=\"L3\"\r\n\r\n" +
		"--L3\r\n" +
		"Content-Type: application/octet-stream\r\n" +
		"Content-Disposition: attachment; filename=\"deep.bin\"\r\n\r\n" +
		"deep-data\r\n" +
		"--L3--\r\n" +
		"\r\n" +
		"--L2--\r\n" +
		"\r\n" +
		"--L1--\r\n"

	data, err := extractAttachmentByIndex([]byte(raw), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(string(data), "deep-data") {
		t.Errorf("got %q, want content containing 'deep-data'", string(data))
	}
}

func TestExtractAttachmentByIndex_InvalidIndex(t *testing.T) {
	t.Parallel()

	msg := buildMultipartMsg("BOUND", []string{
		"Content-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"a.pdf\"\r\n\r\ndata",
	})

	_, err := extractAttachmentByIndex([]byte(msg), 5)
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}

	if !errors.Is(err, errAttachmentNotFound) {
		t.Errorf("expected errAttachmentNotFound, got: %v", err)
	}
}

func TestExtractAttachmentByIndex_NegativeIndex(t *testing.T) {
	t.Parallel()

	msg := buildMultipartMsg("BOUND", []string{
		"Content-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"a.pdf\"\r\n\r\ndata",
	})

	_, err := extractAttachmentByIndex([]byte(msg), -1)
	if err == nil {
		t.Fatal("expected error for negative index")
	}
}

func TestExtractAttachmentByIndex_SkipInline(t *testing.T) {
	t.Parallel()

	msg := buildMultipartMsg("SKIPINLINE", []string{
		"Content-Type: image/png\r\nContent-Disposition: inline; filename=\"logo.png\"\r\n\r\ninline-image",
		"Content-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"doc.pdf\"\r\n\r\nattach-content",
	})

	// Index 0 should be the attachment, not the inline part.
	data, err := extractAttachmentByIndex([]byte(msg), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != "attach-content" {
		t.Errorf("got %q, want %q", string(data), "attach-content")
	}

	// Index 1 should not exist (only 1 attachment).
	_, err = extractAttachmentByIndex([]byte(msg), 1)
	if err == nil {
		t.Error("expected error for index 1 (only 1 attachment)")
	}
}

func TestExtractAttachmentByIndex_NoFilename(t *testing.T) {
	t.Parallel()

	msg := buildMultipartMsg("NONAME", []string{
		"Content-Type: application/octet-stream\r\nContent-Disposition: attachment\r\n\r\nno-name-data",
	})

	data, err := extractAttachmentByIndex([]byte(msg), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != "no-name-data" {
		t.Errorf("got %q, want %q", string(data), "no-name-data")
	}
}

func TestExtractAttachmentByIndex_NotMultipart(t *testing.T) {
	t.Parallel()

	raw := "From: a@b.com\r\nTo: c@d.com\r\nSubject: Plain\r\n" +
		"Content-Type: text/plain\r\n\r\nJust text."

	_, err := extractAttachmentByIndex([]byte(raw), 0)
	if err == nil {
		t.Fatal("expected error for non-multipart message")
	}

	if !errors.Is(err, errNotMultipart) {
		t.Errorf("expected errNotMultipart, got: %v", err)
	}
}

func TestExtractAttachmentByIndex_MalformedBoundary(t *testing.T) {
	t.Parallel()

	// Message with declared boundary that doesn't appear in body.
	raw := "From: a@b.com\r\nTo: c@d.com\r\nSubject: Bad\r\n" +
		"Content-Type: multipart/mixed; boundary=\"DECLARED\"\r\n\r\n" +
		"--WRONG\r\n" +
		"Content-Type: text/plain\r\n\r\nOrphan part\r\n" +
		"--WRONG--\r\n"

	_, err := extractAttachmentByIndex([]byte(raw), 0)
	if err == nil {
		t.Fatal("expected error for malformed boundary")
	}
}

func TestExtractAttachmentByIndex_NoBoundary(t *testing.T) {
	t.Parallel()

	// Multipart content type but no boundary parameter.
	raw := "From: a@b.com\r\nTo: c@d.com\r\nSubject: NoBound\r\n" +
		"Content-Type: multipart/mixed\r\n\r\n" +
		"Some body\r\n"

	_, err := extractAttachmentByIndex([]byte(raw), 0)
	if err == nil {
		t.Fatal("expected error for missing boundary")
	}

	if !errors.Is(err, errNoBoundary) {
		t.Errorf("expected errNoBoundary, got: %v", err)
	}
}

func TestExtractAttachmentByIndex_InvalidMessage(t *testing.T) {
	t.Parallel()

	// Totally invalid RFC 5322 — no headers at all.
	_, err := extractAttachmentByIndex([]byte("not a valid message"), 0)
	if err == nil {
		t.Fatal("expected error for invalid message")
	}
}

func TestTryNestedMultipart_ParseError(t *testing.T) {
	t.Parallel()

	// Nested multipart with a declared boundary but corrupted body.
	// This exercises the walkMultipartForAttachment error path inside tryNestedMultipart.
	raw := "From: a@b.com\r\nTo: c@d.com\r\nSubject: Nest\r\n" +
		"Content-Type: multipart/mixed; boundary=\"OUTER\"\r\n\r\n" +
		"--OUTER\r\n" +
		"Content-Type: multipart/mixed; boundary=\"INNER\"\r\n\r\n" +
		"corrupted inner body with no proper parts\r\n" +
		"--OUTER--\r\n"

	// Should not panic; attachment not found at index 0.
	_, err := extractAttachmentByIndex([]byte(raw), 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTryNestedMultipart_InvalidContentType(t *testing.T) {
	t.Parallel()

	// Nested part with multipart/ prefix but unparseable content-type parameters.
	raw := "From: a@b.com\r\nTo: c@d.com\r\nSubject: BadCT\r\n" +
		"Content-Type: multipart/mixed; boundary=\"OUTER\"\r\n\r\n" +
		"--OUTER\r\n" +
		"Content-Type: multipart/mixed; <<<invalid>>>\r\n\r\n" +
		"some body\r\n" +
		"--OUTER--\r\n"

	// Should not panic; falls through to attachment-not-found.
	_, err := extractAttachmentByIndex([]byte(raw), 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractAttachmentByIndex_Base64Encoded(t *testing.T) {
	t.Parallel()

	plaintext := "Hello, this is base64-encoded attachment data!"
	encoded := base64.StdEncoding.EncodeToString([]byte(plaintext))

	msg := buildMultipartMsg("B64BOUND", []string{
		"Content-Type: text/plain\r\nContent-Disposition: inline\r\n\r\nBody text",
		"Content-Type: application/octet-stream\r\n" +
			"Content-Disposition: attachment; filename=\"encoded.bin\"\r\n" +
			"Content-Transfer-Encoding: base64\r\n\r\n" + encoded,
	})

	data, err := extractAttachmentByIndex([]byte(msg), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != plaintext {
		t.Errorf("got %q, want %q", string(data), plaintext)
	}
}

func TestExtractAttachmentByIndex_QuotedPrintable(t *testing.T) {
	t.Parallel()

	// Quoted-printable encodes = as =3D
	qpBody := "Hello =3D World"
	want := "Hello = World"

	msg := buildMultipartMsg("QPBOUND", []string{
		"Content-Type: text/plain\r\nContent-Disposition: inline\r\n\r\nBody",
		"Content-Type: text/plain\r\n" +
			"Content-Disposition: attachment; filename=\"qp.txt\"\r\n" +
			"Content-Transfer-Encoding: quoted-printable\r\n\r\n" + qpBody,
	})

	data, err := extractAttachmentByIndex([]byte(msg), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestIsAttachmentPart_Inline(t *testing.T) {
	t.Parallel()

	msg := buildMultipartMsg("ISATTACH", []string{
		"Content-Type: image/png\r\nContent-Disposition: inline\r\n\r\ndata",
	})

	// Parse just to check isAttachmentPart returns false for inline.
	// We test indirectly through extractAttachmentByIndex, but also
	// verify that inline parts don't match.
	_, err := extractAttachmentByIndex([]byte(msg), 0)
	if err == nil {
		t.Error("expected error: inline part should not be counted as attachment")
	}
}
