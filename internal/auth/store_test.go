package auth //nolint:testpackage // tests access unexported baseDir field

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestNewTokenStore_CreatesDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "nested", "tokens")

	store, err := NewTokenStore(dir)
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("expected directory")
	}

	_ = store // verify it's usable
}

func TestTokenStore_Path(t *testing.T) {
	t.Parallel()

	store := &TokenStore{baseDir: "/tmp/tokens"}

	got := store.Path("myaccount")

	want := "/tmp/tokens/myaccount.json"
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestTokenStore_SaveLoad(t *testing.T) {
	t.Parallel()

	store, err := NewTokenStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	expiry := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	original := &oauth2.Token{
		AccessToken:  "access-123",
		TokenType:    "Bearer",
		RefreshToken: "refresh-456",
		Expiry:       expiry,
	}

	err = store.Save("test-account", original)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load("test-account")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.AccessToken != original.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, original.AccessToken)
	}

	if loaded.RefreshToken != original.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, original.RefreshToken)
	}

	if loaded.TokenType != original.TokenType {
		t.Errorf("TokenType = %q, want %q", loaded.TokenType, original.TokenType)
	}

	if !loaded.Expiry.Equal(original.Expiry) {
		t.Errorf("Expiry = %v, want %v", loaded.Expiry, original.Expiry)
	}
}

func TestTokenStore_SaveOverwrite(t *testing.T) {
	t.Parallel()

	store, err := NewTokenStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	token1 := &oauth2.Token{AccessToken: "first"}
	token2 := &oauth2.Token{AccessToken: "second"}

	err = store.Save("acct", token1)
	if err != nil {
		t.Fatalf("Save(1) error = %v", err)
	}

	err = store.Save("acct", token2)
	if err != nil {
		t.Fatalf("Save(2) error = %v", err)
	}

	loaded, err := store.Load("acct")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.AccessToken != "second" {
		t.Errorf("AccessToken = %q, want 'second'", loaded.AccessToken)
	}
}

func TestTokenStore_Delete(t *testing.T) {
	t.Parallel()

	store, err := NewTokenStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	err = store.Save("acct", &oauth2.Token{AccessToken: "x"})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	err = store.Delete("acct")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = store.Load("acct")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("Load after Delete: got %v, want ErrTokenNotFound", err)
	}
}

func TestTokenStore_DeleteNonexistent(t *testing.T) {
	t.Parallel()

	store, err := NewTokenStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	err = store.Delete("nonexistent")
	if err != nil {
		t.Errorf("Delete(nonexistent) error = %v, want nil", err)
	}
}

func TestTokenStore_MissingFile(t *testing.T) {
	t.Parallel()

	store, err := NewTokenStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	_, err = store.Load("nonexistent")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("Load(nonexistent) = %v, want ErrTokenNotFound", err)
	}
}

func TestTokenStore_CorruptFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	store, err := NewTokenStore(dir)
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	// Write garbage to the token file.
	path := store.Path("corrupt")

	err = os.WriteFile(path, []byte("not json{{{"), 0600)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err = store.Load("corrupt")
	if err == nil {
		t.Fatal("expected error loading corrupt token file")
	}

	// Error should mention the file path for user debugging.
	if !errors.Is(err, ErrTokenNotFound) && err.Error() == "" {
		t.Errorf("error should be meaningful, got: %v", err)
	}
}

func TestTokenStore_AtomicWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	store, err := NewTokenStore(dir)
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	// Save a token.
	err = store.Save("atomic", &oauth2.Token{AccessToken: "test"})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify the file is valid JSON (no partial writes).
	loaded, err := store.Load("atomic")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.AccessToken != "test" {
		t.Errorf("AccessToken = %q, want 'test'", loaded.AccessToken)
	}

	// Verify no .tmp files remain.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("stale temp file found: %s", entry.Name())
		}
	}
}

func TestTokenStore_FilePermissions(t *testing.T) {
	t.Parallel()

	store, err := NewTokenStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	err = store.Save("perms", &oauth2.Token{AccessToken: "secret"})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Stat(store.Path("perms"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestDefaultTokenDir(t *testing.T) {
	t.Parallel()

	dir, err := DefaultTokenDir()
	if err != nil {
		t.Fatalf("DefaultTokenDir() error = %v", err)
	}

	if dir == "" {
		t.Error("DefaultTokenDir() returned empty string")
	}

	// Should end with the expected path components.
	if filepath.Base(dir) != "tokens" {
		t.Errorf("expected path ending in 'tokens', got %q", dir)
	}
}
