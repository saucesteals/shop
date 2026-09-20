package yanwen

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
)

type shipment struct {
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

func parse(body []byte, number string) ([]shop.TrackingEvent, bool, error) {
	var shipments []shipment
	if err := json.Unmarshal(body, &shipments); err != nil {
		return nil, false, tracking.UpstreamError("invalid_response")
	}
	for _, item := range shipments {
		if !strings.EqualFold(item.Number, number) {
			continue
		}
		if len(item.Events) == 0 {
			return nil, false, tracking.UpstreamError("history_unavailable")
		}
		sort.SliceStable(item.Events, func(i, j int) bool { return item.Events[i].Timestamp > item.Events[j].Timestamp })
		events := make([]shop.TrackingEvent, 0, len(item.Events))
		for _, raw := range item.Events {
			zone, err := time.Parse("Z07:00", raw.TimeZone)
			description := strings.TrimSpace(raw.Message)
			if err != nil || raw.Timestamp <= 0 || description == "" {
				return nil, false, tracking.UpstreamError("invalid_response")
			}
			at := time.UnixMilli(raw.Timestamp).In(zone.Location())
			var location []string
			for _, part := range []string{raw.Location.City, raw.Location.State} {
				if part = strings.TrimSpace(part); part != "" {
					location = append(location, part)
				}
			}
			events = append(events, shop.TrackingEvent{
				Date:        at.Format("2006-01-02"),
				Time:        at.Format("15:04:05Z07:00"),
				Description: description,
				Location:    strings.Join(location, ", "),
			})
		}

		return events, item.Events[0].Category == "Transit", nil
	}

	return nil, false, tracking.UpstreamError("history_unavailable")
}
