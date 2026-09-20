package gofo

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
)

type response struct {
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

func parse(body []byte, number string) ([]shop.TrackingEvent, string, error) {
	var result response
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", tracking.UpstreamError("invalid_response")
	}
	if result.Code != 200 {
		return nil, "", tracking.UpstreamError("history_unavailable")
	}
	for _, shipment := range result.Data.Success {
		if !strings.EqualFold(shipment.WaybillNo, number) {
			continue
		}
		type scan struct {
			at    time.Time
			event shop.TrackingEvent
		}
		scans := make([]scan, 0, len(shipment.Events))
		for _, raw := range shipment.Events {
			at, err := time.Parse("2006-01-02T15:04:05Z0700", raw.Date)
			if err != nil {
				at, err = time.Parse(time.RFC3339Nano, raw.Date)
			}
			description := strings.TrimSpace(raw.Content)
			if err != nil || description == "" {
				return nil, "", tracking.UpstreamError("invalid_response")
			}
			var location []string
			for _, part := range []string{raw.City, raw.Province} {
				if part = strings.TrimSpace(part); part != "" {
					location = append(location, part)
				}
			}
			scans = append(scans, scan{
				at: at,
				event: shop.TrackingEvent{
					Date:        at.Format("2006-01-02"),
					Time:        at.Format("15:04:05Z07:00"),
					Description: description,
					Location:    strings.Join(location, ", "),
				},
			})
		}
		if len(scans) == 0 {
			return nil, "", tracking.UpstreamError("history_unavailable")
		}
		sort.SliceStable(scans, func(i, j int) bool { return scans[i].at.After(scans[j].at) })
		events := make([]shop.TrackingEvent, 0, len(scans))
		for _, scan := range scans {
			events = append(events, scan.event)
		}

		var estimate string
		if !strings.EqualFold(shipment.Status, "Delivered") {
			estimate = strings.TrimSpace(shipment.EstimatedArrival)
		}

		return events, estimate, nil
	}

	return nil, "", tracking.UpstreamError("history_unavailable")
}
