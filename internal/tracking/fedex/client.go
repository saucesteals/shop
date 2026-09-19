// Package fedex implements FedEx's public web tracking protocol.
package fedex

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
)

const (
	propertiesURL = "https://www.fedex.com/wtrk/track/properties/WTRKProperties.json"
	apiURL        = "https://api.fedex.com"
)

// Client retrieves shipment scans with a short-lived anonymous web session.
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
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking session")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	if c.HTTP != nil {
		clone := *c.HTTP
		client = &clone
	}
	client.Jar = jar
	body, err := request(ctx, client, http.MethodGet, propertiesURL, nil, "")
	if err != nil {
		return nil, err
	}
	var config struct {
		API struct {
			ClientID string `json:"client_id"`
		} `json:"api"`
	}
	if json.Unmarshal(body, &config) != nil || config.API.ClientID == "" {
		return nil, tracking.UpstreamError("invalid_configuration")
	}
	// Discover the public web client ID; no developer key or embedded secret.
	query := url.Values{
		"client_id":  {config.API.ClientID},
		"grant_type": {"client_credentials"},
		"scope":      {"oob"},
	}
	body, err = request(ctx, client, http.MethodPost, apiURL+"/auth/oauth/v2/token?"+query.Encode(), nil, "")
	if err != nil {
		return nil, err
	}
	var auth struct {
		Token string `json:"access_token"`
		Type  string `json:"token_type"`
	}
	if json.Unmarshal(body, &auth) != nil || auth.Token == "" || auth.Type != "Bearer" && auth.Type != "bearer" {
		return nil, tracking.UpstreamError("invalid_session")
	}
	payload := lookupRequest{
		AppDeviceType:          "WTRK",
		AppType:                "WTRK",
		SupportHTML:            true,
		SupportCurrentLocation: true,
	}
	payload.TrackingInfo = []trackingInfo{{TrackNumberInfo: numberInfo{TrackingNumber: number}}}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "encode tracking request")
	}
	body, err = request(ctx, client, http.MethodPost, apiURL+"/track/v2/shipments", encoded, "Bearer "+auth.Token)
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

type numberInfo struct {
	TrackingNumber    string `json:"trackingNumber"`
	TrackingQualifier string `json:"trackingQualifier"`
	TrackingCarrier   string `json:"trackingCarrier"`
}
type trackingInfo struct {
	TrackNumberInfo numberInfo `json:"trackNumberInfo"`
}
type lookupRequest struct {
	AppDeviceType            string         `json:"appDeviceType"`
	AppType                  string         `json:"appType"`
	SummaryView              bool           `json:"summaryView"`
	SupportHTML              bool           `json:"supportHTML"`
	SupportCurrentLocation   bool           `json:"supportCurrentLocation"`
	TrackingInfo             []trackingInfo `json:"trackingInfo"`
	UniqueKey                string         `json:"uniqueKey"`
	GuestAuthenticationToken string         `json:"guestAuthenticationToken"`
}

func request(ctx context.Context, client *http.Client, method, endpoint string, body []byte, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking request")
	}
	req.Header.Set("User-Agent", tracking.UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://www.fedex.com")
	req.Header.Set("Referer", "https://www.fedex.com/")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if token != "" {
		req.Header.Set("Authorization", token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("X-clientid", "WTRK")
		req.Header.Set("X-locale", "en_US")
		req.Header.Set("X-version", "1.0.0")
	}

	return tracking.Fetch(client, req)
}
