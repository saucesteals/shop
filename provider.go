package shop

import (
	"cmp"
	"slices"
	"sync"
)

var (
	providersMu sync.RWMutex
	providers   []Provider
	sorted      []Provider // cached sorted copy, rebuilt on Register
)

// Register makes a provider available for store resolution.
// Called in init() by each provider package.
func Register(p Provider) {
	providersMu.Lock()
	defer providersMu.Unlock()

	providers = append(providers, p)

	// Pre-sort so Providers() never re-sorts.
	s := make([]Provider, len(providers))
	copy(s, providers)
	slices.SortFunc(s, func(a, b Provider) int {
		return cmp.Compare(a.DetectCost(), b.DetectCost())
	})
	sorted = s
}

// Providers returns all registered providers, sorted by DetectCost
// (cheapest first). The returned slice must not be modified.
func Providers() []Provider {
	providersMu.RLock()
	defer providersMu.RUnlock()

	return sorted
}
