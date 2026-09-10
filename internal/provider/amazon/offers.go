package amazon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/saucesteals/shop"
	"golang.org/x/net/html"
)

const aodPageSize = 10

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
	if q.Page > 100 {
		return nil, shop.Errorf(shop.ErrInvalidInput, "Amazon offers support pages 1–100")
	}

	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}

	// Amazon serves fixed ten-offer batches. Replay them to expose stable logical
	// pages for arbitrary caller page sizes without skipping offers.
	start, end := (q.Page-1)*q.PageSize, q.Page*q.PageSize
	result := &shop.OffersResult{
		Offers: []shop.Offer{},
		Page:   q.Page,
	}
	seen := make(map[string]bool)
	position := 0
	for amazonPage := 1; position <= end; amazonPage++ {
		offers, exhausted, err := s.fetchAODOffers(ctx, api, productID, amazonPage)
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

func (s *Store) fetchAODOffers(ctx context.Context, api *tvssClient, productID string, page int) ([]shop.Offer, bool, error) {
	params := url.Values{
		"asin": {productID},
	}
	if page > 1 {
		params.Set("isonlyrenderofferlist", "true")
		params.Set("pageno", fmt.Sprintf("%d", page))
	}
	rawURL := fmt.Sprintf("https://www.%s/gp/product/ajax/aodAjaxMain?%s", s.handle, params.Encode())

	// The anonymous response is the authoritative seller catalog. Never return
	// an authenticated buy-box-only response as though it were complete.
	publicBody, err := fetchAODPage(ctx, api, rawURL, false)
	if err != nil {
		return nil, false, err
	}
	publicOffers, publicCount, err := parseAODOffers(publicBody, s.currency, api.marketplaceID)
	if err != nil {
		return nil, false, err
	}

	// Amazon Business sessions can collapse AOD to the account-selected offer.
	// Merge it when available, but a transient authenticated-page failure must
	// not hide the complete public catalog.
	authenticatedBody, authenticatedFetchErr := fetchAODPage(ctx, api, rawURL, true)
	if authenticatedFetchErr != nil {
		return publicOffers, publicCount < aodPageSize, nil
	}
	authenticatedOffers, _, authenticatedParseErr := parseAODOffers(authenticatedBody, s.currency, api.marketplaceID)
	if authenticatedParseErr != nil {
		return publicOffers, publicCount < aodPageSize, nil
	}
	if len(authenticatedOffers) > 0 {
		for i := range publicOffers {
			publicOffers[i].IsBuyBox = false
		}
	}

	return mergeAODOffers(authenticatedOffers, publicOffers), publicCount < aodPageSize, nil
}

func fetchAODPage(ctx context.Context, api *tvssClient, rawURL string, authenticated bool) ([]byte, error) {
	var (
		req *http.Request
		err error
	)
	if authenticated {
		req, err = api.newRequest(ctx, http.MethodGet, rawURL, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	}
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "build Amazon offers request: %v", err)
	}
	req.Header.Set("User-Agent", mobileUA)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := api.http
	if !authenticated {
		clone := *api.http
		clone.Jar = nil
		client = &clone
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "Amazon offers request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "read Amazon offers response: %v", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, shop.Errorf(shop.ErrAuthExpired, "Amazon offers auth expired (%d)", resp.StatusCode)
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

	offer := shop.Offer{
		ID:        offerIDFromNode(node),
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
	for _, input := range reviewElements(node, "input") {
		if strings.HasSuffix(reviewAttr(input, "name"), "[offerListingId]") {
			raw := reviewAttr(input, "value")
			offerID, err := url.PathUnescape(raw)
			if err != nil {
				return raw
			}
			return offerID
		}
	}

	return ""
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

func mergeAODOffers(primary, additional []shop.Offer) []shop.Offer {
	seen := make(map[string]bool, len(primary)+len(additional))
	merged := make([]shop.Offer, 0, len(primary)+len(additional))
	appendUnique := func(offer shop.Offer) {
		key := aodOfferKey(offer)
		if seen[key] {
			return
		}
		seen[key] = true
		merged = append(merged, offer)
	}
	for _, offer := range primary {
		appendUnique(offer)
	}
	for _, offer := range additional {
		appendUnique(offer)
	}

	return merged
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
