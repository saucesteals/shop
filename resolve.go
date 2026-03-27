package shop

import "context"

var resolveFunc func(ctx context.Context, storeValue string) (Store, error)

// SetResolver registers the global store resolution function.
// Called by the app layer during initialization.
func SetResolver(fn func(ctx context.Context, storeValue string) (Store, error)) {
	resolveFunc = fn
}

// Resolve takes a --store value and returns a ready Store instance.
// Implements the full resolution chain: exact match → normalize → detect → fail.
// The resolver must be initialized via SetResolver before calling this function.
func Resolve(ctx context.Context, storeValue string) (Store, error) {
	if resolveFunc == nil {
		return nil, Errorf(ErrInternal, "store resolver not initialized")
	}

	return resolveFunc(ctx, storeValue)
}
