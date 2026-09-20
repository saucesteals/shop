package shop

import (
	"context"
	"fmt"
	"net/http"
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

// Options configures a Shop client. An empty ConfigDir uses ~/.config/shop.
type Options struct {
	ConfigDir string
	// HTTPClient overrides the tracking transport and timeout. It is not mutated.
	// Shopping providers manage their own transports.
	HTTPClient *http.Client
}

// Client shares a configuration directory between shopping and shipment tracking.
// Its configuration is fixed at construction; creating it does not write files.
type Client struct {
	configDir    string
	defaultStore string
	tracking     *tracking.Service
}

// New reads saved defaults and wires the built-in tracking clients. A nil
// HTTPClient uses defaults.timeout, or 30 seconds if no timeout is configured.
func New(options Options) (*Client, error) {
	dir := strings.TrimSpace(options.ConfigDir)
	if dir == "" {
		dir = config.DefaultDir()
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve config directory: %w", err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, Errorf(ErrConfigError, "load config: %v", err)
	}
	client := options.HTTPClient
	if client == nil {
		timeout := 30 * time.Second
		if cfg.Defaults.Timeout != "" {
			timeout, err = time.ParseDuration(cfg.Defaults.Timeout)
			if err != nil || timeout < 0 {
				return nil, Errorf(ErrConfigError, "invalid configured timeout")
			}
		}
		client = &http.Client{Timeout: timeout}
	}
	registry, err := tracking.NewRegistry(
		ups.New(client),
		stamps.New(client),
		fedex.New(client),
		gofo.New(client),
		yanwen.New(client),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		configDir:    dir,
		defaultStore: cfg.Defaults.Store,
		tracking:     tracking.New(dir, registry),
	}, nil
}

// ConfigDir returns the absolute directory shared by authentication and state.
func (c *Client) ConfigDir() string { return c.configDir }

// Tracking returns the reusable shipment service. No shopping login is required.
func (c *Client) Tracking() *tracking.Service { return c.tracking }

// Store resolves a saved alias or domain through registered shopping providers.
// An empty handle uses defaults.store. Discovery is cached in registry.json;
// tracking does not load or depend on this shopping registry.
func (c *Client) Store(ctx context.Context, handle string) (Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	handle = strings.TrimSpace(handle)
	if handle == "" {
		handle = c.defaultStore
	}
	if handle == "" {
		return nil, Errorf(ErrInvalidInput, "store handle is required when no default store is configured")
	}
	registry, err := config.LoadRegistry(c.configDir)
	if err != nil {
		return nil, Errorf(ErrConfigError, "load registry: %v", err)
	}
	providers := Providers()
	if entry := registry.Lookup(handle); entry != nil {
		for _, provider := range providers {
			if provider.Name() == entry.Provider {
				return provider.Store(ctx, entry.Domain, c.configDir)
			}
		}
		return nil, Errorf(ErrStoreNotFound, "provider %q not registered for store %q", entry.Provider, entry.Alias).
			WithDetails(map[string]any{"store": entry.Alias, "provider": entry.Provider})
	}
	for _, provider := range providers {
		info, err := provider.Detect(ctx, handle)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil || info == nil {
			continue
		}
		registry.Add(config.RegistryEntry{
			Alias:      handle,
			Domain:     info.Domain,
			Provider:   provider.Name(),
			Name:       info.Name,
			Country:    info.Country,
			Currency:   info.Currency,
			DetectedAt: time.Now().UTC().Format(time.RFC3339),
			DetectedBy: provider.Name(),
		})
		if err := config.SaveRegistry(c.configDir, registry); err != nil {
			return nil, Errorf(ErrConfigError, "cache discovered store: %v", err)
		}
		return provider.Store(ctx, info.Domain, c.configDir)
	}

	return nil, Errorf(ErrStoreNotFound, "store %q not found; no provider could handle this domain", handle).
		WithDetails(map[string]any{"store": handle})
}
