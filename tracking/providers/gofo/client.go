// Package gofo implements the GOFO tracking client.
package gofo

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

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/tracking"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"

// Client retrieves tracking without accounts or persistent sessions.
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
	return []tracking.Carrier{tracking.GOFO}
}

// Track reads the current published scan history, which may be cached upstream.
func (c *Client) Track(ctx context.Context, number string) (*tracking.Snapshot, error) {
	number, err := tracking.Number(number)
	if err != nil {
		return nil, err
	}
	if tracking.DetectCarrier(number) != tracking.GOFO {
		return nil, shop.Errorf(shop.ErrInvalidInput, "unsupported carrier tracking number")
	}
	payload, err := json.Marshal(struct {
		NumberList []string `json:"numberList"`
	}{NumberList: []string{strings.ToUpper(number)}})
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "encode tracking request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.gofo.com/us/cnee-api/consignee/track/query/page", bytes.NewReader(payload))
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Lang", "en")
	req.Header.Set("User-Time-Zone", "Local Time")
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
	events, estimate, err := parseGOFO(body, number)
	if err != nil {
		return nil, err
	}

	return &tracking.Snapshot{
		TrackingNumber:   number,
		Source:           "gofo",
		URL:              "https://www.gofo.com/us/track?" + url.Values{"searchID": {number}}.Encode(),
		FetchedAt:        time.Now().UTC(),
		Freshness:        "unknown",
		Events:           events,
		ExpectedDelivery: estimate,
	}, nil
}

type gofoResponse struct {
	Code int `json:"code"`
	Data struct {
		Success []struct {
			WaybillNo        string `json:"waybillNo"`
			Status           string `json:"status"`
			EstimatedArrival string `json:"estimatedArrivalTime"`
			Events           []struct {
				Date     string `json:"processDate"`
				Content  string `json:"processContent"`
				City     string `json:"processCity"`
				Province string `json:"processProvince"`
			} `json:"trackEventList"`
		} `json:"success"`
	} `json:"data"`
}

func parseGOFO(body []byte, number string) ([]tracking.Event, string, error) {
	var result gofoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", upstreamError("invalid_response")
	}
	if result.Code != 200 {
		return nil, "", upstreamError("history_unavailable")
	}
	for _, shipment := range result.Data.Success {
		if !strings.EqualFold(shipment.WaybillNo, number) {
			continue
		}
		type scan struct {
			at    time.Time
			event tracking.Event
		}
		scans := make([]scan, 0, len(shipment.Events))
		for _, raw := range shipment.Events {
			at, err := time.Parse("2006-01-02T15:04:05Z0700", raw.Date)
			if err != nil {
				at, err = time.Parse(time.RFC3339Nano, raw.Date)
			}
			description := strings.TrimSpace(raw.Content)
			if err != nil || description == "" {
				return nil, "", upstreamError("invalid_response")
			}
			var location []string
			for _, part := range []string{raw.City, raw.Province} {
				if part = strings.TrimSpace(part); part != "" {
					location = append(location, part)
				}
			}
			scans = append(scans, scan{
				at: at,
				event: tracking.Event{
					Date:        at.Format("2006-01-02"),
					Time:        at.Format("15:04:05Z07:00"),
					Description: description,
					Location:    strings.Join(location, ", "),
				},
			})
		}
		if len(scans) == 0 {
			return nil, "", upstreamError("history_unavailable")
		}
		sort.SliceStable(scans, func(i, j int) bool { return scans[i].at.After(scans[j].at) })
		events := make([]tracking.Event, 0, len(scans))
		for _, scan := range scans {
			events = append(events, scan.event)
		}

		var estimate string
		if !strings.EqualFold(shipment.Status, "Delivered") {
			estimate = strings.TrimSpace(shipment.EstimatedArrival)
		}

		return events, estimate, nil
	}

	return nil, "", upstreamError("history_unavailable")
}

// upstreamError identifies an unusable tracking response without leaking its contents.
func upstreamError(reason string) *shop.Error {
	return shop.Errorf(shop.ErrUpstream, "carrier did not provide usable history").WithDetails(map[string]any{"reason": reason})
}
