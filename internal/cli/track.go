package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/tracking"
	"github.com/saucesteals/shop/tracking/providers"
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
			client, err := providers.New(nil)
			if err != nil {
				return err
			}
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
	var entry tracking.Shipment
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
	var selection tracking.Selection
	cmd := &cobra.Command{Use: "list", Short: "List active and recently delivered shipments (offline)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := selection.Validate(); err != nil {
				return err
			}
			entries, err := c.shipmentLedger().List()
			if err != nil {
				return err
			}

			return c.outputJSON(selection.Select(entries, time.Now()))
		},
	}
	shipmentSelectionFlags(cmd, &selection)

	return cmd
}

func shipmentSelectionFlags(cmd *cobra.Command, selection *tracking.Selection) {
	cmd.Flags().BoolVar(&selection.All, "all", false, "include all saved shipments, including older deliveries")
	cmd.Flags().StringVar(&selection.DeliveredSince, "delivered-since", "", "include deliveries on or after YYYY-MM-DD (default today)")
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
	var selection tracking.Selection
	cmd := &cobra.Command{
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
			client, err := providers.New(nil)
			if err != nil {
				return err
			}
			result, err := c.shipmentLedger().Refresh(ctx, client, number, selection)
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
	shipmentSelectionFlags(cmd, &selection)

	return cmd
}
