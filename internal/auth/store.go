package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
)

const (
	tokenDirPerms  = 0700
	tokenFilePerms = 0600
)

// ErrTokenNotFound indicates no token file exists for the account.
var ErrTokenNotFound = errors.New("oauth2 token not found (run device code auth first)")

// TokenStore persists OAuth2 tokens as JSON files.
type TokenStore struct {
	baseDir string
}

// NewTokenStore creates a TokenStore, ensuring the base directory exists.
func NewTokenStore(baseDir string) (*TokenStore, error) {
	err := os.MkdirAll(baseDir, tokenDirPerms)
	if err != nil {
		return nil, fmt.Errorf("create token dir %s: %w", baseDir, err)
	}

	return &TokenStore{baseDir: baseDir}, nil
}

// Path returns the token file path for the given account.
func (s *TokenStore) Path(accountID string) string {
	return filepath.Join(s.baseDir, accountID+".json")
}

// Load reads a token from disk for the given account.
func (s *TokenStore) Load(accountID string) (*oauth2.Token, error) {
	path := s.Path(accountID)

	data, err := os.ReadFile(path) //nolint:gosec // token files have restricted perms
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrTokenNotFound
		}

		return nil, fmt.Errorf("read token %s: %w", path, err)
	}

	var token oauth2.Token

	err = json.Unmarshal(data, &token)
	if err != nil {
		return nil, fmt.Errorf("parse token %s: %w", path, err)
	}

	return &token, nil
}

// Save persists a token to disk using atomic write (temp file + rename).
func (s *TokenStore) Save(accountID string, token *oauth2.Token) error {
	path := s.Path(accountID)

	data, err := json.Marshal(token) //nolint:gosec // G117: fields named after OAuth2 spec, not credentials
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	// Write to temp file in the same directory for atomic rename.
	tmpFile, err := os.CreateTemp(s.baseDir, ".token-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpPath := tmpFile.Name()

	_, err = tmpFile.Write(data)

	closeErr := tmpFile.Close()
	if closeErr != nil && err == nil {
		err = closeErr
	}

	if err != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("write token: %w", err)
	}

	// Set restricted permissions before rename.
	err = os.Chmod(tmpPath, tokenFilePerms)
	if err != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("chmod token: %w", err)
	}

	err = os.Rename(tmpPath, path)
	if err != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("rename token: %w", err)
	}

	return nil
}

// Delete removes the token file for the given account.
func (s *TokenStore) Delete(accountID string) error {
	path := s.Path(accountID)

	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete token %s: %w", path, err)
	}

	return nil
}

// DefaultTokenDir returns the default token storage directory.
func DefaultTokenDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine config dir: %w", err)
	}

	return filepath.Join(configDir, "mcp-server-email", "tokens"), nil
}
