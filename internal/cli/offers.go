package cli

import (
	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
)

func (c *CLI) newOffersCmd() *cobra.Command {
	var (
		condition string
		page      int
		pageSize  int
	)

	cmd := &cobra.Command{
		Use:   "offers <product-id>",
		Short: "List offers (sellers) for a product",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			opts := &shop.OffersQuery{
				Condition: shop.OfferCondition(condition),
				Page:      page,
				PageSize:  pageSize,
			}

			result, err := s.Offers(ctx, args[0], opts)
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}

	f := cmd.Flags()
	f.StringVar(&condition, "condition", "", "filter: new|used_like_new|used_good|used_fair|refurbished")
	f.IntVar(&page, "page", 1, "page number")
	f.IntVar(&pageSize, "page-size", 0, "results per page")

	return cmd
}
