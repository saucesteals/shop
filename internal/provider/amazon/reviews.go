package amazon

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/saucesteals/shop"
)

// Reviews combines TVSS aggregate ratings with full web review text. Amazon's
// web pagination is cursor-based; pageNumber alone silently repeats page one.
func (s *Store) Reviews(ctx context.Context, productID string, opts *shop.ReviewsQuery) (*shop.ReviewsResult, error) {
	if err := validateASIN(productID); err != nil {
		return nil, err
	}
	q := shop.ReviewsQuery{}
	if opts != nil {
		q = *opts
	}
	if q.Page < 0 || q.PageSize < 0 || q.PageSize > 100 {
		return nil, shop.Errorf(shop.ErrInvalidInput, "page must be positive and page-size must be between 1 and 100")
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 10
	}
	if q.Page > 100 {
		return nil, shop.Errorf(shop.ErrInvalidInput, "Amazon reviews support pages 1–100")
	}
	if q.Rating != nil && (*q.Rating < 1 || *q.Rating > 5) {
		return nil, shop.Errorf(shop.ErrInvalidInput, "rating must be between 1 and 5")
	}
	if q.Sort != "" && q.Sort != shop.ReviewSortHelpful && q.Sort != shop.ReviewSortRecent {
		return nil, shop.Errorf(shop.ErrInvalidInput, "Amazon reviews support sort helpful or recent")
	}
	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}
	base := "https://www." + s.handle
	body, err := fetchReviewPage(ctx, api, base+"/product-reviews/"+productID+"/", nil, "")
	if err != nil {
		return nil, err
	}
	page, err := parseReviewPage(body)
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(page.Endpoint)
	if err != nil || endpoint == nil || endpoint.IsAbs() || endpoint.Host != "" || !strings.HasPrefix(endpoint.Path, "/portal/customer-reviews/ajax/") || strings.Contains(page.Endpoint, "\\") {
		return nil, shop.Errorf(shop.ErrStoreError, "Amazon review pagination endpoint is missing or unsupported")
	}
	if page.CSRF == "" {
		return nil, shop.Errorf(shop.ErrStoreError, "Amazon review session is missing; log in again")
	}
	result := &shop.ReviewsResult{Page: q.Page, Reviews: []shop.Review{}}
	var aggregate tvssReviewsResponse
	if err := api.doGet(ctx, api.tvssPath([]string{"products", productID, "customer-reviews"}, url.Values{"page-size": {"10"}}), &aggregate); err != nil {
		return nil, err
	}
	if r := aggregate.ProductStarRatings; r != nil {
		result.Rating = shop.Rating{
			Average: r.OverallAverageRating.Float64(),
			Count:   r.Count,
			Stars: &shop.StarBreakdown{
				Five: r.FiveStarPercent.Float64(), Four: r.FourStarPercent.Float64(),
				Three: r.ThreeStarPercent.Float64(), Two: r.TwoStarPercent.Float64(), One: r.OneStarPercent.Float64(),
			},
		}
	} else {
		result.Rating.Count = aggregate.OneStarCount + aggregate.TwoStarCount + aggregate.ThreeStarCount + aggregate.FourStarCount + aggregate.FiveStarCount
	}
	params := url.Values{"asin": {productID}, "pageSize": {"10"}, "deviceType": {"mobile"}, "reviewerType": {"all_reviews"}, "sortBy": {"helpful"}, "filterByStar": {"all_stars"}}
	if q.Sort != "" {
		params.Set("sortBy", string(q.Sort))
	}
	if q.Rating != nil {
		params.Set("filterByStar", []string{"", "one_star", "two_star", "three_star", "four_star", "five_star"}[*q.Rating])
	}
	// Expose stable-size CLI pages over Amazon’s native ten-review batches.
	// Cursors are replayed per invocation, never persisted as account state.
	start, end := (q.Page-1)*q.PageSize, q.Page*q.PageSize
	seen := map[string]bool{}
	position := 0
	for batch := 1; position < end; batch++ {
		params.Set("pageNumber", strconv.Itoa(batch))
		body, err = fetchReviewPage(ctx, api, base+endpoint.String(), params, page.CSRF)
		if err != nil {
			return nil, err
		}
		fragment, err := decodeReviewStream(body)
		if err != nil {
			return nil, err
		}
		current, err := parseReviewPage(fragment)
		if err != nil {
			return nil, err
		}
		for _, review := range current.Reviews {
			if seen[review.ID] {
				return nil, shop.Errorf(shop.ErrStoreError, "Amazon repeated a review page; pagination could not be verified")
			}
			seen[review.ID] = true
			if position >= start && position < end {
				result.Reviews = append(result.Reviews, review)
			}
			position++
		}
		result.HasMore = position > end || current.Next != ""
		if current.Next == "" {
			break
		}
		if len(current.Reviews) == 0 || current.Next == params.Get("nextPageToken") {
			return nil, shop.Errorf(shop.ErrStoreError, "Amazon review pagination made no progress")
		}
		params.Set("nextPageToken", current.Next)
	}
	return result, nil
}

