package cli

import (
	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
)

func (c *CLI) newCartCmd() *cobra.Command {
	cart := &cobra.Command{
		Use:   "cart",
		Short: "Manage the shopping cart",
	}

	cart.AddCommand(
		c.newCartAddCmd(),
		c.newCartRemoveCmd(),
		c.newCartViewCmd(),
		c.newCartClearCmd(),
	)

	return cart
}

func (c *CLI) newCartAddCmd() *cobra.Command {
	var qty int

	cmd := &cobra.Command{
		Use:   "add <product-id>",
		Short: "Add a product to the cart",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if qty < 1 {
				return shop.Errorf(shop.ErrInvalidInput, "quantity must be >= 1, got %d", qty)
			}

			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			result, err := s.Cart().Add(ctx, args[0], qty)
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}

	cmd.Flags().IntVar(&qty, "qty", 1, "quantity to add")

	return cmd
}

func (c *CLI) newCartRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <product-id>",
		Short: "Remove a product from the cart",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			result, err := s.Cart().Remove(ctx, args[0])
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}
}

func (c *CLI) newCartViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view",
		Short: "Display current cart contents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			result, err := s.Cart().View(ctx)
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}
}

func (c *CLI) newCartClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Empty the cart",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			result, err := s.Cart().Clear(ctx)
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}
}
