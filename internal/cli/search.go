package cli

import (
	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
)

func (c *CLI) newSearchCmd() *cobra.Command {
	var (
		page      int
		cursor    string
		pageSize  int
		sortBy    string
		minPrice  int64
		maxPrice  int64
		minRating float64
		category  string
		filters   []string
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for products",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			q := &shop.SearchQuery{
				Cursor:   cursor,
				Query:    args[0],
				Page:     page,
				PageSize: pageSize,
				Sort:     shop.SearchSort(sortBy),
				Category: category,
			}
			if cursor != "" && !cmd.Flags().Changed("page") {
				q.Page = 0
			}

			if cmd.Flags().Changed("min-price") {
				q.MinPrice = &minPrice
			}
			if cmd.Flags().Changed("max-price") {
				q.MaxPrice = &maxPrice
			}
			if cmd.Flags().Changed("min-rating") {
				q.MinRating = &minRating
			}

			if len(filters) > 0 {
				q.Filters = parseKeyValues(filters)
			}

			result, err := s.Search(ctx, q)
			if err != nil {
				return err
			}

			return c.output(result)
		},
	}

	f := cmd.Flags()
	f.IntVar(&page, "page", 1, "page number")
	f.StringVar(&cursor, "cursor", "", "search continuation token (provider-specific)")
	f.IntVar(&pageSize, "page-size", 0, "results per page")
	f.StringVar(&sortBy, "sort", "", "sort: relevance|price_low|price_high|rating|newest|best_seller")
	f.Int64Var(&minPrice, "min-price", 0, "minimum price in cents")
	f.Int64Var(&maxPrice, "max-price", 0, "maximum price in cents")
	f.Float64Var(&minRating, "min-rating", 0, "minimum average rating")
	f.StringVar(&category, "category", "", "category filter (provider-specific)")
	f.StringArrayVar(&filters, "filter", nil, "arbitrary filter key=value (repeatable)")

	return cmd
}
