package cli

import (
	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
)

func (c *CLI) newReviewsCmd() *cobra.Command {
	var (
		page     int
		pageSize int
		sortBy   string
		rating   int
	)

	cmd := &cobra.Command{
		Use:   "reviews <product-id>",
		Short: "Get customer reviews for a product",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			opts := &shop.ReviewsQuery{
				Page:     page,
				PageSize: pageSize,
				Sort:     shop.ReviewSort(sortBy),
			}
			if cmd.Flags().Changed("rating") {
				opts.Rating = &rating
			}

			result, err := s.Reviews(ctx, args[0], opts)
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}

	f := cmd.Flags()
	f.IntVar(&page, "page", 1, "page number")
	f.IntVar(&pageSize, "page-size", 0, "results per page")
	f.StringVar(&sortBy, "sort", "", "sort: recent|helpful|rating")
	f.IntVar(&rating, "rating", 0, "filter to specific star rating (1-5)")

	return cmd
}
