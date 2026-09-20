// Package yanwen implements the Yanwen tracking client.
package yanwen

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

	"github.com/saucesteals/shop/internal/fault"
	"github.com/saucesteals/shop/tracking"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"

// Client retrieves tracking without accounts or persistent sessions.
type Client struct{ http *http.Client }

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
	return []tracking.Carrier{tracking.Yanwen}
}

// Track reads published scan history, which may be cached upstream.
func (c *Client) Track(ctx context.Context, number string) (*tracking.Snapshot, error) {
	number, err := tracking.NormalizeNumber(number)
	if err != nil {
		return nil, err
	}
	if tracking.DetectCarrier(number) != tracking.Yanwen {
		return nil, fault.Errorf(fault.ErrInvalidInput, "unsupported carrier tracking number")
	}
	query := url.Values{"label": {strings.ToUpper(number)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.yanwenexpress.com/server?"+query, nil)
	if err != nil {
		return nil, fault.Errorf(fault.ErrInternal, "create tracking request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	events, inTransit, err := parseYanwen(body, number)
	if err != nil {
		return nil, err
	}

	var estimate string
	if inTransit {
		estimate = c.estimate(ctx, number)
	}

	return &tracking.Snapshot{
		TrackingNumber:   number,
		Source:           "yanwen",
		URL:              "https://www.yanwenexpress.com/tracking.html?" + query,
		FetchedAt:        time.Now().UTC(),
		Freshness:        "unknown",
		Events:           events,
		ExpectedDelivery: estimate,
	}, nil
}

// The website requests this optional window only for in-transit shipments.
// Failure must not discard an otherwise successful scan lookup.
func (c *Client) estimate(ctx context.Context, number string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	payload, err := json.Marshal(struct {
		Number string `json:"waybillNumber"`
	}{Number: strings.ToUpper(number)})
	if err != nil {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.yanwenexpress.com/data/getDeliveryTime", bytes.NewReader(payload))
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	body, err := c.do(req)
	if err != nil {
		return ""
	}

	return parseYanwenEstimate(body)
}

func parseYanwenEstimate(body []byte) string {
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Now       int64  `json:"now"`
			Remaining int64  `json:"remaining"`
			TimeZone  string `json:"timeZone"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &result) != nil || !result.Success || result.Data.Now <= 0 || result.Data.Remaining <= 0 {
		return ""
	}
	zone, err := time.LoadLocation(result.Data.TimeZone)
	if err != nil || result.Data.TimeZone == "" {
		return ""
	}
	now := time.UnixMilli(result.Data.Now).In(zone)
	// Match the carrier UI: only display during its 07:00–17:59 local window.
	if now.Hour() <= 6 || now.Hour() >= 18 {
		return ""
	}
	// The carrier publishes remaining milliseconds and displays a two-hour window.
	startMillis := result.Data.Now + result.Data.Remaining
	if startMillis < result.Data.Now {
		return ""
	}
	start := time.UnixMilli(startMillis).In(zone)

	const layout = "Mon, Jan 2, 2006 3:04 PM -07:00"

	return start.Format(layout) + " – " + start.Add(2*time.Hour).Format(layout)
}

type yanwenShipment struct {
	Number string `json:"expressCode"`
	Events []struct {
		Category  string `json:"category"`
		Message   string `json:"message"`
		Timestamp int64  `json:"timestamp"`
		TimeZone  string `json:"timeZone"`
		Location  struct {
			City  string `json:"city"`
			State string `json:"state"`
		} `json:"location"`
	} `json:"events"`
}

func parseYanwen(body []byte, number string) ([]tracking.Event, bool, error) {
	var shipments []yanwenShipment
	if err := json.Unmarshal(body, &shipments); err != nil {
		return nil, false, upstreamError("invalid_response")
	}
	for _, item := range shipments {
		if !strings.EqualFold(item.Number, number) {
			continue
		}
		if len(item.Events) == 0 {
			return nil, false, upstreamError("history_unavailable")
		}
		sort.SliceStable(item.Events, func(i, j int) bool { return item.Events[i].Timestamp > item.Events[j].Timestamp })
		events := make([]tracking.Event, 0, len(item.Events))
		for _, raw := range item.Events {
			zone, err := time.Parse("Z07:00", raw.TimeZone)
			description := strings.TrimSpace(raw.Message)
			if err != nil || raw.Timestamp <= 0 || description == "" {
				return nil, false, upstreamError("invalid_response")
			}
			at := time.UnixMilli(raw.Timestamp).In(zone.Location())
			var location []string
			for _, part := range []string{raw.Location.City, raw.Location.State} {
				if part = strings.TrimSpace(part); part != "" {
					location = append(location, part)
				}
			}
			events = append(events, tracking.Event{
				Date:        at.Format("2006-01-02"),
				Time:        at.Format("15:04:05Z07:00"),
				Description: description,
				Location:    strings.Join(location, ", "),
			})
		}

		return events, item.Events[0].Category == "Transit", nil
	}

	return nil, false, upstreamError("history_unavailable")
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fault.Errorf(fault.ErrNetwork, "carrier tracking request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fault.Errorf(fault.ErrRateLimited, "tracking source rate limited").WithDetails(map[string]any{"retryAfter": resp.Header.Get("Retry-After")})
	}
	if resp.StatusCode != http.StatusOK {
		return nil, upstreamError("http_error").WithDetails(map[string]any{"status": resp.StatusCode})
	}
	const maxBody = 2 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fault.Errorf(fault.ErrNetwork, "read tracking response")
	}
	if len(body) > maxBody {
		return nil, upstreamError("response_too_large")
	}

	return body, nil
}

// upstreamError identifies an unusable tracking response without leaking its contents.
func upstreamError(reason string) *fault.Error {
	return fault.Errorf(fault.ErrUpstream, "carrier did not provide usable history").WithDetails(map[string]any{"reason": reason})
}
