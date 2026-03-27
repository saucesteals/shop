package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
)

func (c *CLI) newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login [store] [key=value...]",
		Short: "Authenticate with a store",
		Long: `Authenticate with a store. Key=value pairs are passed as credentials.

The store can be specified as a positional argument, via --store flag,
SHOP_STORE env var, or the config default.

Examples:
  shop login amazon                          # device code flow
  shop login nike.com username=x password=y  # password flow
  shop login -s someapi.com token=sk_xxx     # API key flow`,
		RunE: func(cmd *cobra.Command, args []string) error {
			creds := make(map[string]string)
			for _, arg := range args {
				k, v, ok := strings.Cut(arg, "=")
				if !ok {
					// First non-kv arg is the store name.
					if c.store == "" {
						c.store = arg

						continue
					}

					return shop.Errorf(shop.ErrInvalidInput, "unexpected argument %q; credentials must be in key=value format", arg)
				}
				creds[k] = v
			}

			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			result, err := s.Login(ctx, creds)
			if err != nil {
				return err
			}

			return c.outputJSON(result)
		},
	}
}

func (c *CLI) newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout [store]",
		Short: "Revoke and clear credentials",
		Long: `Revoke and clear credentials for a store.

The store can be specified as a positional argument, via --store flag,
SHOP_STORE env var, or the config default.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				c.store = args[0]
			}

			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			if err := s.Logout(ctx); err != nil {
				return err
			}

			return c.outputJSON(map[string]bool{"success": true})
		},
	}
}

func (c *CLI) newWhoAmICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Check authentication state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel, s, err := c.resolveStore(cmd)
			if err != nil {
				return err
			}
			defer cancel()

			info, err := s.WhoAmI(ctx)
			if err != nil {
				return err
			}

			return c.outputJSON(info)
		},
	}
}
