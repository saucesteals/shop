package cli

import (
	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
)

// storeInfoOutput combines store metadata with capabilities for the
// "store info" command output.
type storeInfoOutput struct {
	shop.StoreInfo
	Capabilities shop.Capabilities `json:"capabilities"`
}

func (c *CLI) newStoresCmd() *cobra.Command {
	var provider string

	cmd := &cobra.Command{
		Use:   "stores",
		Short: "List known stores in the registry",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if provider != "" {
				entries := c.app.Registry.FilterByProvider(provider)

				return c.outputJSON(entries)
			}

			return c.outputJSON(c.app.Registry.Stores)
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "filter by provider name")

	return cmd
}

func (c *CLI) newStoreCmd() *cobra.Command {
	store := &cobra.Command{
		Use:   "store",
		Short: "Store details",
	}

	store.AddCommand(c.newStoreInfoCmd())

	return store
}

func (c *CLI) newStoreInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show details about a specific store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			out := storeInfoOutput{
				StoreInfo:    s.Info(),
				Capabilities: s.Capabilities(),
			}

			return c.outputJSON(out)
		},
	}
}

func (c *CLI) newCapabilitiesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "capabilities",
		Short: "Show what a store supports",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			caps := s.Capabilities()

			return c.outputJSON(caps)
		},
	}
}
