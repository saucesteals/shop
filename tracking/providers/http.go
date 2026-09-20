package providers

import (
	"io"
	"net/http"
	"time"

	"github.com/saucesteals/shop"
)

// userAgent is the browser profile used by public tracking clients.
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"

// httpClient executes bounded provider requests using an injected HTTP client.
// Configure the underlying client before concurrent use.
type httpClient struct {
	http *http.Client
}

// newHTTPClient uses the supplied HTTP client or creates one with a finite timeout.
func newHTTPClient(client *http.Client) *httpClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &httpClient{http: client}
}

// withCookies creates a session without mutating the underlying client.
// Transport and timeout are retained; cookies remain isolated to this session.
func (c *httpClient) withCookies(jar http.CookieJar) *httpClient {
	session := *c.http
	session.Jar = jar

	return newHTTPClient(&session)
}

// do executes a context-bound request, maps errors, and bounds the response body.
func (c *httpClient) do(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "carrier tracking request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, shop.Errorf(shop.ErrRateLimited, "tracking source rate limited").WithDetails(map[string]any{"retryAfter": resp.Header.Get("Retry-After")})
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

// upstreamError identifies an unusable tracking response without leaking its contents.
func upstreamError(reason string) *shop.Error {
	return shop.Errorf(shop.ErrUpstream, "carrier did not provide usable history").WithDetails(map[string]any{"reason": reason})
}
