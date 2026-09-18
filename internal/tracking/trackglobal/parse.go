package trackglobal

import (
	"io"
	"strings"

	"golang.org/x/net/html"

	"github.com/saucesteals/shop"
)

func parse(r io.Reader) ([]shop.TrackingEvent, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, upstreamError("invalid_response")
	}
	var events []shop.TrackingEvent
	widget := false
	incomplete := false
	empty := false
	walk(doc, func(n *html.Node) {
		if hasClass(n, "tracking-widget-empty") {
			empty = true
		}
		if hasClass(n, "tracking-widget") {
			widget = true
		}
		if !hasClass(n, "tracking-widget__list-item") || hasClass(n, "tracking-widget__list-item_loader") || hasClass(n, "tracking-widget__list-item_banner") {
			return
		}
		var event shop.TrackingEvent
		walk(n, func(field *html.Node) {
			switch {
			case hasClass(field, "tracking-widget__date"):
				event.Date = nodeText(field)
			case hasClass(field, "tracking-widget__date-day"):
				event.Time = nodeText(field)
			case hasClass(field, "tracking-widget__translated"):
				event.Description = strings.TrimSpace(attr(field, "data-original"))
				if event.Description == "" {
					event.Description = nodeText(field)
				}
			case hasClass(field, "tracking-widget__list-country"):
				event.Location = nodeText(field)
			}
		})
		if event.Description == "" || event.Date == "" {
			incomplete = true
			return
		}
		events = append(events, event)
	})
	if widget && empty {
		return nil, upstreamError("history_unavailable")
	}
	if !widget || incomplete {
		return nil, upstreamError("unexpected_markup")
	}
	if len(events) == 0 {
		return nil, upstreamError("history_unavailable")
	}

	return events, nil
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
