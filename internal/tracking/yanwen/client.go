// Package yanwen reads Yanwen Express history from its public tracking service.
package yanwen

import (
	"bytes"
	"context"
	"encoding/json"
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
	events, inTransit, err := parse(body, number)
	if err != nil {
		return nil, err
	}

	var estimate string
	if inTransit {
		estimate = c.estimate(ctx, number)
	}

	return &shop.TrackingSnapshot{
		TrackingNumber:   number,
		Source:           "yanwen",
		URL:              "https://www.yanwenexpress.com/tracking.html?" + query,
		FetchedAt:        time.Now().UTC(),
		Freshness:        "unknown",
		Events:           events,
		ExpectedDelivery: estimate,
	}, nil
}

// The website requests this optional window only for in-transit shipments.
// Failure must not discard an otherwise successful scan lookup.
func (c *Client) estimate(ctx context.Context, number string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	payload, err := json.Marshal(struct {
		Number string `json:"waybillNumber"`
	}{Number: strings.ToUpper(number)})
	if err != nil {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.yanwenexpress.com/data/getDeliveryTime", bytes.NewReader(payload))
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", tracking.UserAgent)
	body, err := tracking.Fetch(c.HTTP, req)
	if err != nil {
		return ""
	}

	return parseEstimate(body)
}
