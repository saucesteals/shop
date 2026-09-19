// Package carriers wires the built-in tracking providers.
package carriers

import (
	"github.com/saucesteals/shop/internal/tracking"
	"github.com/saucesteals/shop/internal/tracking/fedex"
	"github.com/saucesteals/shop/internal/tracking/stamps"
	"github.com/saucesteals/shop/internal/tracking/ups"
)

// New constructs the registry from each provider's declared carriers.
func New() (*tracking.Registry, error) {
	return tracking.NewRegistry(&ups.Client{}, &stamps.Client{}, &fedex.Client{})
}
