package tracking

import (
	"io"
	"net/http"
	"time"

	"github.com/saucesteals/shop"
)

// UserAgent is the browser profile used by public tracking clients.
const UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"

// Fetch executes a context-bound request and bounds the response size.
// A nil client uses a finite timeout; supplied clients retain their transport and session.
func Fetch(client *http.Client, req *http.Request) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "carrier tracking request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, shop.Errorf(shop.ErrRateLimited, "tracking source rate limited").WithDetails(map[string]any{"retryAfter": resp.Header.Get("Retry-After")})
	}
	if resp.StatusCode != http.StatusOK {
		return nil, UpstreamError("http_error").WithDetails(map[string]any{"status": resp.StatusCode})
	}
	const maxBody = 2 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "read tracking response")
	}
	if len(body) > maxBody {
		return nil, UpstreamError("response_too_large")
	}

	return body, nil
}

// UpstreamError identifies an unusable tracking response without leaking its contents.
func UpstreamError(reason string) *shop.Error {
	return shop.Errorf(shop.ErrUpstream, "carrier did not provide usable history").WithDetails(map[string]any{"reason": reason})
}
