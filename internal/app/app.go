// Package app wires providers, config, and the store registry together.
// It implements the store resolution chain described in the design doc.
package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/config"
)

// App is the central coordinator. It holds configuration, the store registry,
// and performs store resolution against registered providers.
type App struct {
	Config    *config.Config
	Registry  *config.Registry
	ConfigDir string
}

// New creates an App by loading config and registry from the given directory.
// On first run, it creates the directory and writes default files.
func New(configDir string) (*App, error) {
	if err := ensureDir(configDir); err != nil {
		return nil, shop.Errorf(shop.ErrConfigError, "create config directory: %v", err)
	}

	cfg, err := config.Load(configDir)
	if err != nil {
		return nil, shop.Errorf(shop.ErrConfigError, "load config: %v", err)
	}

	reg, err := config.LoadRegistry(configDir)
	if err != nil {
		return nil, shop.Errorf(shop.ErrConfigError, "load registry: %v", err)
	}

	// First-run: write default files if they don't exist.
	if err := config.EnsureDefaults(configDir, cfg, reg); err != nil {
		return nil, shop.Errorf(shop.ErrConfigError, "write default config: %v", err)
	}

	a := &App{
		Config:    cfg,
		Registry:  reg,
		ConfigDir: configDir,
	}

	// Wire the global resolver so shop.Resolve() works.
	shop.SetResolver(a.Resolve)

	return a, nil
}

// Resolve takes a --store value and returns a ready Store instance.
// Resolution chain:
//  1. Exact match in registry (alias or domain)
//  2. Domain normalization + registry lookup
//  3. Auto-discovery via provider Detect() in cost order
//  4. Fail with ErrStoreNotFound
func (a *App) Resolve(ctx context.Context, storeValue string) (shop.Store, error) {
	if storeValue == "" {
		return nil, shop.Errorf(shop.ErrInvalidInput, "no store specified; use -s or set a default: shop config set defaults.store amazon")
	}

	// Step 1+2: Registry lookup (handles both exact alias and normalized domain).
	if entry := a.Registry.Lookup(storeValue); entry != nil {
		return a.storeFromEntry(ctx, entry)
	}

	// Step 3: Auto-discovery.
	for _, p := range shop.Providers() {
		info, err := p.Detect(ctx, storeValue)
		if err != nil || info == nil {
			continue
		}

		// Cache discovery in registry.
		entry := shop.RegistryEntry{
			Alias:      storeValue,
			Domain:     info.Domain,
			Provider:   p.Name(),
			Name:       info.Name,
			Country:    info.Country,
			Currency:   info.Currency,
			DetectedAt: time.Now().UTC().Format(time.RFC3339),
			DetectedBy: p.Name(),
		}
		a.Registry.Add(entry)

		if err := config.SaveRegistry(a.ConfigDir, a.Registry); err != nil {
			// Non-fatal but worth reporting.
			fmt.Fprintf(os.Stderr, "warning: could not cache registry: %v\n", err)
		}

		return p.Store(ctx, info.Domain, a.ConfigDir)
	}

	// Step 4: Not found.
	return nil, shop.Errorf(shop.ErrStoreNotFound, "store %q not found; no provider could handle this domain", storeValue).
		WithDetails(map[string]any{"store": storeValue})
}

// storeFromEntry finds the registered provider for an entry and creates a Store.
func (a *App) storeFromEntry(ctx context.Context, entry *shop.RegistryEntry) (shop.Store, error) {
	for _, p := range shop.Providers() {
		if p.Name() == entry.Provider {
			return p.Store(ctx, entry.Domain, a.ConfigDir)
		}
	}

	return nil, shop.Errorf(shop.ErrStoreNotFound, "provider %q not registered for store %q", entry.Provider, entry.Alias).
		WithDetails(map[string]any{"store": entry.Alias, "provider": entry.Provider})
}

// ensureDir creates the config directory if it doesn't exist.
func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}
