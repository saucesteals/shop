// Package fedex implements the FedEx tracking client.
package fedex

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/saucesteals/shop/tracking"
)

const (
	// The Deliveries guest bootstrap issues short-lived carrier tokens.
	// No developer credentials are embedded or persisted by Shop.
	fedexSessionURL    = "https://api.lieferungen.app/oauthProxy/FedEx"
	fedexTrackingURL   = "https://apis.fedex.com/track/v1/trackingnumbers"
	fedexClientVersion = "Deliveries/6.0.2.1975"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"

// Client retrieves shipment scans using a per-lookup guest token.
type Client struct {
	http *http.Client
}

// New constructs a client using standard net/http configuration.
// A nil client uses a 30-second timeout. Configure supplied clients before use.
// The caller owns the supplied transport.
func New(client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{http: client}
}

var _ tracking.Provider = (*Client)(nil)

// Carriers declares the delivery networks handled by this provider.
func (c *Client) Carriers() []tracking.Carrier {
	return []tracking.Carrier{tracking.FedEx}
}

// Track retrieves scans without persisting session credentials.
func (c *Client) Track(ctx context.Context, input string) (*tracking.Snapshot, error) {
	number, err := tracking.NormalizeNumber(input)
	if err != nil {
		return nil, err
	}
	if tracking.DetectCarrier(number) != tracking.FedEx {
		return nil, &tracking.Error{Kind: tracking.ErrInvalidInput, Message: "unsupported FedEx tracking format"}
	}
	token, err := c.session(ctx)
	if err != nil {
		return nil, err
	}
	payload := fedexLookupRequest{
		TrackingInfo:  []fedexTrackingInfo{{NumberInfo: fedexNumberInfo{Number: number}}},
		DetailedScans: true,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, &tracking.Error{Kind: tracking.ErrInternal, Message: "encode tracking request"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fedexTrackingURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, &tracking.Error{Kind: tracking.ErrInternal, Message: "create tracking request"}
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Locale", "en_US")
	req.Header.Set("Authorization", "Bearer "+token)
	body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	events, estimate, err := parseFedEx(body, number)
	if err != nil {
		return nil, err
	}

	return &tracking.Snapshot{
		TrackingNumber:   number,
		Source:           "fedex",
		URL:              "https://www.fedex.com/fedextrack/?trknbr=" + url.QueryEscape(number),
		FetchedAt:        time.Now().UTC(),
		Freshness:        "unknown",
		Events:           events,
		ExpectedDelivery: estimate,
	}, nil
}

func (c *Client) session(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fedexSessionURL, nil)
	if err != nil {
		return "", &tracking.Error{Kind: tracking.ErrInternal, Message: "create tracking session request"}
	}
	req.Header.Set("User-Agent", fedexClientVersion)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("X-Deliveries-P", "0")
	body, err := c.do(req)
	if err != nil {
		return "", err
	}
	var auth struct {
		Token     string `json:"access_token"`
		Type      string `json:"token_type"`
		ExpiresIn int    `json:"expires_in"`
	}
	if json.Unmarshal(body, &auth) != nil || auth.Token == "" || !strings.EqualFold(auth.Type, "bearer") || auth.ExpiresIn <= 0 {
		return "", upstreamError("invalid_session")
	}

	return auth.Token, nil
}

type fedexTrackingInfo struct {
	NumberInfo fedexNumberInfo `json:"trackingNumberInfo"`
}

type fedexLookupRequest struct {
	TrackingInfo  []fedexTrackingInfo `json:"trackingInfo"`
	DetailedScans bool                `json:"includeDetailedScans"`
}

type fedexNumberInfo struct {
	Number string `json:"trackingNumber"`
}

type fedexResponse struct {
	Output struct {
		Shipments []struct {
			Number  string `json:"trackingNumber"`
			Results []struct {
				NumberInfo fedexNumberInfo `json:"trackingNumberInfo"`
				Error      struct {
					Code string `json:"code"`
				} `json:"error"`
				Status struct {
					Code        string `json:"code"`
					DerivedCode string `json:"derivedCode"`
				} `json:"latestStatusDetail"`
				Dates []struct {
					Type  string `json:"type"`
					Value string `json:"dateTime"`
				} `json:"dateAndTimes"`
				Estimate struct {
					Window struct {
						Starts string `json:"begins"`
						Ends   string `json:"ends"`
					} `json:"window"`
				} `json:"estimatedDeliveryTimeWindow"`
				Events []struct {
					Date        time.Time `json:"date"`
					Description string    `json:"eventDescription"`
					Exception   string    `json:"exceptionDescription"`
					Location    struct {
						City    string `json:"city"`
						State   string `json:"stateOrProvinceCode"`
						Country string `json:"countryCode"`
					} `json:"scanLocation"`
				} `json:"scanEvents"`
			} `json:"trackResults"`
		} `json:"completeTrackResults"`
	} `json:"output"`
}

func parseFedEx(body []byte, number string) ([]tracking.Event, string, error) {
	var result fedexResponse
	if json.Unmarshal(body, &result) != nil {
		return nil, "", upstreamError("invalid_response")
	}
	if len(result.Output.Shipments) != 1 {
		return nil, "", upstreamError("shipment_unavailable")
	}
	shipment := result.Output.Shipments[0]
	if shipment.Number != number {
		return nil, "", upstreamError("shipment_mismatch")
	}
	// Reused numbers may have multiple shipments; never select one arbitrarily.
	if len(shipment.Results) != 1 {
		return nil, "", upstreamError("shipment_unavailable")
	}
	detail := shipment.Results[0]
	if detail.NumberInfo.Number != number {
		return nil, "", upstreamError("shipment_mismatch")
	}
	if detail.Error.Code != "" {
		return nil, "", upstreamError("shipment_unavailable")
	}
	if len(detail.Events) == 0 {
		return nil, "", upstreamError("history_unavailable")
	}
	sort.SliceStable(detail.Events, func(i, j int) bool {
		return detail.Events[i].Date.After(detail.Events[j].Date)
	})

	events := make([]tracking.Event, 0, len(detail.Events))
	for _, scan := range detail.Events {
		when := scan.Date
		description := strings.TrimSpace(scan.Description)
		if when.IsZero() || description == "" {
			return nil, "", upstreamError("invalid_response")
		}
		if exception := strings.TrimSpace(scan.Exception); exception != "" && exception != description {
			description += ": " + exception
		}
		var location []string
		for _, part := range []string{scan.Location.City, scan.Location.State, scan.Location.Country} {
			if part = strings.TrimSpace(part); part != "" {
				location = append(location, part)
			}
		}
		events = append(events, tracking.Event{
			Date:        when.Format("2006-01-02"),
			Time:        when.Format("15:04:05Z07:00"),
			Description: description,
			Location:    strings.Join(location, ", "),
		})
	}

	var estimate string
	delivered := detail.Status.Code == "DL" || detail.Status.DerivedCode == "DL"
	for _, date := range detail.Dates {
		if date.Type == "ACTUAL_DELIVERY" {
			delivered = true
		}
	}
	if !delivered {
		start, startErr := time.Parse(time.RFC3339, detail.Estimate.Window.Starts)
		end, endErr := time.Parse(time.RFC3339, detail.Estimate.Window.Ends)
		if startErr == nil && endErr == nil && !end.Before(start) {
			const layout = "Mon, Jan 2, 2006 3:04 PM -07:00"
			estimate = start.Format(layout) + " – " + end.Format(layout)
		}
		if estimate == "" {
			for _, kind := range []string{"ESTIMATED_DELIVERY", "SCHEDULED_DELIVERY"} {
				for _, date := range detail.Dates {
					if date.Type != kind {
						continue
					}
					if day, err := time.Parse(time.RFC3339, date.Value); err == nil {
						// These fields describe a delivery date; midnight is not a promised time.
						estimate = day.Format("Mon, Jan 2, 2006")
						break
					}
				}
				if estimate != "" {
					break
				}
			}
		}
	}

	return events, estimate, nil
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &tracking.Error{Kind: tracking.ErrNetwork, Message: "carrier tracking request failed"}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &tracking.Error{Kind: tracking.ErrRateLimited, RetryAfter: resp.Header.Get("Retry-After")}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &tracking.Error{Kind: tracking.ErrUpstream, Reason: "http_error", StatusCode: resp.StatusCode}
	}
	const maxBody = 2 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, &tracking.Error{Kind: tracking.ErrNetwork, Message: "read tracking response"}
	}
	if len(body) > maxBody {
		return nil, upstreamError("response_too_large")
	}

	return body, nil
}

// upstreamError identifies an unusable tracking response without leaking its contents.
func upstreamError(reason string) *tracking.Error {
	return &tracking.Error{Kind: tracking.ErrUpstream, Reason: reason}
}
