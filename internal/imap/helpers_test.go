package imap

import (
	"testing"

	"github.com/emersion/go-imap/v2/imapclient"
)

// ==================== extractRawBody ====================

func TestExtractRawBody_NonEmpty(t *testing.T) {
	t.Parallel()

	sections := []imapclient.FetchBodySectionBuffer{
		{Bytes: nil},
		{Bytes: []byte("hello world")},
	}

	result := extractRawBody(sections)
	if string(result) != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", string(result))
	}
}

func TestExtractRawBody_AllEmpty(t *testing.T) {
	t.Parallel()

	sections := []imapclient.FetchBodySectionBuffer{
		{Bytes: nil},
		{Bytes: []byte{}},
	}

	result := extractRawBody(sections)
	if result != nil {
		t.Errorf("expected nil, got %q", string(result))
	}
}

func TestExtractRawBody_NilInput(t *testing.T) {
	t.Parallel()

	result := extractRawBody(nil)
	if result != nil {
		t.Errorf("expected nil, got %q", string(result))
	}
}

func TestExtractRawBody_FirstNonEmpty(t *testing.T) {
	t.Parallel()

	sections := []imapclient.FetchBodySectionBuffer{
		{Bytes: []byte("first")},
		{Bytes: []byte("second")},
	}

	result := extractRawBody(sections)
	if string(result) != "first" {
		t.Errorf("expected %q, got %q", "first", string(result))
	}
}
