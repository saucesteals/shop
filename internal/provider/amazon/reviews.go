package amazon

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/url"
	"strconv"

	"github.com/saucesteals/shop"
)

// Reviews fetches customer reviews for a product.
//
// TVSS endpoint: GET /marketplaces/{marketplace}/products/{asin}/customer-reviews
func (s *Store) Reviews(ctx context.Context, productID string, opts *shop.ReviewsQuery) (*shop.ReviewsResult, error) {
	if err := validateASIN(productID); err != nil {
		return nil, err
	}

	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}

	if opts == nil {
		opts = &shop.ReviewsQuery{}
	}

	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}

	params := url.Values{}
	params.Set("page-size", strconv.Itoa(pageSize))

	if opts.Page > 1 {
		params.Set("page-index", strconv.Itoa(opts.Page-1))
	}

	if opts.Rating != nil && *opts.Rating >= 1 && *opts.Rating <= 5 {
		params.Set("stars", strconv.Itoa(*opts.Rating))
	}

	switch opts.Sort {
	case shop.ReviewSortRecent:
		params.Set("sort-by-type", "recent")
	case shop.ReviewSortHelpful:
		params.Set("sort-by-type", "helpful")
	default:
		// TVSS default is helpful
	}

	u := api.tvssPath([]string{"products", productID, "customer-reviews"}, params)

	var resp tvssReviewsResponse
	if err := api.doGet(ctx, u, &resp); err != nil {
		return nil, err
	}

	result := &shop.ReviewsResult{
		Page:    opts.Page,
		HasMore: resp.PageNext != "",
	}
	if result.Page < 1 {
		result.Page = 1
	}

	// Aggregate rating.
	totalReviews := resp.OneStarCount + resp.TwoStarCount + resp.ThreeStarCount +
		resp.FourStarCount + resp.FiveStarCount

	result.Rating = shop.Rating{
		Count: totalReviews,
	}

	if resp.ProductStarRatings != nil {
		result.Rating.Average = resp.ProductStarRatings.OverallAverageRating.Float64()
		result.Rating.Count = resp.ProductStarRatings.Count
		result.Rating.Stars = &shop.StarBreakdown{
			Five:  resp.ProductStarRatings.FiveStarPercent.Float64(),
			Four:  resp.ProductStarRatings.FourStarPercent.Float64(),
			Three: resp.ProductStarRatings.ThreeStarPercent.Float64(),
			Two:   resp.ProductStarRatings.TwoStarPercent.Float64(),
			One:   resp.ProductStarRatings.OneStarPercent.Float64(),
		}
	}

	// Map individual reviews.
	for _, r := range resp.Reviews {
		review := shop.Review{
			Author: r.AuthorName,
			Title:  r.Title,
			Body:   r.Text,
			Date:   r.SubmissionDate,
		}

		// Generate a stable ID from author+title+date to reduce collisions
		// (e.g., multiple "Five Stars" reviews from different authors).
		review.ID = fmt.Sprintf("%x", md5.Sum([]byte(r.AuthorName+r.Title+r.SubmissionDate)))[:12]

		if r.OverallRating != nil {
			review.Rating = int(*r.OverallRating)
		}
		if r.IsVerifiedPurchase != nil && *r.IsVerifiedPurchase {
			review.Verified = true
		}

		for _, img := range r.ImageURLs {
			u := img.LargeImageURL
			if u == "" {
				u = img.MediumImageURL
			}
			if u == "" {
				u = img.SmallImageURL
			}
			if u != "" {
				review.Images = append(review.Images, shop.Image{URL: u})
			}
		}

		result.Reviews = append(result.Reviews, review)
	}

	return result, nil
}
