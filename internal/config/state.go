package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const stateDir = "state"

// SaveState atomically replaces a named JSON blob in a scope and namespace.
// An empty scope stores state directly under its namespace. Files are private (0600).
func SaveState(configDir, scope, namespace, key string, data json.RawMessage) error {
	return writeState(configDir, scope, namespace, key, data, false)
}

// CreateState atomically creates a blob without overwriting an existing entry.
// A duplicate returns an error wrapping os.ErrExist.
func CreateState(configDir, scope, namespace, key string, data json.RawMessage) error {
	return writeState(configDir, scope, namespace, key, data, true)
}

func writeState(configDir, scope, namespace, key string, data json.RawMessage, exclusive bool) error {
	if !json.Valid(data) {
		return fmt.Errorf("state payload is not valid JSON")
	}
	dir := stateDirPath(configDir, scope, namespace)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	file, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return fmt.Errorf("create state file: %w", err)
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write state: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close state: %w", closeErr)
	}
	path := stateFilePath(configDir, scope, namespace, key)
	if exclusive {
		err = os.Link(file.Name(), path)
	} else {
		err = os.Rename(file.Name(), path)
	}
	if err != nil {
		return fmt.Errorf("publish state: %w", err)
	}

	return nil
}

// LoadState reads a named state blob. Returns nil if it doesn't exist.
func LoadState(configDir, scope, namespace, key string) (json.RawMessage, error) {
	path := stateFilePath(configDir, scope, namespace, key)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state %s/%s/%s: %w", scope, namespace, key, err)
	}

	return json.RawMessage(data), nil
}

// DeleteState removes a single state entry. No error if it doesn't exist.
func DeleteState(configDir, scope, namespace, key string) error {
	path := stateFilePath(configDir, scope, namespace, key)

	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ListStates returns the keys in a namespace (without .json extension).
func ListStates(configDir, scope, namespace string) ([]string, error) {
	dir := stateDirPath(configDir, scope, namespace)

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list state %s/%s: %w", scope, namespace, err)
	}

	var keys []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			keys = append(keys, strings.TrimSuffix(name, ".json"))
		}
	}

	return keys, nil
}

// stateDirPath returns a namespace directory, optionally nested under a scope.
func stateDirPath(configDir, scope, namespace string) string {
	dir := filepath.Join(configDir, stateDir)
	if scope != "" {
		dir = filepath.Join(dir, sanitizeKey(scope))
	}

	return filepath.Join(dir, sanitizeKey(namespace))
}

// stateFilePath returns the full path for a state entry.
func stateFilePath(configDir, scope, namespace, key string) string {
	return filepath.Join(configDir, stateDir, sanitizeKey(scope), sanitizeKey(namespace), sanitizeKey(key)+".json")
}

// sanitizeKey prevents path traversal in state keys.
func sanitizeKey(s string) string {
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, s)
	safe = strings.TrimLeft(safe, ".")
	if safe == "" {
		safe = "_"
	}
	return safe
}
