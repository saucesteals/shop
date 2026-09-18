// Package trackglobal reads available shipment history from Track.global.
package trackglobal

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/saucesteals/shop"
)

// Client retrieves cached shipment history. A nil HTTP client uses a bounded default.
type Client struct {
	HTTP *http.Client
}

// Track returns available events. A cache miss does not mean a shipment is invalid.
func (c *Client) Track(ctx context.Context, number string) (*shop.TrackingResult, error) {
	number = strings.TrimSpace(number)
	if number == "" || len(number) > 100 || strings.ContainsFunc(number, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		return nil, shop.Errorf(shop.ErrInvalidInput, "provide a tracking number containing only letters and digits")
	}
	query := url.Values{"track": {number}, "noBanner": {"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://track.global/en/fast-track?"+query.Encode(), nil)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking request")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "tracking request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, shop.Errorf(shop.ErrRateLimited, "tracking source rate limited").WithDetails(map[string]any{"retryAfter": resp.Header.Get("Retry-After")})
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil, upstreamError("cache_miss")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, upstreamError("http_error").WithDetails(map[string]any{"status": resp.StatusCode})
	}
	const maxBody = 2 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "read tracking response")
	}
	if len(body) > maxBody {
		return nil, upstreamError("response_too_large")
	}
	events, err := parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	return &shop.TrackingResult{
		TrackingNumber: number,
		Source:         "trackglobal",
		URL:            "https://track.global/en?" + url.Values{"trackingNumber": {number}}.Encode(),
		FetchedAt:      time.Now().UTC().Format(time.RFC3339),
		Freshness:      "unknown",
		Events:         events,
	}, nil
}

func upstreamError(reason string) *shop.Error {
	return shop.Errorf(shop.ErrUpstream, "tracking source did not provide usable history").WithDetails(map[string]any{"reason": reason})
}
