package cli

import (
	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
	"github.com/saucesteals/shop/internal/tracking/carriers"
)

func (c *CLI) newTrackCmd() *cobra.Command {
	track := &cobra.Command{
		Use:   "track <tracking-number>",
		Short: "Look up available shipment history (experimental)",
		Long:  "Look up shipment history without a store login. Cold lookups may be unavailable; results do not guarantee current carrier status.",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Root().PersistentPreRunE(cmd, args); err != nil {
				return err
			}
			if cmd.Flags().Changed("store") {
				return shop.Errorf(shop.ErrInvalidInput, "tracking is independent of --store")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := c.timeoutCtx(cmd)
			defer cancel()
			client := &carriers.Client{}
			result, err := client.Track(ctx, args[0])
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}
	track.AddCommand(c.newTrackAddCmd(), c.newTrackListCmd(), c.newTrackRemoveCmd(), c.newTrackRefreshCmd())
	return track
}

func (c *CLI) shipmentLedger() tracking.Ledger {
	return tracking.Ledger{ConfigDir: c.app.ConfigDir}
}

func (c *CLI) newTrackAddCmd() *cobra.Command {
	var entry shop.Shipment
	cmd := &cobra.Command{
		Use: "add <tracking-number>", Short: "Save local shipment attribution", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry.TrackingNumber = args[0]
			saved, err := c.shipmentLedger().Add(entry)
			if err != nil {
				return err
			}
			return c.outputJSON(saved)
		},
	}
	cmd.Flags().StringVar(&entry.Label, "label", "", "descriptive label")
	cmd.Flags().StringVar(&entry.Merchant, "merchant", "", "merchant name")
	cmd.Flags().StringVar(&entry.OrderID, "order-id", "", "associated order identifier")
	cmd.Flags().StringVar(&entry.Note, "note", "", "local note")
	return cmd
}

func (c *CLI) newTrackListCmd() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List saved shipments (offline)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := c.shipmentLedger().List()
			if err != nil {
				return err
			}
			return c.outputJSON(entries)
		},
	}
}

func (c *CLI) newTrackRemoveCmd() *cobra.Command {
	return &cobra.Command{Use: "remove <tracking-number>", Short: "Remove local shipment attribution", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.shipmentLedger().Remove(args[0]); err != nil {
				return err
			}
			return c.outputJSON(struct {
				Removed bool `json:"removed"`
			}{Removed: true})
		},
	}
}

func (c *CLI) newTrackRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh [tracking-number]",
		Short: "Refresh saved shipments and summarize their latest scans",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var number string
			if len(args) > 0 {
				number = args[0]
			}
			ctx, cancel := c.timeoutCtx(cmd)
			defer cancel()
			result, err := c.shipmentLedger().Refresh(ctx, &carriers.Client{}, number)
			if err != nil {
				return err
			}
			if err := c.outputJSON(result); err != nil {
				return err
			}
			if result.Failed > 0 {
				return shop.Errorf(shop.ErrUpstream, "%d shipment refreshes failed", result.Failed)
			}

			return nil
		},
	}
}
