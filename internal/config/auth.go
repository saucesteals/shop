package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const authDir = "auth"

// LoadAuth reads the auth state for a given store handle from the config directory.
// Returns nil if no auth file exists (not authenticated).
func LoadAuth(configDir, handle string) (json.RawMessage, error) {
	path := authPath(configDir, handle)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read auth for %s: %w", handle, err)
	}

	return json.RawMessage(data), nil
}

// SaveAuth writes the auth state for a given store handle. The data is
// provider-defined — the config layer treats it as opaque JSON.
// Files are written with 0600 permissions.
func SaveAuth(configDir, handle string, data json.RawMessage) error {
	dir := filepath.Join(configDir, authDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create auth dir: %w", err)
	}

	path := authPath(configDir, handle)

	return os.WriteFile(path, data, 0o600)
}

// DeleteAuth removes the auth file for a given store handle.
func DeleteAuth(configDir, handle string) error {
	path := authPath(configDir, handle)

	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}

	return err
}

// authPath returns the filesystem path for a store's auth file. The handle
// is sanitized to prevent path traversal — slashes, backslashes, and leading
// dots are replaced with underscores.
func authPath(configDir, handle string) string {
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' {
			return '_'
		}

		return r
	}, handle)
	// Strip leading dots to prevent hidden files or "../" after slash removal.
	safe = strings.TrimLeft(safe, ".")
	if safe == "" {
		safe = "_"
	}

	return filepath.Join(configDir, authDir, safe+".json")
}
