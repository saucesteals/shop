package cli

import (
	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking/trackglobal"
)

func (c *CLI) newTrackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "track <tracking-number>",
		Short: "Look up available shipment history (experimental)",
		Long:  "Look up cached shipment history without a store login. Cold lookups may be unavailable; results do not guarantee current carrier status.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("store") {
				return shop.Errorf(shop.ErrInvalidInput, "tracking is independent of --store")
			}
			ctx, cancel := c.timeoutCtx(cmd)
			defer cancel()
			client := &trackglobal.Client{}
			result, err := client.Track(ctx, args[0])
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}
}
