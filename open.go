package shop

import (
	"context"
	"os"
	"strings"
)

// Open creates a Store for handle using configDir for auth and provider
// state. Library callers own that directory; it is not ~/.config/shop unless
// they pass that path.
func Open(ctx context.Context, handle, configDir string) (Store, error) {
	handle = strings.TrimSpace(handle)
	configDir = strings.TrimSpace(configDir)
	if handle == "" {
		return nil, Errorf(ErrInvalidInput, "store handle is required")
	}
	if configDir == "" {
		return nil, Errorf(ErrInvalidInput, "config directory is required")
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, Errorf(ErrConfigError, "create config directory: %v", err)
	}

	for _, provider := range Providers() {
		info, err := provider.Detect(ctx, handle)
		if err != nil || info == nil {
			continue
		}

		return provider.Store(ctx, info.Domain, configDir)
	}

	return nil, Errorf(ErrStoreNotFound, "store %q not found", handle)
}
