package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/tracking"
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
			client := c.client.Tracking()
			result, err := client.Track(ctx, args[0])
			if err != nil {
				return shipmentError(err, shop.ErrNetwork)
			}

			return c.output(result)
		},
	}
	track.AddCommand(c.newTrackAddCmd(), c.newTrackListCmd(), c.newTrackRemoveCmd(), c.newTrackRefreshCmd())

	return track
}

func (c *CLI) newTrackAddCmd() *cobra.Command {
	var entry tracking.Shipment
	cmd := &cobra.Command{
		Use:   "add <tracking-number>",
		Short: "Save local shipment attribution",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry.TrackingNumber = args[0]
			saved, err := c.client.Tracking().Add(cmd.Context(), entry)
			if err != nil {
				return shipmentError(err, shop.ErrConfigError)
			}

			return c.output(saved)
		},
	}
	cmd.Flags().StringVar(&entry.Label, "label", "", "descriptive label")
	cmd.Flags().StringVar(&entry.Merchant, "merchant", "", "merchant name")
	cmd.Flags().StringVar(&entry.OrderID, "order-id", "", "associated order identifier")
	cmd.Flags().StringVar(&entry.Note, "note", "", "local note")

	return cmd
}

type shipmentFilterFlags struct {
	all   bool
	since string
}

func (f *shipmentFilterFlags) bind(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.all, "all", false, "include all saved shipments, including older deliveries")
	cmd.Flags().StringVar(&f.since, "delivered-since", "", "include deliveries on or after YYYY-MM-DD (default today)")
}

func (f shipmentFilterFlags) filter() (tracking.Filter, error) {
	filter := tracking.Filter{All: f.all}
	if f.since != "" {
		date, err := time.Parse(time.DateOnly, f.since)
		if err != nil {
			return filter, shop.Errorf(shop.ErrInvalidInput, "--delivered-since must be YYYY-MM-DD")
		}
		filter.DeliveredSince = date
	}

	return filter, filter.Validate()
}

func (c *CLI) newTrackListCmd() *cobra.Command {
	var flags shipmentFilterFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List active and recently delivered shipments (offline)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			filter, err := flags.filter()
			if err != nil {
				return err
			}
			entries, err := c.client.Tracking().List(cmd.Context(), filter)
			if err != nil {
				return shipmentError(err, shop.ErrConfigError)
			}

			return c.output(entries)
		},
	}
	flags.bind(cmd)

	return cmd
}

func (c *CLI) newTrackRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <tracking-number>",
		Short: "Remove local shipment attribution",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.client.Tracking().Remove(cmd.Context(), args[0]); err != nil {
				return shipmentError(err, shop.ErrConfigError)
			}

			return c.output(removalResult{Removed: true})
		},
	}
}

func (c *CLI) newTrackRefreshCmd() *cobra.Command {
	var flags shipmentFilterFlags
	cmd := &cobra.Command{
		Use:   "refresh [tracking-number]",
		Short: "Refresh saved shipments and summarize their latest scans",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filter, err := flags.filter()
			if err != nil {
				return err
			}
			ctx, cancel := c.timeoutCtx(cmd)
			defer cancel()
			store := c.client.Tracking()
			var results []tracking.Result
			var refreshErr error
			if len(args) > 0 {
				shipment, err := store.Refresh(ctx, args[0])
				if shipment == nil {
					return shipmentError(err, shop.ErrConfigError)
				}
				results = []tracking.Result{{Shipment: *shipment, Err: err}}
			} else {
				results, refreshErr = store.RefreshAll(ctx, tracking.RefreshOptions{Filter: filter})
				if results == nil && refreshErr != nil {
					return shipmentError(refreshErr, shop.ErrConfigError)
				}
			}
			summary := summarizeRefresh(results)
			if err := c.output(summary); err != nil {
				return err
			}
			if refreshErr != nil {
				return shipmentError(refreshErr, shop.ErrNetwork)
			}
			if summary.Failed > 0 {
				return shop.Errorf(shop.ErrUpstream, "%d shipment refreshes failed", summary.Failed)
			}

			return nil
		},
	}
	flags.bind(cmd)

	return cmd
}
