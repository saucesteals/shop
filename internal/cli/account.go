package cli

import "github.com/spf13/cobra"

func (c *CLI) newAddressesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "addresses",
		Short: "List saved shipping addresses",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			addresses, err := s.Addresses(ctx)
			if err != nil {
				return err
			}

			return c.outputJSON(addresses)
		},
	}
}

func (c *CLI) newPaymentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "payments",
		Short: "List saved payment methods",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			methods, err := s.PaymentMethods(ctx)
			if err != nil {
				return err
			}

			return c.outputJSON(methods)
		},
	}
}
