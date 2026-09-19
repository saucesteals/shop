// Package stamps reads USPS history from the public shipment tracking page.
package stamps

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
)

// Client retrieves tracking without accounts or persistent sessions.
type Client struct {
	HTTP *http.Client
}

// Carriers declares the delivery networks handled by this provider.
func (c *Client) Carriers() []tracking.Carrier {
	return []tracking.Carrier{tracking.USPS}
}

// Track reads the current published scan history, which may be cached upstream.
func (c *Client) Track(ctx context.Context, number string) (*shop.TrackingSnapshot, error) {
	number, err := tracking.Number(number)
	if err != nil {
		return nil, err
	}
	if tracking.DetectCarrier(number) != tracking.USPS {
		return nil, shop.Errorf(shop.ErrInvalidInput, "unsupported carrier tracking number")
	}
	endpoint := "https://www.stamps.com/tracking-details/?" + url.Values{"t": {number}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking request")
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", tracking.UserAgent)
	body, err := tracking.Fetch(c.HTTP, req)
	if err != nil {
		return nil, err
	}
	events, err := parse(bytes.NewReader(body), number)
	if err != nil {
		return nil, err
	}

	return &shop.TrackingSnapshot{
		TrackingNumber: number,
		Source:         "stamps",
		URL:            endpoint,
		FetchedAt:      time.Now().UTC(),
		Freshness:      "unknown",
		Events:         events,
	}, nil
}
