package amazon

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/saucesteals/shop"
	"golang.org/x/net/html"
)

type reviewPage struct {
	Reviews  []shop.Review
	Endpoint string
	CSRF     string
	Next     string
}

// Amazon embeds review-session settings in inline state. Decode JSON string
// values, not JavaScript, and keep all session material internal.
var reviewStateField = regexp.MustCompile(`"(reviewsAjaxUrl|reviewsCsrfToken)"\s*:\s*("(?:\\.|[^"\\])*")`)
var reviewStars = regexp.MustCompile(`a-star-(?:small-)?([1-5])(?:\s|$)`)
var reviewHelpfulCount = regexp.MustCompile(`[\d,]+`)

func parseReviewPage(body []byte) (*reviewPage, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, shop.Errorf(shop.ErrStoreError, "parse Amazon reviews")
	}
	p := &reviewPage{Reviews: []shop.Review{}}
	for _, m := range reviewStateField.FindAllSubmatch(body, -1) {
		var value string
		if json.Unmarshal(m[2], &value) != nil {
			continue
		}
		switch string(m[1]) {
		case "reviewsAjaxUrl":
			p.Endpoint = value
		case "reviewsCsrfToken":
			p.CSRF = value
		}
	}
	for _, n := range reviewFind(doc, "data-hook", "show-more-button") {
		var state struct {
			Next string `json:"nextPageToken"`
		}
		if json.Unmarshal([]byte(reviewAttr(n, "data-reviews-state-param")), &state) != nil || strings.TrimSpace(state.Next) == "" {
			return nil, shop.Errorf(shop.ErrStoreError, "Amazon show-more control is missing valid continuation state")
		}
		p.Next = state.Next
	}
	nodes := reviewFind(doc, "data-hook", "mobley-review-content")
	if len(nodes) == 0 {
		nodes = reviewFind(doc, "data-hook", "review")
	}
	if len(nodes) == 0 && !strings.Contains(reviewText(doc), "Sorry, no reviews match your current selections.") {
		return nil, shop.Errorf(shop.ErrStoreError, "Amazon returned unrecognized review content, not a confirmed empty result")
	}
	for _, n := range nodes {
		r := shop.Review{ID: reviewAttr(n, "id"), Author: reviewFirstText(n, "class", "a-profile-name"), Title: reviewFirstText(n, "data-hook", "review-title"), Date: reviewFirstText(n, "data-hook", "review-date")}
		// Mobile pages contain a shortened copy followed by the full hidden body.
		// Choose the fullest body, preserving line breaks rather than UI controls.
		for _, b := range reviewFind(n, "data-hook", "review-body") {
			if text := reviewText(b); len(text) > len(r.Body) {
				r.Body = text
			}
		}
		for _, hook := range []string{"review-star-rating", "cmps-review-star-rating"} {
			for _, star := range reviewFind(n, "data-hook", hook) {
				if m := reviewStars.FindStringSubmatch(reviewAttr(star, "class")); m != nil {
					r.Rating, _ = strconv.Atoi(m[1])
				}
				// Desktop review titles nest the star label alongside the actual title.
				label := reviewText(star)
				r.Title = strings.TrimSpace(strings.TrimPrefix(r.Title, label))
			}
		}
		r.Verified = len(reviewFind(n, "data-hook", "avp-badge"))+len(reviewFind(n, "data-hook", "msrp-avp-badge-linkless")) > 0
		helpful := reviewFirstText(n, "data-hook", "helpful-vote-statement")
		if strings.HasPrefix(helpful, "One person") {
			r.Helpful = 1
		} else if count := reviewHelpfulCount.FindString(helpful); count != "" {
			r.Helpful, _ = strconv.Atoi(strings.ReplaceAll(count, ",", ""))
		}
		imageSeen := map[string]bool{}
		for _, hook := range []string{"review-image-tile", "review-image-tile-section"} {
			for _, tile := range reviewFind(n, "data-hook", hook) {
				for _, img := range reviewElements(tile, "img") {
					u := reviewAttr(img, "src")
					if strings.HasPrefix(u, "https://") && !imageSeen[u] {
						r.Images = append(r.Images, shop.Image{URL: u})
						imageSeen[u] = true
					}
				}
			}
		}
		if r.Body == "" || r.Rating == 0 {
			return nil, shop.Errorf(shop.ErrStoreError, "Amazon review is missing text or rating; page format may have changed")
		}
		if strings.HasSuffix(r.Body, "...") || strings.HasSuffix(r.Body, "…") {
			r.Attributes = map[string]any{"possiblyTruncated": true}
		}
		if r.ID == "" {
			r.ID = fmt.Sprintf("%x", sha256.Sum256([]byte(r.Author+"\x00"+r.Title+"\x00"+r.Date)))[:24]
		}
		p.Reviews = append(p.Reviews, r)
	}
	return p, nil
}

func reviewAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func reviewFind(n *html.Node, key, value string) []*html.Node {
	var found []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		v := reviewAttr(n, key)
		match := v == value
		if key == "class" {
			match = false
			for _, c := range strings.Fields(v) {
				if c == value {
					match = true
					break
				}
			}
		}
		if n.Type == html.ElementNode && match {
			found = append(found, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return found
}
func reviewElements(n *html.Node, tag string) []*html.Node {
	var found []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			found = append(found, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return found
}
func reviewFirstText(n *html.Node, key, value string) string {
	nodes := reviewFind(n, key, value)
	if len(nodes) > 0 {
		return reviewText(nodes[0])
	}
	return ""
}
func reviewText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && n.Data == "br" {
			b.WriteByte('\n')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	var lines []string
	for _, line := range strings.Split(b.String(), "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
