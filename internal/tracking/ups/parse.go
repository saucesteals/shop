package ups

import (
	"encoding/json"
	"html"
	"strings"

	"github.com/saucesteals/shop"
)

type response struct {
	StatusCode   string `json:"statusCode"`
	TrackDetails []struct {
		TrackingNumber string `json:"trackingNumber"`
		Activities     []struct {
			Date     string `json:"date"`
			Time     string `json:"time"`
			Location string `json:"location"`
			Scan     string `json:"activityScan"`
		} `json:"shipmentProgressActivities"`
	} `json:"trackDetails"`
}

func parse(body []byte, number string) ([]shop.TrackingEvent, error) {
	var result response
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, upstreamError("invalid_response")
	}
	if result.StatusCode != "200" {
		return nil, upstreamError("history_unavailable")
	}
	for _, detail := range result.TrackDetails {
		if !strings.EqualFold(detail.TrackingNumber, number) {
			continue
		}
		events := make([]shop.TrackingEvent, 0, len(detail.Activities))
		for _, activity := range detail.Activities {
			event := shop.TrackingEvent{
				Date:        strings.TrimSpace(activity.Date),
				Time:        strings.TrimSpace(activity.Time),
				Description: strings.TrimSpace(html.UnescapeString(activity.Scan)),
				Location:    strings.TrimSpace(html.UnescapeString(activity.Location)),
			}
			if event.Date == "" || event.Description == "" {
				return nil, upstreamError("invalid_response")
			}
			events = append(events, event)
		}
		if len(events) == 0 {
			return nil, upstreamError("history_unavailable")
		}

		return events, nil
	}

	return nil, upstreamError("history_unavailable")
}
