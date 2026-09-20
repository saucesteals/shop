// Package providers implements the built-in carrier tracking clients.
package providers

import (
	"net/http"

	"github.com/saucesteals/shop/tracking"
)

// New constructs the carrier registry using the supplied HTTP client.
// A nil client selects the default timeout; UPS cookies remain per-lookup.
func New(client *http.Client) (*tracking.Registry, error) {
	http := newHTTPClient(client)

	return tracking.NewRegistry(
		&upsClient{http: http},
		&stampsClient{http: http},
		&fedexClient{http: http},
		&gofoClient{http: http},
		&yanwenClient{http: http},
	)
}
