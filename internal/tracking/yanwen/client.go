// Package yanwen reads Yanwen Express history from its public tracking service.
package yanwen

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
)

// Client retrieves tracking without accounts or persistent sessions.
type Client struct{ HTTP *http.Client }

// Carriers declares the delivery networks handled by this provider.
func (c *Client) Carriers() []tracking.Carrier {
	return []tracking.Carrier{tracking.Yanwen}
}

// Track reads published scan history, which may be cached upstream.
func (c *Client) Track(ctx context.Context, number string) (*shop.TrackingSnapshot, error) {
	number, err := tracking.Number(number)
	if err != nil {
		return nil, err
	}
	if tracking.DetectCarrier(number) != tracking.Yanwen {
		return nil, shop.Errorf(shop.ErrInvalidInput, "unsupported carrier tracking number")
	}
	query := url.Values{"label": {strings.ToUpper(number)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.yanwenexpress.com/server?"+query, nil)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", tracking.UserAgent)
	body, err := tracking.Fetch(c.HTTP, req)
	if err != nil {
		return nil, err
	}
	events, err := parse(body, number)
	if err != nil {
		return nil, err
	}

	return &shop.TrackingSnapshot{
		TrackingNumber: number,
		Source:         "yanwen",
		URL:            "https://www.yanwenexpress.com/tracking.html?" + query,
		FetchedAt:      time.Now().UTC(),
		Freshness:      "unknown",
		Events:         events,
	}, nil
}
