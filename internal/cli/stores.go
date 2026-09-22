package cli

import (
	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/config"
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
			registry, err := config.LoadRegistry(c.configPath)
			if err != nil {
				return shop.Errorf(shop.ErrConfigError, "load registry: %v", err)
			}
			if provider != "" {
				entries := registry.FilterByProvider(provider)

				return c.output(entries)
			}

			return c.output(registry.Stores)
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

			return c.output(out)
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

			return c.output(caps)
		},
	}
}
