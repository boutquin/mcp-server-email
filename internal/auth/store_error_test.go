package auth //nolint:testpackage // tests access unexported baseDir field

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestNewTokenStore_MkdirAllError(t *testing.T) {
	t.Parallel()

	// Use a path under a file (not a directory) to force MkdirAll to fail.
	tmpDir := t.TempDir()

	blocker := filepath.Join(tmpDir, "blocker")

	err := os.WriteFile(blocker, []byte("x"), 0600)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Trying to create a directory under a file should fail.
	_, err = NewTokenStore(filepath.Join(blocker, "subdir"))
	if err == nil {
		t.Fatal("expected error when MkdirAll fails")
	}

	if !strings.Contains(err.Error(), "create token dir") {
		t.Errorf("error = %q, want 'create token dir' prefix", err.Error())
	}
}

func TestTokenStore_Save_CreateTempError(t *testing.T) {
	t.Parallel()

	// Create a store with a valid directory, then make it read-only.
	dir := t.TempDir()

	store, err := NewTokenStore(dir)
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	// Make directory read-only so CreateTemp fails.
	err = os.Chmod(dir, 0500) //nolint:gosec // G302: intentionally restricted for test
	if err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chmod(dir, 0700) //nolint:gosec // G302: restore for TempDir cleanup
	})

	err = store.Save("acct", &oauth2.Token{AccessToken: "x"})
	if err == nil {
		t.Fatal("expected error when CreateTemp fails")
	}

	if !strings.Contains(err.Error(), "create temp file") {
		t.Errorf("error = %q, want 'create temp file' prefix", err.Error())
	}
}

func TestTokenStore_Save_RenameError(t *testing.T) {
	t.Parallel()

	// Trigger rename error: use an accountID with a path separator so the
	// rename target points to a non-existent subdirectory. CreateTemp
	// succeeds in baseDir, but Rename fails because the target's parent
	// directory doesn't exist.
	tmpDir := t.TempDir()

	store, err := NewTokenStore(tmpDir)
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	// accountID "nodir/acct" → target path is tmpDir/nodir/acct.json
	// tmpDir/nodir/ doesn't exist → os.Rename fails.
	err = store.Save("nodir/acct", &oauth2.Token{AccessToken: "x"})
	if err == nil {
		t.Fatal("expected error when Rename target dir doesn't exist")
	}

	if !strings.Contains(err.Error(), "rename token") {
		t.Errorf("error = %q, want 'rename token' prefix", err.Error())
	}

	// Verify cleanup: no .tmp files left behind.
	entries, readErr := os.ReadDir(tmpDir)
	if readErr != nil {
		t.Fatalf("ReadDir() error = %v", readErr)
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("stale temp file not cleaned up: %s", entry.Name())
		}
	}
}

func TestDefaultTokenDir_ReturnsNonEmpty(t *testing.T) {
	t.Parallel()

	dir, err := DefaultTokenDir()
	if err != nil {
		t.Fatalf("DefaultTokenDir() error = %v", err)
	}

	if dir == "" {
		t.Error("DefaultTokenDir() returned empty string")
	}

	if filepath.Base(dir) != "tokens" {
		t.Errorf("expected path ending in 'tokens', got %q", dir)
	}

	parent := filepath.Base(filepath.Dir(dir))
	if parent != "mcp-server-email" {
		t.Errorf("expected parent dir 'mcp-server-email', got %q", parent)
	}
}
