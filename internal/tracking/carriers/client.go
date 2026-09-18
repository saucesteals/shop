// Package carriers wires the built-in tracking providers.
package carriers

import (
	"github.com/saucesteals/shop/internal/tracking"
	"github.com/saucesteals/shop/internal/tracking/trackglobal"
	"github.com/saucesteals/shop/internal/tracking/ups"
)

// New constructs the built-in registry. Providers declare their own carriers;
// the general source also handles numbers without a dedicated registration.
func New() (*tracking.Registry, error) {
	general := &trackglobal.Client{}

	return tracking.NewRegistry(general, &ups.Client{}, general)
}
