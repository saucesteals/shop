package amazon

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/saucesteals/shop"
)

// asinPattern validates that a product ID is a well-formed 10-character
// Amazon Standard Identification Number.
var asinPattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// validateASIN checks that id matches the ASIN format before it is used in
// URL path construction.
func validateASIN(id string) error {
	if !asinPattern.MatchString(id) {
		return shop.Errorf(shop.ErrInvalidInput, "invalid ASIN format: %q", id)
	}

	return nil
}

type requestProfile uint8

const (
	profileWeb requestProfile = iota
	profileAOD
	profileJSON
	profileTVSS
)

type requestOptions struct {
	profile       requestProfile
	cookies       []*http.Cookie
	headers       http.Header
	checkRedirect func(*http.Request, []*http.Request) error
}

// amazonClient owns Amazon's shared HTTP request lifecycle.
type amazonClient struct {
	http *http.Client
}

// newAmazonClient creates the provider's canonical HTTP client.
func newAmazonClient() *amazonClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true

	return &amazonClient{
		http: &http.Client{
			Transport: transport,
			Timeout:   httpTimeout,
		},
	}
}

// do builds and sends an Amazon request with consistent defaults, cookies,
// compression handling, and optional redirect policy.
func (c *amazonClient) do(ctx context.Context, method, rawURL string, body io.Reader, opts requestOptions) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	applyRequestProfile(req, opts.profile)
	for key, values := range opts.headers {
		req.Header.Del(key)
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	for _, cookie := range opts.cookies {
		req.AddCookie(cookie)
	}

	client := c.http
	if opts.checkRedirect != nil {
		clone := *c.http
		clone.CheckRedirect = opts.checkRedirect
		client = &clone
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if err := decodeGzip(resp); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}

	return resp, nil
}

func applyRequestProfile(req *http.Request, profile requestProfile) {
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip")

	switch profile {
	case profileWeb:
		req.Header.Set("User-Agent", mobileUA)
		req.Header.Set("Accept", "text/html")
	case profileAOD:
		req.Header.Set("User-Agent", mobileUA)
		req.Header.Set("Accept", "text/html")
		req.Header.Del("Accept-Encoding")
	case profileJSON:
		req.Header.Set("User-Agent", mobileUA)
		req.Header.Set("Accept", "application/json")
	case profileTVSS:
	}
}

func decodeGzip(resp *http.Response) error {
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "", "identity":
		return nil
	case "gzip":
		decoded, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("decode gzip response: %w", err)
		}
		resp.Body = &gzipBody{Reader: decoded, compressed: resp.Body}
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		resp.Uncompressed = true
		return nil
	default:
		return fmt.Errorf("unsupported content encoding %q", resp.Header.Get("Content-Encoding"))
	}
}

type gzipBody struct {
	*gzip.Reader
	compressed io.Closer
}

func (b *gzipBody) Close() error {
	decodeErr := b.Reader.Close()
	compressedErr := b.compressed.Close()
	if decodeErr != nil {
		return decodeErr
	}

	return compressedErr
}

const tvssBaseURL = "https://tvss.amazon.com"

// tvssClient wraps an http.Client with the auth state needed for TVSS API
// calls. All requests go through doGet or doPost which handle headers and
// error mapping automatically.
//
// Cookies, the access token, marketplace ID, and device UDID are
// pre-computed at construction and reused for every request.
type tvssClient struct {
	http          *amazonClient
	state         *authState
	cookies       []*http.Cookie // standard cookies for AddCookie
	accessToken   string         // at-main Atza token for x-amz-access-token header
	marketplaceID string         // region-specific marketplace ID
	udid          string         // stable device UDID, reused per request
}

// newTVSSClient creates a tvssClient from the store's auth state.
// Pre-computes the cookie list, extracts the access token, and uses the
// device serial from registration as the stable UDID.
func newTVSSClient(httpClient *amazonClient, state *authState, marketplaceID string) *tvssClient {
	return &tvssClient{
		http:          httpClient,
		state:         state,
		cookies:       state.httpCookies(),
		accessToken:   state.cookieValue("at-main"),
		marketplaceID: marketplaceID,
		udid:          state.Device.DeviceSerial,
	}
}

// tvssPath builds a full TVSS URL for the given path segments and optional
// query params. Path segments are joined with "/" and prefixed with the
// client's marketplace base path.
//
// Example: api.tvssPath([]string{"products", "B08N5WRWNW"}, nil)
// → "https://tvss.amazon.com/marketplaces/ATVPDKIKX0DER/products/B08N5WRWNW?sif_profile=tvss"
func (c *tvssClient) tvssPath(segments []string, params url.Values) string {
	parts := []string{tvssBaseURL, "marketplaces", c.marketplaceID}
	parts = append(parts, segments...)
	u := strings.Join(parts, "/")

	if params == nil {
		params = url.Values{}
	}
	params.Set("sif_profile", "tvss")

	return u + "?" + params.Encode()
}

