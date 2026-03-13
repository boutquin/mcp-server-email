package tools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/tools"
)

const (
	defaultMaxFileBytes  = 18 * 1024 * 1024
	defaultMaxTotalBytes = 18 * 1024 * 1024
)

func TestValidateAttachments_MissingPath(t *testing.T) {
	t.Parallel()

	err := tools.ValidateAttachments(
		[]tools.AttachmentInput{{Path: ""}}, defaultMaxFileBytes, defaultMaxTotalBytes,
	)
	if err == nil {
		t.Fatal("expected error for missing path")
	}

	if !strings.Contains(err.Error(), "path is required") {
		t.Errorf("expected 'path is required' error, got %q", err)
	}
}

func TestValidateAttachments_RelativePath(t *testing.T) {
	t.Parallel()

	err := tools.ValidateAttachments(
		[]tools.AttachmentInput{{Path: "relative/file.txt"}}, defaultMaxFileBytes, defaultMaxTotalBytes,
	)
	if err == nil {
		t.Fatal("expected error for relative path")
	}

	if !strings.Contains(err.Error(), "must be absolute") {
		t.Errorf("expected 'must be absolute' error, got %q", err)
	}
}

func TestValidateAttachments_NonExistent(t *testing.T) {
	t.Parallel()

	err := tools.ValidateAttachments(
		[]tools.AttachmentInput{{Path: "/tmp/nonexistent-attachment-test-xyz.txt"}},
		defaultMaxFileBytes, defaultMaxTotalBytes,
	)
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}

	if !strings.Contains(err.Error(), "file not found") {
		t.Errorf("expected 'file not found' error, got %q", err)
	}
}

func TestValidateAttachments_FileTooLarge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")

	f, err := os.Create(path) //nolint:gosec // test file in temp dir
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	// Truncate to just over 18 MB (sparse — no disk space used).
	truncErr := f.Truncate(defaultMaxFileBytes + 1)
	if truncErr != nil {
		_ = f.Close()

		t.Fatalf("truncate: %v", truncErr)
	}

	_ = f.Close()

	err = tools.ValidateAttachments(
		[]tools.AttachmentInput{{Path: path}}, defaultMaxFileBytes, defaultMaxTotalBytes,
	)
	if err == nil {
		t.Fatal("expected error for file exceeding 18 MB")
	}

	if !strings.Contains(err.Error(), "exceeds 18 MB limit") {
		t.Errorf("expected 'exceeds 18 MB limit' error, got %q", err)
	}
}

func TestValidateAttachments_TotalTooLarge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create two files, each 10 MB (total 20 MB > 18 MB limit).
	size := int64(10 * 1024 * 1024)

	path1 := filepath.Join(dir, "file1.bin")
	path2 := filepath.Join(dir, "file2.bin")

	for _, p := range []string{path1, path2} {
		f, fErr := os.Create(p) //nolint:gosec // test file in temp dir
		if fErr != nil {
			t.Fatalf("create temp file: %v", fErr)
		}

		truncErr := f.Truncate(size)
		if truncErr != nil {
			_ = f.Close()

			t.Fatalf("truncate: %v", truncErr)
		}

		_ = f.Close()
	}

	err := tools.ValidateAttachments(
		[]tools.AttachmentInput{{Path: path1}, {Path: path2}},
		defaultMaxFileBytes, defaultMaxTotalBytes,
	)
	if err == nil {
		t.Fatal("expected error for total size exceeding 18 MB")
	}

	if !strings.Contains(err.Error(), "total attachment size") {
		t.Errorf("expected 'total attachment size' error, got %q", err)
	}
}

func TestValidateAttachments_HappyPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	err := os.WriteFile(path, []byte("hello"), 0o644)
	if err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	err = tools.ValidateAttachments(
		[]tools.AttachmentInput{{Path: path}}, defaultMaxFileBytes, defaultMaxTotalBytes,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAttachments_CustomLimits(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "small.bin")

	f, err := os.Create(path) //nolint:gosec // test file in temp dir
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	// 2 MB file
	truncErr := f.Truncate(2 * 1024 * 1024)
	if truncErr != nil {
		_ = f.Close()

		t.Fatalf("truncate: %v", truncErr)
	}

	_ = f.Close()

	// 1 MB per-file limit → should fail
	err = tools.ValidateAttachments(
		[]tools.AttachmentInput{{Path: path}}, 1*1024*1024, 10*1024*1024,
	)
	if err == nil {
		t.Fatal("expected error for file exceeding custom 1 MB limit")
	}

	if !strings.Contains(err.Error(), "exceeds 1 MB limit") {
		t.Errorf("expected 'exceeds 1 MB limit' error, got %q", err)
	}

	// 5 MB per-file limit → should pass
	err = tools.ValidateAttachments(
		[]tools.AttachmentInput{{Path: path}}, 5*1024*1024, 10*1024*1024,
	)
	if err != nil {
		t.Fatalf("unexpected error with 5 MB limit: %v", err)
	}
}
