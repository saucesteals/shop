package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/tracking"
)

// yanwenClient retrieves tracking without accounts or persistent sessions.
type yanwenClient struct{ http *httpClient }

// Carriers declares the delivery networks handled by this provider.
func (c *yanwenClient) Carriers() []tracking.Carrier {
	return []tracking.Carrier{tracking.Yanwen}
}

// Track reads published scan history, which may be cached upstream.
func (c *yanwenClient) Track(ctx context.Context, number string) (*tracking.Snapshot, error) {
	number, err := tracking.Number(number)
	if err != nil {
		return nil, err
	}
	if tracking.DetectCarrier(number) != tracking.Yanwen {
		return nil, shop.Errorf(shop.ErrInvalidInput, "unsupported carrier tracking number")
	}
	query := url.Values{"label": {strings.ToUpper(number)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.yanwenexpress.com/server?"+query, nil)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	body, err := c.http.do(req)
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
func (c *yanwenClient) estimate(ctx context.Context, number string) string {
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
	body, err := c.http.do(req)
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

	return deliveryWindow(start, start.Add(2*time.Hour))
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
