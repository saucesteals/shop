// Package trackglobal reads available shipment history from Track.global.
package trackglobal

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
)

const origin = "https://track.global"

// Client retrieves shipment history using an isolated anonymous session per lookup.
type Client struct {
	HTTP *http.Client
}

// Track follows the consumer lookup flow; carrier freshness remains source-dependent.
func (c *Client) Track(ctx context.Context, number string) (*shop.TrackingResult, error) {
	number, err := tracking.Number(number)
	if err != nil {
		return nil, err
	}
	client := http.Client{Timeout: 30 * time.Second}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.Jar, err = cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking session")
	}
	query := url.Values{"track": {number}}
	if _, _, err := request(ctx, &client, "/en", nil); err != nil {
		return nil, err
	}
	if _, _, err := request(ctx, &client, "/en/begin-tracking", query); err != nil {
		return nil, err
	}
	query.Set("noBanner", "1")
	body, fresh, err := request(ctx, &client, "/en/fast-track", query)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || !fresh {
		body, _, err = request(ctx, &client, "/en/ajax-track", url.Values{"track": {number}, "not_frame": {"true"}, "alias": {""}})
		if err != nil {
			return nil, err
		}
	}
	events, err := parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	return &shop.TrackingResult{
		TrackingNumber: number, Source: "trackglobal",
		URL:       origin + "/en?" + url.Values{"trackingNumber": {number}}.Encode(),
		FetchedAt: time.Now().UTC().Format(time.RFC3339), Freshness: "unknown", Events: events,
	}, nil
}

func request(ctx context.Context, client *http.Client, path string, query url.Values) ([]byte, bool, error) {
	rawURL := origin + path
	if len(query) > 0 {
		rawURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, shop.Errorf(shop.ErrInternal, "create tracking request")
	}
	req.Header.Set("User-Agent", "shop/0 (+https://github.com/saucesteals/shop)")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", origin+"/en")
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, shop.Errorf(shop.ErrNetwork, "tracking request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, false, shop.Errorf(shop.ErrRateLimited, "tracking source rate limited").WithDetails(map[string]any{"retryAfter": resp.Header.Get("Retry-After")})
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, upstreamError("http_error").WithDetails(map[string]any{"status": resp.StatusCode})
	}
	const maxBody = 2 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, false, shop.Errorf(shop.ErrNetwork, "read tracking response")
	}
	if len(body) > maxBody {
		return nil, false, upstreamError("response_too_large")
	}

	return body, resp.Header.Get("X-Fresh") == "1" || bytes.Contains(body, []byte(`data-fresh="1"`)), nil
}

func upstreamError(reason string) *shop.Error {
	return shop.Errorf(shop.ErrUpstream, "tracking source did not provide usable history").WithDetails(map[string]any{"reason": reason})
}
