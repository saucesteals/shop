// Package ups reads shipment history from the carrier's anonymous tracking service.
package ups

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
)

const origin = "https://www.ups.com"

// Client retrieves history in a new anonymous session for each lookup.
type Client struct {
	HTTP *http.Client
}

// Matches recognizes the carrier's standard 1Z tracking identifiers.
func Matches(number string) bool {
	if len(number) != 18 || !strings.EqualFold(number[:2], "1Z") {
		return false
	}
	for _, ch := range strings.ToUpper(number[2:]) {
		if (ch < '0' || ch > '9') && (ch < 'A' || ch > 'Z') {
			return false
		}
	}

	return true
}

// Track retrieves scans without retaining cookies or account information.
func (c *Client) Track(ctx context.Context, number string) (*shop.TrackingSnapshot, error) {
	number, err := tracking.Number(number)
	if err != nil {
		return nil, err
	}
	if !Matches(number) {
		return nil, shop.Errorf(shop.ErrInvalidInput, "unsupported carrier tracking number")
	}
	client := http.Client{Timeout: 30 * time.Second}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.Jar, err = cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking session")
	}
	trackingURL := origin + "/track?" + url.Values{
		"loc":      {"en_US"},
		"tracknum": {strings.ToUpper(number)},
	}.Encode()
	if _, err := request(ctx, &client, trackingURL, nil, ""); err != nil {
		return nil, err
	}
	page, _ := url.Parse(trackingURL)
	var token string
	for _, cookie := range client.Jar.Cookies(page) {
		if cookie.Name == "X-XSRF-TOKEN-ST" {
			token, err = url.PathUnescape(cookie.Value)
			if err != nil {
				return nil, upstreamError("invalid_session")
			}
			break
		}
	}
	if token == "" {
		return nil, upstreamError("session_unavailable")
	}
	payload, err := json.Marshal(struct {
		Locale         string
		TrackingNumber []string
		Requester      string
	}{
		Locale:         "en_US",
		TrackingNumber: []string{strings.ToUpper(number)},
		Requester:      "ST/trackdetails",
	})
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "encode tracking request")
	}
	body, err := request(ctx, &client, "https://webapis.ups.com/track/api/Track/GetStatus?loc=en_US", payload, token)
	if err != nil {
		return nil, err
	}
	events, err := parse(body, number)
	if err != nil {
		return nil, err
	}

	return &shop.TrackingSnapshot{
		TrackingNumber: number,
		Source:         "ups",
		URL:            trackingURL,
		FetchedAt:      time.Now().UTC(),
		Freshness:      "unknown",
		Events:         events,
	}, nil
}

func request(ctx context.Context, client *http.Client, endpoint string, payload []byte, token string) ([]byte, error) {
	method := http.MethodGet
	if payload != nil {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking request")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if payload != nil {
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.Header.Set("Referer", origin+"/")
		req.Header.Set("X-XSRF-TOKEN", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "carrier tracking request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, shop.Errorf(shop.ErrRateLimited, "tracking source rate limited")
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

	return body, nil
}

func upstreamError(reason string) *shop.Error {
	return shop.Errorf(shop.ErrUpstream, "carrier did not provide usable history").WithDetails(map[string]any{"reason": reason})
}
