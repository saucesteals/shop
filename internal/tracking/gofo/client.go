// Package gofo reads GOFO US history from its public tracking service.
package gofo

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
type Client struct {
	HTTP *http.Client
}

// Carriers declares the delivery networks handled by this provider.
func (c *Client) Carriers() []tracking.Carrier {
	return []tracking.Carrier{tracking.GOFO}
}

// Track reads the current published scan history, which may be cached upstream.
func (c *Client) Track(ctx context.Context, number string) (*shop.TrackingSnapshot, error) {
	number, err := tracking.Number(number)
	if err != nil {
		return nil, err
	}
	if tracking.DetectCarrier(number) != tracking.GOFO {
		return nil, shop.Errorf(shop.ErrInvalidInput, "unsupported carrier tracking number")
	}
	payload, err := json.Marshal(struct {
		NumberList []string `json:"numberList"`
	}{NumberList: []string{strings.ToUpper(number)}})
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "encode tracking request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.gofo.com/us/cnee-api/consignee/track/query/page", bytes.NewReader(payload))
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", tracking.UserAgent)
	req.Header.Set("Lang", "en")
	req.Header.Set("User-Time-Zone", "Local Time")
	body, err := tracking.Fetch(c.HTTP, req)
	if err != nil {
		return nil, err
	}
	events, estimate, err := parse(body, number)
	if err != nil {
		return nil, err
	}

	return &shop.TrackingSnapshot{
		TrackingNumber:   number,
		Source:           "gofo",
		URL:              "https://www.gofo.com/us/track?" + url.Values{"searchID": {number}}.Encode(),
		FetchedAt:        time.Now().UTC(),
		Freshness:        "unknown",
		Events:           events,
		ExpectedDelivery: estimate,
	}, nil
}