// headers returns the canonical headers for TVSS requests.
func (c *tvssClient) headers() http.Header {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	requestID := strings.ToUpper(hex.EncodeToString(b))

	headers := make(http.Header)
	headers.Set("x-amzn-RequestId", requestID)
	headers.Set("User-Agent", tvssUA)
	headers.Set("x-amz-msh-appid", fmt.Sprintf(
		"name=ShopTV3P;ver=2000610;device=AFTMM;os=Android_7.1.2;UDID=%s;tag=mshop-amazon-us-20",
		c.udid,
	))

	if c.accessToken != "" {
		headers.Set("x-amz-access-token", c.accessToken)
	}

	return headers
}

// doGet performs an authenticated GET against the TVSS API and unmarshals
// the response into dest. acceptType is optional.
func (c *tvssClient) doGet(ctx context.Context, rawURL string, dest any, headers ...map[string]string) error {
	requestHeaders := c.headers()
	for _, h := range headers {
		for k, v := range h {
			requestHeaders.Set(k, v)
		}
	}

	return c.execute(ctx, http.MethodGet, rawURL, nil, requestHeaders, dest)
}

// doMutate performs an authenticated request with a JSON body for the given
// HTTP method. Shared implementation for POST, PUT, and PATCH.
func (c *tvssClient) doMutate(ctx context.Context, method, rawURL, mediaType string, body, dest any, headers ...map[string]string) error {
	data, err := json.Marshal(body)
	if err != nil {
		return shop.Errorf(shop.ErrInternal, "marshal request body: %v", err)
	}

	requestHeaders := c.headers()
	for _, h := range headers {
		for k, v := range h {
			requestHeaders.Set(k, v)
		}
	}

	ct := "application/vnd.com.amazon.tvss.api+json"
	if mediaType != "" {
		ct = fmt.Sprintf("application/vnd.com.amazon.tvss.api+json; type=%q", mediaType)
	}
	requestHeaders.Set("Content-Type", ct)

	return c.execute(ctx, method, rawURL, bytes.NewReader(data), requestHeaders, dest)
}

// doPost performs an authenticated POST.
func (c *tvssClient) doPost(ctx context.Context, rawURL, mediaType string, body, dest any, headers ...map[string]string) error {
	return c.doMutate(ctx, http.MethodPost, rawURL, mediaType, body, dest, headers...)
}

// doPut performs an authenticated PUT.
func (c *tvssClient) doPut(ctx context.Context, rawURL, mediaType string, body, dest any, headers ...map[string]string) error {
	return c.doMutate(ctx, http.MethodPut, rawURL, mediaType, body, dest, headers...)
}

// doPatch performs an authenticated PATCH.
func (c *tvssClient) doPatch(ctx context.Context, rawURL, mediaType string, body, dest any, headers ...map[string]string) error {
	return c.doMutate(ctx, http.MethodPatch, rawURL, mediaType, body, dest, headers...)
}

// execute sends the request, reads the response, maps HTTP errors to
// *shop.Error, and unmarshals the body into dest.
func (c *tvssClient) execute(ctx context.Context, method, rawURL string, requestBody io.Reader, headers http.Header, dest any) error {
	resp, err := c.http.do(ctx, method, rawURL, requestBody, requestOptions{
		profile: profileTVSS,
		cookies: c.cookies,
		headers: headers,
	})
	if err != nil {
		return shop.Errorf(shop.ErrNetwork, "tvss request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return shop.Errorf(shop.ErrNetwork, "read tvss response: %v", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return shop.Errorf(shop.ErrAuthExpired, "tvss auth expired (%d)", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return shop.Errorf(shop.ErrNotFound, "tvss resource not found")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return shop.Errorf(shop.ErrRateLimited, "tvss rate limited")
	}
	if resp.StatusCode >= 500 {
		return shop.Errorf(shop.ErrStoreError, "tvss server error (%d): %s", resp.StatusCode, truncateBody(body))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return shop.Errorf(shop.ErrStoreError, "tvss unexpected status (%d): %s", resp.StatusCode, truncateBody(body))
	}

	if dest != nil {
		// TVSS wraps some endpoints in a {"resource":..,"type":..,"entity":{..}}
		// envelope. Unwrap the entity if present so callers always get the
		// inner payload regardless of whether the envelope exists.
		raw := body
		var envelope struct {
			Entity json.RawMessage `json:"entity"`
		}
		if json.Unmarshal(body, &envelope) == nil && len(envelope.Entity) > 0 {
			raw = envelope.Entity
		}

		if err := json.Unmarshal(raw, dest); err != nil {
			return shop.Errorf(shop.ErrInternal, "parse tvss response: %v", err)
		}
	}

	return nil
}
