package amazon

import (
	"bytes"
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

const tvssBaseURL = "https://tvss.amazon.com"

// tvssClient wraps an http.Client with the auth state needed for TVSS API
// calls. All requests go through doGet or doPost which handle headers and
// error mapping automatically.
//
// Cookies, the access token, marketplace ID, and device UDID are
// pre-computed at construction and reused for every request.
type tvssClient struct {
	http          *http.Client
	state         *authState
	cookies       []*http.Cookie // standard cookies for AddCookie
	accessToken   string         // at-main Atza token for x-amz-access-token header
	marketplaceID string         // region-specific marketplace ID
	udid          string         // stable device UDID, reused per request
}

// newTVSSClient creates a tvssClient from the store's auth state.
// Pre-computes the cookie list, extracts the access token, and uses the
// device serial from registration as the stable UDID.
func newTVSSClient(httpClient *http.Client, state *authState, marketplaceID string) *tvssClient {
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

// newRequest creates an http.Request with session cookies already applied.
// All request paths (TVSS, web search, Alexa) use this as their starting
// point instead of manually looping over cookies.
func (c *tvssClient) newRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	for _, cookie := range c.cookies {
		req.AddCookie(cookie)
	}

	return req, nil
}

// setTVSSHeaders applies TVSS-specific request headers (user agent, request
// ID, app ID, access token). Does NOT apply cookies — those are set by
// newRequest.
func (c *tvssClient) setTVSSHeaders(req *http.Request) {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	requestID := strings.ToUpper(hex.EncodeToString(b))

	req.Header.Set("x-amzn-RequestId", requestID)
	req.Header.Set("User-Agent", tvssUA)
	req.Header.Set("x-amz-msh-appid", fmt.Sprintf(
		"name=ShopTV3P;ver=2000610;device=AFTMM;os=Android_7.1.2;UDID=%s;tag=mshop-amazon-us-20",
		c.udid,
	))

	if c.accessToken != "" {
		req.Header.Set("x-amz-access-token", c.accessToken)
	}
}

// doGet performs an authenticated GET against the TVSS API and unmarshals
// the response into dest. acceptType is optional.
func (c *tvssClient) doGet(ctx context.Context, rawURL string, dest any, headers ...map[string]string) error {
	req, err := c.newRequest(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return shop.Errorf(shop.ErrInternal, "build request: %v", err)
	}
	c.setTVSSHeaders(req)
	for _, h := range headers {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}

	return c.execute(req, dest)
}

// doMutate performs an authenticated request with a JSON body for the given
// HTTP method. Shared implementation for POST, PUT, and PATCH.
func (c *tvssClient) doMutate(ctx context.Context, method, rawURL, mediaType string, body, dest any, headers ...map[string]string) error {
	data, err := json.Marshal(body)
	if err != nil {
		return shop.Errorf(shop.ErrInternal, "marshal request body: %v", err)
	}

	req, err := c.newRequest(ctx, method, rawURL, bytes.NewReader(data))
	if err != nil {
		return shop.Errorf(shop.ErrInternal, "build request: %v", err)
	}
	c.setTVSSHeaders(req)
	for _, h := range headers {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}

	ct := "application/vnd.com.amazon.tvss.api+json"
	if mediaType != "" {
		ct = fmt.Sprintf("application/vnd.com.amazon.tvss.api+json; type=%q", mediaType)
	}
	req.Header.Set("Content-Type", ct)

	return c.execute(req, dest)
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
func (c *tvssClient) execute(req *http.Request, dest any) error {
	resp, err := c.http.Do(req)
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
