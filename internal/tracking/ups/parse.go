package ups

import (
	"encoding/json"
	"html"
	"strings"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
)

type response struct {
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

func parse(body []byte, number string) ([]shop.TrackingEvent, string, error) {
	var result response
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", tracking.UpstreamError("invalid_response")
	}
	if result.StatusCode != "200" {
		return nil, "", tracking.UpstreamError("history_unavailable")
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
				return nil, "", tracking.UpstreamError("invalid_response")
			}
			events = append(events, event)
		}
		if len(events) == 0 {
			return nil, "", tracking.UpstreamError("history_unavailable")
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

	return nil, "", tracking.UpstreamError("history_unavailable")
}
