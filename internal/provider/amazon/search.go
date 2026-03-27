package amazon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/saucesteals/shop"
)

// mapSearchSort maps shop.SearchSort values to Amazon's URL sort parameter.
func mapSearchSort(s shop.SearchSort) string {
	switch s {
	case shop.SortPriceLow:
		return "price-asc-rank"
	case shop.SortPriceHigh:
		return "price-desc-rank"
	case shop.SortRating:
		return "review-rank"
	case shop.SortNewest:
		return "date-desc-rank"
	case shop.SortBestSeller:
		return "salesrank"
	default:
		return ""
	}
}

// Search finds products by keyword using Amazon's mobile web search to
// discover ranked ASINs, then enriches them with structured data from the
// TVSS basic-products endpoint.
//
// The TVSS search/legacy endpoint requires Fire TV MAP device tokens which
// are not obtainable through iOS code-pair auth. Mobile web search works
// with any valid session cookies and returns the same catalog results.
func (s *Store) Search(ctx context.Context, query *shop.SearchQuery) (*shop.SearchResult, error) {
	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}

	page := query.Page
	if page < 1 {
		page = 1
	}
	size := query.PageSize
	if size <= 0 {
		size = 16
	}

	// Step 1: Discover ranked ASINs from mobile web search.
	asins, err := s.searchASINs(ctx, api, query.Query, page, mapSearchSort(query.Sort))
	if err != nil {
		return nil, err
	}

	result := &shop.SearchResult{
		Page:    page,
		HasMore: len(asins) >= size,
	}

	if len(asins) == 0 {
		return result, nil
	}

	// Cap to requested page size.
	if len(asins) > size {
		asins = asins[:size]
	}

	// Step 2: Batch lookup via TVSS basic-products for structured data.
	products, err := s.basicProducts(ctx, api, asins)
	if err != nil {
		// Fall back to bare ASIN results if TVSS fails, but surface the
		// error as a warning so the caller knows enrichment was skipped.
		result.Warnings = append(result.Warnings, "TVSS enrichment failed: "+err.Error())
		for _, asin := range asins {
			result.Products = append(result.Products, shop.ProductSummary{
				ID:  asin,
				URL: productURL(s.handle, asin),
			})
		}
		result.Count = len(result.Products)

		return result, nil
	}

	// Build a map for O(1) lookup, then iterate asins to preserve rank order.
	byASIN := make(map[string]*tvssBasicProductEntity, len(products))
	for i := range products {
		if bp := products[i].BasicProduct; bp != nil {
			byASIN[bp.ASIN] = &products[i]
		}
	}

	currency := s.currency

	for _, asin := range asins {
		ps := shop.ProductSummary{
			ID:           asin,
			URL:          productURL(s.handle, asin),
			Availability: shop.Availability{Status: shop.AvailabilityInStock},
		}

		if entity, ok := byASIN[asin]; ok {
			if bp := entity.BasicProduct; bp != nil {
				ps.Title = bp.Title
				ps.ImageURL = bp.ImageURL
				if bp.ListPrice != "" {
					lp := toMoney(bp.ListPrice, currency)
					ps.ListPrice = &lp
				}
				if bp.CustomerReviewsCount > 0 || bp.AverageOverallRating > 0 {
					ps.Rating = &shop.Rating{
						Average: bp.AverageOverallRating / 2, // TVSS returns 0–10; normalize to 0–5
						Count:   bp.CustomerReviewsCount,
					}
				}
			}
			if bo := entity.BasicOffer; bo != nil {
				if bo.Price != "" {
					p := toMoney(bo.Price, currency)
					ps.Price = &p
				}
				if bo.Badge != nil {
					ps.Badge = bo.Badge.Type
				}
			}
		}

		result.Products = append(result.Products, ps)
	}

	result.Count = len(result.Products)

	return result, nil
}

// searchASINs performs a mobile web search on the store's domain and extracts
// unique ASINs in rank order from the HTML response.
func (s *Store) searchASINs(ctx context.Context, api *tvssClient, keyword string, page int, sort string) ([]string, error) {
	params := url.Values{}
	params.Set("k", keyword)
	params.Set("ref", "nb_sb_noss")
	if page > 1 {
		params.Set("page", fmt.Sprintf("%d", page))
	}
	if sort != "" {
		params.Set("s", sort)
	}

	searchURL := fmt.Sprintf("https://www.%s/s?%s", s.handle, params.Encode())

	req, err := api.newRequest(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "build search request: %v", err)
	}

	req.Header.Set("User-Agent", mobileUA)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := api.http.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "search request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, shop.Errorf(shop.ErrStoreError, "search returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "read search response: %v", err)
	}

	return parseOrganicASINs(body)
}

// parseOrganicASINs walks an Amazon search result HTML document and returns
// ASINs for organic results in page rank order. It targets only nodes with
// data-component-type="s-search-result", which Amazon uses exclusively for
// organic results — sponsored/featured units use different component types.
func parseOrganicASINs(body []byte) ([]string, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "parse search HTML: %v", err)
	}

	seen := make(map[string]bool)
	var asins []string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			var componentType, asin string
			for _, a := range n.Attr {
				switch a.Key {
				case "data-component-type":
					componentType = a.Val
				case "data-asin":
					asin = a.Val
				}
			}
			if componentType == "s-search-result" && asin != "" && !seen[asin] {
				seen[asin] = true
				asins = append(asins, asin)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return asins, nil
}

// basicProducts fetches structured product data for a batch of ASINs via
// the TVSS basic-products endpoint.
//
// TVSS endpoint: GET /marketplaces/{marketplace}/basicproducts/{asins}?get-deals=false
func (s *Store) basicProducts(ctx context.Context, api *tvssClient, asins []string) ([]tvssBasicProductEntity, error) {
	joined := strings.Join(asins, ",")
	params := url.Values{}
	params.Set("get-deals", "false")
	u := api.tvssPath([]string{"basicproducts", joined}, params)

	var resp tvssBasicProductsResponse
	if err := api.doGet(ctx, u, &resp); err != nil {
		return nil, err
	}

	return resp.Products, nil
}
