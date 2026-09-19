// Package fedex retrieves scans from FedEx's tracking API.
package fedex

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

const (
	// The Deliveries guest bootstrap issues short-lived carrier tokens.
	// No developer credentials are embedded or persisted by Shop.
	sessionURL    = "https://api.lieferungen.app/oauthProxy/FedEx"
	trackingURL   = "https://apis.fedex.com/track/v1/trackingnumbers"
	clientVersion = "Deliveries/6.0.2.1975"
)

// Client retrieves shipment scans using a per-lookup guest token.
type Client struct {
	HTTP *http.Client
}

// Carriers declares the delivery networks handled by this provider.
func (c *Client) Carriers() []tracking.Carrier {
	return []tracking.Carrier{tracking.FedEx}
}

// Track retrieves scans without persisting session credentials.
func (c *Client) Track(ctx context.Context, input string) (*shop.TrackingSnapshot, error) {
	number, err := tracking.Number(input)
	if err != nil {
		return nil, err
	}
	if tracking.DetectCarrier(number) != tracking.FedEx {
		return nil, shop.Errorf(shop.ErrInvalidInput, "unsupported FedEx tracking format")
	}
	token, err := c.session(ctx)
	if err != nil {
		return nil, err
	}
	payload := lookupRequest{
		TrackingInfo:  []trackingInfo{{NumberInfo: numberInfo{Number: number}}},
		DetailedScans: true,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "encode tracking request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, trackingURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking request")
	}
	req.Header.Set("User-Agent", tracking.UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Locale", "en_US")
	req.Header.Set("Authorization", "Bearer "+token)
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
		Source:         "fedex",
		URL:            "https://www.fedex.com/fedextrack/?trknbr=" + url.QueryEscape(number),
		FetchedAt:      time.Now().UTC(),
		Freshness:      "unknown",
		Events:         events,
	}, nil
}

func (c *Client) session(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sessionURL, nil)
	if err != nil {
		return "", shop.Errorf(shop.ErrInternal, "create tracking session request")
	}
	req.Header.Set("User-Agent", clientVersion)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("X-Deliveries-P", "0")
	body, err := tracking.Fetch(c.HTTP, req)
	if err != nil {
		return "", err
	}
	var auth struct {
		Token     string `json:"access_token"`
		Type      string `json:"token_type"`
		ExpiresIn int    `json:"expires_in"`
	}
	if json.Unmarshal(body, &auth) != nil || auth.Token == "" || !strings.EqualFold(auth.Type, "bearer") || auth.ExpiresIn <= 0 {
		return "", tracking.UpstreamError("invalid_session")
	}

	return auth.Token, nil
}

type trackingInfo struct {
	NumberInfo numberInfo `json:"trackingNumberInfo"`
}

type lookupRequest struct {
	TrackingInfo  []trackingInfo `json:"trackingInfo"`
	DetailedScans bool           `json:"includeDetailedScans"`
}
