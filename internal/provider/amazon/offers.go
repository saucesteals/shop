package amazon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/saucesteals/shop"
	"golang.org/x/net/html"
)

const (
	aodPageSize     = 10
	aodMaxPages     = 100
	aodMaxBodyBytes = 8 << 20
)

// Offers returns every offer on the requested Amazon All Offers Display page.
func (s *Store) Offers(ctx context.Context, productID string, query *shop.OffersQuery) (*shop.OffersResult, error) {
	if err := validateASIN(productID); err != nil {
		return nil, err
	}
	q := shop.OffersQuery{}
	if query != nil {
		q = *query
	}
	if q.Page < 0 || q.PageSize < 0 || q.PageSize > 100 {
		return nil, shop.Errorf(shop.ErrInvalidInput, "page must be positive and page-size must be between 1 and 100")
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = aodPageSize
	}
	// Amazon serves fixed ten-offer batches. Replay them to expose stable logical
	// pages for arbitrary caller page sizes without skipping offers.
	maxLogicalPage := (aodPageSize*aodMaxPages + q.PageSize - 1) / q.PageSize
	if q.Page > maxLogicalPage {
		return nil, shop.Errorf(shop.ErrInvalidInput, "Amazon offers support offsets below %d", aodPageSize*aodMaxPages)
	}
	start, end := (q.Page-1)*q.PageSize, q.Page*q.PageSize
	result := &shop.OffersResult{
		Offers: []shop.Offer{},
		Page:   q.Page,
	}
	seen := make(map[string]bool)
	position := 0
	for amazonPage := 1; position <= end && amazonPage <= aodMaxPages; amazonPage++ {
		offers, exhausted, err := s.fetchAODOffers(ctx, productID, amazonPage)
		if err != nil {
			return nil, err
		}

		newOffers := 0
		for _, offer := range offers {
			key := aodOfferKey(offer)
			if seen[key] {
				continue
			}
			seen[key] = true
			newOffers++
			if q.Condition != shop.ConditionAny && offer.Condition != q.Condition {
				continue
			}
			if position >= start && position < end {
				result.Offers = append(result.Offers, offer)
			}
			position++
		}
		if exhausted || newOffers == 0 {
			break
		}
	}
	result.HasMore = position > end

	return result, nil
}

func (s *Store) fetchAODOffers(ctx context.Context, productID string, page int) ([]shop.Offer, bool, error) {
	params := url.Values{
		"asin": {productID},
	}
	if page > 1 {
		params.Set("isonlyrenderofferlist", "true")
		params.Set("pageno", fmt.Sprintf("%d", page))
	}
	rawURL := fmt.Sprintf("https://www.%s/gp/product/ajax/aodAjaxMain?%s", s.handle, params.Encode())

	body, err := s.fetchAODPage(ctx, rawURL)
	if err != nil {
		return nil, false, err
	}
	offers, count, err := parseAODOffers(body, s.currency, s.marketplaceID)
	if err != nil {
		return nil, false, err
	}

	return offers, count < aodPageSize, nil
}

func (s *Store) fetchAODPage(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "build Amazon offers request: %v", err)
	}
	req.Header.Set("User-Agent", mobileUA)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "Amazon offers request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, aodMaxBodyBytes+1))
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "read Amazon offers response: %v", err)
	}
	if len(body) > aodMaxBodyBytes {
		return nil, shop.Errorf(shop.ErrStoreError, "Amazon offers response exceeded size limit")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, shop.Errorf(shop.ErrStoreError, "Amazon offers access denied (%d)", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, shop.Errorf(shop.ErrNotFound, "Amazon offers not found")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, shop.Errorf(shop.ErrRateLimited, "Amazon offers rate limited")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, shop.Errorf(shop.ErrStoreError, "Amazon offers returned %d: %s", resp.StatusCode, truncateBody(body))
	}

	return body, nil
}

func parseAODOffers(body []byte, currency, marketplaceID string) ([]shop.Offer, int, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, 0, shop.Errorf(shop.ErrStoreError, "parse Amazon offers")
	}

	pinned := reviewFind(doc, "id", "aod-pinned-offer")
	other := reviewFind(doc, "class", "aod-other-offer-block")
	if len(pinned) == 0 && len(other) == 0 {
		return nil, 0, shop.Errorf(shop.ErrStoreError, "Amazon returned unrecognized offers content")
	}

	offers := make([]shop.Offer, 0, len(pinned)+len(other))
	for _, node := range pinned {
		offer, err := parseAODOffer(node, currency, marketplaceID)
		if err != nil {
			return nil, 0, err
		}
		offer.IsBuyBox = true
		offers = append(offers, offer)
	}
	for _, node := range other {
		offer, err := parseAODOffer(node, currency, marketplaceID)
		if err != nil {
			return nil, 0, err
		}
		offers = append(offers, offer)
	}

	return offers, len(other), nil
}

