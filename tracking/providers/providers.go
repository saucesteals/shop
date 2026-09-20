// Package providers assembles the built-in carrier tracking clients.
package providers

import (
	"net/http"

	"github.com/saucesteals/shop/tracking"
	"github.com/saucesteals/shop/tracking/providers/fedex"
	"github.com/saucesteals/shop/tracking/providers/gofo"
	"github.com/saucesteals/shop/tracking/providers/stamps"
	"github.com/saucesteals/shop/tracking/providers/ups"
	"github.com/saucesteals/shop/tracking/providers/yanwen"
)

// New constructs the carrier registry using the supplied HTTP client.
// A nil client selects the default timeout; UPS cookies remain per-lookup.
func New(client *http.Client) (*tracking.Registry, error) {
	return tracking.NewRegistry(
		ups.New(client),
		stamps.New(client),
		fedex.New(client),
		gofo.New(client),
		yanwen.New(client),
	)
}
