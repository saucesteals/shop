package cli

import "github.com/spf13/cobra"

func (c *CLI) newProductCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "product <product-id>",
		Short: "Get full product details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			product, err := s.Product(ctx, args[0])
			if err != nil {
				return err
			}

			return c.outputJSON(product)
		},
	}
}