// fetchReviewPage never includes remote bodies or session values in errors.
func fetchReviewPage(ctx context.Context, api *tvssClient, target string, form url.Values, csrf string) ([]byte, error) {
	method := http.MethodGet
	var reader io.Reader
	if form != nil {
		method = http.MethodPost
		reader = strings.NewReader(form.Encode())
	}
	req, err := api.newRequest(ctx, method, target, reader)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "build reviews request")
	}
	req.Header.Set("User-Agent", mobileUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept", "text/html")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("anti-csrftoken-a2z", csrf)
	}
	client := *api.http
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" || r.URL.Host != req.URL.Host || strings.HasPrefix(r.URL.Path, "/ap/") {
			return http.ErrUseLastResponse
		}
		if len(via) >= 5 {
			return http.ErrUseLastResponse
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "reviews request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, shop.Errorf(shop.ErrAuthExpired, "Amazon reviews require a valid session; log in again")
	}
	if resp.StatusCode != 200 {
		return nil, shop.Errorf(shop.ErrStoreError, "Amazon reviews returned HTTP %d", resp.StatusCode)
	}
	const maxBody = 8 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "read reviews response")
	}
	if len(body) > maxBody {
		return nil, shop.Errorf(shop.ErrStoreError, "Amazon reviews response exceeded size limit")
	}
	return body, nil
}

// A loaded event precedes the content and is not completion. Require a
// review-list operation followed by pagination, including explicit empty pages.
func decodeReviewStream(body []byte) ([]byte, error) {
	var list, pagination strings.Builder
	listSeen, paginationSeen := false, false
	for _, part := range strings.Split(string(body), "&&&") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		var chunk []json.RawMessage
		if json.Unmarshal([]byte(part), &chunk) != nil || len(chunk) == 0 {
			return nil, shop.Errorf(shop.ErrStoreError, "unexpected Amazon reviews response")
		}
		var op string
		if json.Unmarshal(chunk[0], &op) != nil {
			return nil, shop.Errorf(shop.ErrStoreError, "invalid Amazon reviews operation")
		}
		if op != "append" && op != "update" {
			if op == "loaded" {
				continue
			}
			return nil, shop.Errorf(shop.ErrStoreError, "unsupported Amazon reviews operation")
		}
		if len(chunk) != 3 {
			return nil, shop.Errorf(shop.ErrStoreError, "invalid Amazon reviews fragment")
		}
		var selector, markup string
		if json.Unmarshal(chunk[1], &selector) != nil || json.Unmarshal(chunk[2], &markup) != nil {
			return nil, shop.Errorf(shop.ErrStoreError, "invalid Amazon reviews fragment")
		}
		switch selector {
		case "#cm_cr-review_list":
			if op == "update" {
				list.Reset()
			}
			list.WriteString(markup)
			listSeen = true
			paginationSeen = false
		case "#reviews-pagination", "#cm_cr-pagination_bar":
			if op == "update" {
				pagination.Reset()
			}
			pagination.WriteString(markup)
			paginationSeen = listSeen
		}
	}
	if !listSeen || !paginationSeen {
		return nil, shop.Errorf(shop.ErrStoreError, "Amazon review response ended before content and pagination were complete")
	}
	return []byte(list.String() + pagination.String()), nil
}
