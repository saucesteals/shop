// Package ups implements the UPS tracking client.
package ups

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/saucesteals/shop/internal/fault"
	"github.com/saucesteals/shop/tracking"
)

const upsOrigin = "https://www.ups.com"

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"

// Client retrieves history in a new anonymous session for each lookup.
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
	return []tracking.Carrier{tracking.UPS}
}

// Track retrieves scans without retaining cookies or account information.
func (c *Client) Track(ctx context.Context, number string) (*tracking.Snapshot, error) {
	number, err := tracking.NormalizeNumber(number)
	if err != nil {
		return nil, err
	}
	if tracking.DetectCarrier(number) != tracking.UPS {
		return nil, fault.Errorf(fault.ErrInvalidInput, "unsupported carrier tracking number")
	}
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, fault.Errorf(fault.ErrInternal, "create tracking session")
	}
	client := *c.http
	client.Jar = jar
	trackingURL := upsOrigin + "/track?" + url.Values{
		"loc":      {"en_US"},
		"tracknum": {strings.ToUpper(number)},
	}.Encode()
	if _, err := request(ctx, &client, trackingURL, nil, ""); err != nil {
		return nil, err
	}
	page, _ := url.Parse(trackingURL)
	var token string
	for _, cookie := range jar.Cookies(page) {
		if cookie.Name == "X-XSRF-TOKEN-ST" {
			token, err = url.PathUnescape(cookie.Value)
			if err != nil {
				return nil, upstreamError("invalid_session")
			}
			break
		}
	}
	if token == "" {
		return nil, upstreamError("session_unavailable")
	}
	payload, err := json.Marshal(struct {
		Locale         string
		TrackingNumber []string
		Requester      string
	}{
		Locale:         "en_US",
		TrackingNumber: []string{strings.ToUpper(number)},
		Requester:      "ST/trackdetails",
	})
	if err != nil {
		return nil, fault.Errorf(fault.ErrInternal, "encode tracking request")
	}
	body, err := request(ctx, &client, "https://webapis.ups.com/track/api/Track/GetStatus?loc=en_US", payload, token)
	if err != nil {
		return nil, err
	}
	events, estimate, err := parseUPS(body, number)
	if err != nil {
		return nil, err
	}

	return &tracking.Snapshot{
		TrackingNumber:   number,
		Source:           "ups",
		URL:              trackingURL,
		FetchedAt:        time.Now().UTC(),
		Freshness:        "unknown",
		Events:           events,
		ExpectedDelivery: estimate,
	}, nil
}

func request(ctx context.Context, client *http.Client, endpoint string, payload []byte, token string) ([]byte, error) {
	method := http.MethodGet
	if payload != nil {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fault.Errorf(fault.ErrInternal, "create tracking request")
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if payload != nil {
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", upsOrigin)
		req.Header.Set("Referer", upsOrigin+"/")
		req.Header.Set("X-XSRF-TOKEN", token)
	}

	resp, err := client.Do(req)
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

type upsResponse struct {
	StatusCode   string `json:"statusCode"`
	TrackDetails []struct {
		TrackingNumber string `json:"trackingNumber"`
		Delivered      bool   `json:"isDelivered"`
		Suppressed     bool   `json:"disableSDDSection"`
		ScheduledDate  string `json:"sdd"`
		Start          string `json:"sdst"`
		End            string `json:"sdt"`
		Activities     []struct {
			Date     string `json:"date"`
			Time     string `json:"time"`
			Location string `json:"location"`
			Scan     string `json:"activityScan"`
		} `json:"shipmentProgressActivities"`
	} `json:"trackDetails"`
}

func parseUPS(body []byte, number string) ([]tracking.Event, string, error) {
	var result upsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", upstreamError("invalid_response")
	}
	if result.StatusCode != "200" {
		return nil, "", upstreamError("history_unavailable")
	}
	for _, detail := range result.TrackDetails {
		if !strings.EqualFold(detail.TrackingNumber, number) {
			continue
		}
		events := make([]tracking.Event, 0, len(detail.Activities))
		for _, activity := range detail.Activities {
			event := tracking.Event{
				Date:        strings.TrimSpace(activity.Date),
				Time:        strings.TrimSpace(activity.Time),
				Description: strings.TrimSpace(html.UnescapeString(activity.Scan)),
				Location:    strings.TrimSpace(html.UnescapeString(activity.Location)),
			}
			if event.Date == "" || event.Description == "" {
				return nil, "", upstreamError("invalid_response")
			}
			events = append(events, event)
		}
		if len(events) == 0 {
			return nil, "", upstreamError("history_unavailable")
		}

		var estimate string
		if !detail.Delivered && !detail.Suppressed {
			if day, err := time.Parse("20060102", detail.ScheduledDate); err == nil {
				estimate = day.Format("Mon, Jan 2, 2006")
				start, startErr := time.Parse("15:04:05", detail.Start)
				end, endErr := time.Parse("15:04:05", detail.End)
				if startErr == nil && endErr == nil {
					estimate += " · " + start.Format("3:04 PM") + "–" + end.Format("3:04 PM") + " (carrier local time)"
				}
			}
		}

		return events, estimate, nil
	}

	return nil, "", upstreamError("history_unavailable")
}

// upstreamError identifies an unusable tracking response without leaking its contents.
func upstreamError(reason string) *fault.Error {
	return fault.Errorf(fault.ErrUpstream, "carrier did not provide usable history").WithDetails(map[string]any{"reason": reason})
}
