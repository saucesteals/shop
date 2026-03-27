package cli

import (
	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
)

func (c *CLI) newCheckoutCmd() *cobra.Command {
	var (
		addressID  string
		paymentID  string
		shippingID string
		coupon     string
	)

	cmd := &cobra.Command{
		Use:   "checkout",
		Short: "Preview the order (does NOT place it)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			opts := &shop.CheckoutOpts{
				AddressID:       addressID,
				PaymentMethodID: paymentID,
				ShippingOption:  shippingID,
				CouponCode:      coupon,
			}

			result, err := s.Checkout(ctx, opts)
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}

	f := cmd.Flags()
	f.StringVar(&addressID, "address", "", "shipping address ID")
	f.StringVar(&paymentID, "payment", "", "payment method ID")
	f.StringVar(&shippingID, "shipping", "", "shipping option ID")
	f.StringVar(&coupon, "coupon", "", "coupon/promo code")

	return cmd
}
