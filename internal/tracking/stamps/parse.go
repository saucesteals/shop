package stamps

import (
	"io"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
)

var latestTime = regexp.MustCompile(`(?i)at (\d{1,2}:\d{2} [ap]m) on ([a-z]+ \d{1,2}, \d{4})`)

func parse(r io.Reader, number string) ([]shop.TrackingEvent, string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, "", tracking.UpstreamError("invalid_response")
	}
	var block *html.Node
	walk(doc, func(n *html.Node) {
		if hasClass(n, "tracking-details-block") {
			block = n
		}
	})
	if block == nil {
		return nil, "", tracking.UpstreamError("unexpected_markup")
	}
	var identity, carrier, summary, heading string
	delivered := false
	var events []shop.TrackingEvent
	incomplete := false
	walk(block, func(n *html.Node) {
		if hasClass(n, "tracking-number") {
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				if hasClass(child, "code") {
					identity = nodeText(child)
				}
			}
		}
		if hasClass(n, "carrier") {
			walk(n, func(c *html.Node) {
				if hasClass(c, "code") {
					carrier = nodeText(c)
				}
			})
		}
		if hasClass(n, "status") {
			summary = nodeText(n)
			walk(n, func(c *html.Node) {
				if c.Type == html.ElementNode && c.Data == "h2" {
					heading = nodeText(c)
				}
			})
		}
		if hasClass(n, "step--3") && hasClass(n, "passed") {
			delivered = true
		}
		if !hasClass(n, "event") {
			return
		}
		var event shop.TrackingEvent
		walk(n, func(field *html.Node) {
			switch {
			case hasClass(field, "event_time"):
				event.Date = nodeText(field)
			case hasClass(field, "event_status"):
				event.Description = nodeText(field)
			case hasClass(field, "event_location"):
				event.Location = nodeText(field)
			}
		})
		if event.Date == "" || event.Description == "" {
			incomplete = true
			return
		}
		events = append(events, event)
	})
	if identity != number || !strings.EqualFold(carrier, "USPS") {
		return nil, "", tracking.UpstreamError("shipment_mismatch")
	}
	if incomplete {
		return nil, "", tracking.UpstreamError("invalid_response")
	}
	if len(events) == 0 {
		return nil, "", tracking.UpstreamError("history_unavailable")
	}
	// The table omits times. Only attach the headline's local time when its date
	// matches the newest row; never manufacture times for older scans.
	if match := latestTime.FindStringSubmatch(summary); len(match) == 3 {
		headlineDate, e1 := time.Parse("January 2, 2006", match[2])
		eventDate, e2 := time.Parse("Mon, Jan 2, 2006", events[0].Date)
		if e1 == nil && e2 == nil && headlineDate.Equal(eventDate) {
			events[0].Time = match[1]
		}
	}

	// The progress bar can lag behind the scan table.
	delivered = delivered || strings.EqualFold(strings.SplitN(events[0].Description, ",", 2)[0], "Delivered")
	var estimate string
	if !delivered {
		if value, ok := strings.CutPrefix(heading, "Arrives "); ok {
			value = strings.TrimSpace(value)
			if value != "" && !strings.EqualFold(value, "Unknown") && !strings.EqualFold(value, "Pending") {
				estimate = value
			}
		}
	}

	return events, estimate, nil
}

func walk(n *html.Node, visit func(*html.Node)) {
	visit(n)
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walk(child, visit)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}

	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, value := range strings.Fields(attr(n, "class")) {
		if value == class {
			return true
		}
	}

	return false
}

func nodeText(n *html.Node) string {
	var text strings.Builder
	walk(n, func(child *html.Node) {
		if child.Type == html.TextNode {
			text.WriteString(child.Data)
		}
	})

	return strings.Join(strings.Fields(text.String()), " ")
}
