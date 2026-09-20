package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/tracking"
)

const upsOrigin = "https://www.ups.com"

// upsClient retrieves history in a new anonymous session for each lookup.
type upsClient struct {
	http *httpClient
}

// Carriers declares the delivery networks handled by this provider.
func (c *upsClient) Carriers() []tracking.Carrier {
	return []tracking.Carrier{tracking.UPS}
}

// Track retrieves scans without retaining cookies or account information.
func (c *upsClient) Track(ctx context.Context, number string) (*tracking.Snapshot, error) {
	number, err := tracking.Number(number)
	if err != nil {
		return nil, err
	}
	if tracking.DetectCarrier(number) != tracking.UPS {
		return nil, shop.Errorf(shop.ErrInvalidInput, "unsupported carrier tracking number")
	}
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking session")
	}
	client := c.http.withCookies(jar)
	trackingURL := upsOrigin + "/track?" + url.Values{
		"loc":      {"en_US"},
		"tracknum": {strings.ToUpper(number)},
	}.Encode()
	if _, err := upsRequest(ctx, client, trackingURL, nil, ""); err != nil {
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
		return nil, shop.Errorf(shop.ErrInternal, "encode tracking request")
	}
	body, err := upsRequest(ctx, client, "https://webapis.ups.com/track/api/Track/GetStatus?loc=en_US", payload, token)
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

func upsRequest(ctx context.Context, client *httpClient, endpoint string, payload []byte, token string) ([]byte, error) {
	method := http.MethodGet
	if payload != nil {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "create tracking request")
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

	return client.do(req)
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
