package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saucesteals/shop"
)

const registryFile = "registry.json"

// Registry is the persisted store-to-provider mapping.
type Registry struct {
	Version int                  `json:"version"`
	Stores  []shop.RegistryEntry `json:"stores"`
}

// LoadRegistry reads the registry from the given directory. If the file
// doesn't exist, returns the default registry with built-in stores.
func LoadRegistry(dir string) (*Registry, error) {
	path := filepath.Join(dir, registryFile)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultRegistry(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}

	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}

	return &reg, nil
}

// SaveRegistry writes the registry to the given directory.
func SaveRegistry(dir string, reg *Registry) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}

	path := filepath.Join(dir, registryFile)

	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Lookup finds a registry entry by alias or domain. Returns nil if not found.
func (r *Registry) Lookup(value string) *shop.RegistryEntry {
	normalized := normalizeDomain(value)

	for i := range r.Stores {
		entry := &r.Stores[i]
		if entry.Alias == value || entry.Domain == normalized {
			return entry
		}
	}

	return nil
}

// Add adds or updates a registry entry. If an entry with the same domain
// already exists, it is updated.
func (r *Registry) Add(entry shop.RegistryEntry) {
	for i := range r.Stores {
		if r.Stores[i].Domain == entry.Domain {
			r.Stores[i] = entry

			return
		}
	}

	r.Stores = append(r.Stores, entry)
}

// FilterByProvider returns entries matching the given provider name.
func (r *Registry) FilterByProvider(provider string) []shop.RegistryEntry {
	var result []shop.RegistryEntry
	for _, entry := range r.Stores {
		if entry.Provider == provider {
			result = append(result, entry)
		}
	}

	return result
}

// DefaultRegistry returns the pre-seeded registry with built-in stores.
func DefaultRegistry() *Registry {
	return &Registry{
		Version: 1,
		Stores: []shop.RegistryEntry{
			{
				Alias:    "amazon",
				Domain:   "amazon.com",
				Provider: "amazon",
				Name:     "Amazon US",
				Country:  "US",
				Currency: "USD",
				BuiltIn:  true,
			},
		},
	}
}

// normalizeDomain strips protocol, www prefix, paths, and lowercases.
func normalizeDomain(s string) string {
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")

	if idx := strings.IndexByte(s, '/'); idx >= 0 {
		s = s[:idx]
	}

	return strings.ToLower(s)
}
