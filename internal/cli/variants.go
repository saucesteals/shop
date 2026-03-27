package cli

import "github.com/spf13/cobra"

func (c *CLI) newVariantsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "variants <product-id>",
		Short: "Get variant tree for a product",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			result, err := s.Variants(ctx, args[0])
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}
}
