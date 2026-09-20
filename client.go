package shop

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saucesteals/shop/internal/config"
	"github.com/saucesteals/shop/tracking"
	"github.com/saucesteals/shop/tracking/fedex"
	"github.com/saucesteals/shop/tracking/gofo"
	"github.com/saucesteals/shop/tracking/stamps"
	"github.com/saucesteals/shop/tracking/ups"
	"github.com/saucesteals/shop/tracking/yanwen"
)

// Client owns configuration, store resolution, and shipment tracking.
// Configure it before use. Tracking supports concurrent calls; store discovery
// and edits to Config or Registry require caller synchronization.
type Client struct {
	Config    *Config
	Registry  *Registry
	configDir string
	tracking  *tracking.Service
}

// New creates a Client by loading config and registry from the given directory.
// On first run, it creates the directory and writes default files.
func New(options Options) (*Client, error) {
	configDir := strings.TrimSpace(options.ConfigDir)
	if configDir == "" {
		configDir = config.DefaultDir()
	}
	configDir, err := filepath.Abs(configDir)
	if err != nil {
		return nil, fmt.Errorf("resolve config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, Errorf(ErrConfigError, "create config directory: %v", err)
	}

	cfg, err := config.Load(configDir)
	if err != nil {
		return nil, Errorf(ErrConfigError, "load config: %v", err)
	}

	reg, err := config.LoadRegistry(configDir)
	if err != nil {
		return nil, Errorf(ErrConfigError, "load registry: %v", err)
	}

	// First-run: write default files if they don't exist.
	if err := config.EnsureDefaults(configDir, cfg, reg); err != nil {
		return nil, Errorf(ErrConfigError, "write default config: %v", err)
	}

	tracker := options.Tracker
	if tracker == nil {
		httpClient := options.HTTPClient
		if httpClient == nil {
			timeout := 30 * time.Second
			if cfg.Defaults.Timeout != "" {
				configured, err := time.ParseDuration(cfg.Defaults.Timeout)
				if err != nil || configured < 0 {
					return nil, Errorf(ErrConfigError, "invalid configured timeout")
				}
				timeout = configured
			}
			httpClient = &http.Client{Timeout: timeout}
		}
		registry, err := tracking.NewRegistry(
			ups.New(httpClient),
			stamps.New(httpClient),
			fedex.New(httpClient),
			gofo.New(httpClient),
			yanwen.New(httpClient),
		)
		if err != nil {
			return nil, err
		}
		tracker = registry
	}

	return &Client{
		Config:    cfg,
		Registry:  reg,
		configDir: configDir,
		tracking:  tracking.New(configDir, tracker),
	}, nil
}

// Store takes a store handle and returns a ready Store instance.
// Resolution chain:
//  1. Exact match in registry (alias or domain)
//  2. Domain normalization + registry lookup
//  3. Auto-discovery via provider Detect() in cost order
//  4. Fail with ErrStoreNotFound
func (a *Client) Store(ctx context.Context, storeValue string) (Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	storeValue = strings.TrimSpace(storeValue)
	if storeValue == "" {
		storeValue = a.Config.Defaults.Store
	}
	if storeValue == "" {
		return nil, Errorf(ErrInvalidInput, "store handle is required when no default store is configured")
	}

	// Step 1+2: Registry lookup (handles both exact alias and normalized domain).
	if entry := a.Registry.Lookup(storeValue); entry != nil {
		return a.storeFromEntry(ctx, entry)
	}

	// Step 3: Auto-discovery.
	for _, p := range Providers() {
		info, err := p.Detect(ctx, storeValue)
		if err != nil || info == nil {
			continue
		}

		// Cache discovery in registry.
		entry := RegistryEntry{
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

		if err := config.SaveRegistry(a.configDir, a.Registry); err != nil {
			return nil, fmt.Errorf("cache discovered store: %w", err)
		}

		return p.Store(ctx, info.Domain, a.configDir)
	}

	// Step 4: Not found.
	return nil, Errorf(ErrStoreNotFound, "store %q not found; no provider could handle this domain", storeValue).
		WithDetails(map[string]any{"store": storeValue})
}

// storeFromEntry finds the registered provider for an entry and creates a Store.
func (a *Client) storeFromEntry(ctx context.Context, entry *RegistryEntry) (Store, error) {
	for _, p := range Providers() {
		if p.Name() == entry.Provider {
			return p.Store(ctx, entry.Domain, a.configDir)
		}
	}

	return nil, Errorf(ErrStoreNotFound, "provider %q not registered for store %q", entry.Provider, entry.Alias).
		WithDetails(map[string]any{"store": entry.Alias, "provider": entry.Provider})
}

// Options configures a Client. Zero values use the standard Shop directory and
// built-in tracking clients. A supplied HTTPClient is reused without mutation;
// it overrides defaults.timeout for tracking. Shopping providers own their transports.
type Options struct {
	ConfigDir  string
	HTTPClient *http.Client
	// Tracker optionally replaces the built-in carrier registry.
	Tracker tracking.Tracker
}

// Tracking returns this client's reusable shipment service. It shares ConfigDir
// with shopping authentication and state, and requires no store login.
func (a *Client) Tracking() *tracking.Service { return a.tracking }

// Config holds persisted defaults and shopping-provider settings.
type Config = config.Config

// Registry holds the persisted shopping-store directory.
type Registry = config.Registry

// ConfigDir returns the absolute directory shared by all client services.
func (a *Client) ConfigDir() string { return a.configDir }