func parseAODOffer(node *html.Node, currency, marketplaceID string) (shop.Offer, error) {
	priceText := reviewText(node)
	price := parsePriceCents(priceText, currency)
	if price == 0 {
		return shop.Offer{}, shop.Errorf(shop.ErrStoreError, "Amazon offer is missing a valid price")
	}

	sellerNode := firstNode(node, "id", "aod-offer-soldBy")
	if sellerNode == nil {
		return shop.Offer{}, shop.Errorf(shop.ErrStoreError, "Amazon offer is missing seller information")
	}
	sellerName := sellerNameFromNode(sellerNode)
	sellerID := sellerIDFromNode(sellerNode)
	if isAmazonSeller(sellerName) {
		sellerName = "Amazon"
		if sellerID == "" {
			sellerID = marketplaceID
		}
	}
	offerID := offerIDFromNode(node)
	if offerID == "" {
		return shop.Offer{}, shop.Errorf(shop.ErrStoreError, "Amazon offer is missing its offer ID")
	}

	offer := shop.Offer{
		ID:        offerID,
		Seller:    shop.Seller{ID: sellerID, Name: sellerName},
		Condition: parseAODCondition(reviewFirstText(node, "id", "aod-offer-heading")),
		Price: shop.Money{
			Amount:   price,
			Currency: currency,
		},
		Availability: shop.Availability{Status: shop.AvailabilityInStock},
		IsPrime:      len(reviewFind(node, "class", "a-icon-prime")) > 0,
		DeliveryDate: reviewFirstText(node, "class", "aod-delivery-promise"),
	}

	if shipsFrom := firstNode(node, "id", "aod-offer-shipsFrom"); shipsFrom != nil {
		offer.Shipping = &shop.ShippingInfo{
			From: stripAODLabel(reviewText(shipsFrom), "Ships from"),
		}
	}

	return offer, nil
}

func sellerNameFromNode(node *html.Node) string {
	for _, anchor := range reviewElements(node, "a") {
		if sellerIDFromNode(anchor) != "" {
			return reviewText(anchor)
		}
	}

	name := stripAODLabel(reviewText(node), "Sold by")
	if line, _, ok := strings.Cut(name, "\n"); ok {
		return strings.TrimSpace(line)
	}

	return name
}

func firstNode(node *html.Node, key, value string) *html.Node {
	nodes := reviewFind(node, key, value)
	if len(nodes) == 0 {
		return nil
	}

	return nodes[0]
}

func stripAODLabel(value, label string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len(label) && strings.EqualFold(value[:len(label)], label) {
		value = value[len(label):]
	}

	return strings.TrimSpace(strings.TrimPrefix(value, ":"))
}

func sellerIDFromNode(node *html.Node) string {
	for _, anchor := range reviewElements(node, "a") {
		href := reviewAttr(anchor, "href")
		u, err := url.Parse(href)
		if err == nil && u.Query().Get("seller") != "" {
			return u.Query().Get("seller")
		}
	}

	return ""
}

func offerIDFromNode(node *html.Node) string {
	// Amazon's mobile AOD embeds the offer token as `oid` in a JSON action
	// payload. The HTML parser has already decoded attribute entities here.
	var offerID string
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if offerID != "" {
			return
		}
		if current.Type == html.ElementNode {
			for _, attribute := range []string{"data-aw-aod-cart-api", "data-select-aod-qty-atc"} {
				raw := reviewAttr(current, attribute)
				if raw == "" {
					continue
				}
				var action struct {
					OfferID string `json:"oid"`
				}
				if err := json.Unmarshal([]byte(raw), &action); err == nil && action.OfferID != "" {
					offerID = decodeAODOfferID(action.OfferID)
					return
				}
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	if offerID != "" {
		return offerID
	}

	// Retain compatibility with desktop AOD variants that expose the token as
	// a conventional hidden form field.
	for _, input := range reviewElements(node, "input") {
		if strings.HasSuffix(reviewAttr(input, "name"), "[offerListingId]") {
			return decodeAODOfferID(reviewAttr(input, "value"))
		}
	}

	return ""
}

func decodeAODOfferID(raw string) string {
	offerID, err := url.PathUnescape(raw)
	if err != nil {
		return raw
	}

	return offerID
}

func isAmazonSeller(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))

	return name == "amazon" || strings.HasPrefix(name, "amazon.")
}

func parseAODCondition(value string) shop.OfferCondition {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	switch {
	case strings.Contains(value, "used - like new"):
		return shop.ConditionUsedLikeNew
	case strings.Contains(value, "used - good"):
		return shop.ConditionUsedGood
	case strings.Contains(value, "used - acceptable"), strings.Contains(value, "used - fair"):
		return shop.ConditionUsedFair
	case strings.Contains(value, "renewed"), strings.Contains(value, "refurbished"):
		return shop.ConditionRefurbished
	default:
		return shop.ConditionNew
	}
}

func aodOfferKey(offer shop.Offer) string {
	if offer.ID != "" {
		return "id\x00" + offer.ID
	}

	shipping := ""
	if offer.Shipping != nil {
		shipping = fmt.Sprintf("%s\x00%s\x00%s", offer.Shipping.From, offer.Shipping.Description, offer.Shipping.Speed)
		if offer.Shipping.Price != nil {
			shipping += fmt.Sprintf("\x00%d\x00%s", offer.Shipping.Price.Amount, offer.Shipping.Price.Currency)
		}
	}

	return fmt.Sprintf("fallback\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s", offer.Seller.ID, offer.Seller.Name, offer.Price.Amount, offer.Price.Currency, offer.Condition, shipping)
}
