package cli

import "github.com/spf13/cobra"

func (c *CLI) newOrderCmd() *cobra.Command {
	order := &cobra.Command{
		Use:   "order",
		Short: "Order management",
	}

	order.AddCommand(c.newOrderPlaceCmd())

	return order
}

func (c *CLI) newOrderPlaceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "place <checkout-id>",
		Short: "Place the order (point of no return)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			result, err := s.PlaceOrder(ctx, args[0])
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}
}
