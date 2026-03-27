// Package cli defines the cobra command tree for the shop CLI.
package cli

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/app"
	"github.com/saucesteals/shop/internal/config"
)

// Version is set at build time via ldflags. Defaults to "dev" for
// development builds.
var Version = "dev"

// CLI holds shared state across all commands.
type CLI struct {
	app *app.App

	// Global flags.
	store      string
	jsonOutput bool
	pretty     bool
	configPath string
	timeout    time.Duration
}

// New returns the root cobra command with all subcommands wired up.
func New() *cobra.Command {
	var c CLI

	root := &cobra.Command{
		Use:           "shop",
		Short:         "Multi-platform shopping CLI",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Skip app init for commands that don't need it.
			if cmd.Name() == "help" || cmd.Name() == "completion" {
				return nil
			}

			dir := c.configPath
			if dir == "" {
				dir = config.DefaultDir()
			}

			a, err := app.New(dir)
			if err != nil {
				return err
			}

			c.app = a

			// Wire config output settings into CLI flags when not
			// explicitly overridden by command-line flags.
			if !cmd.Flags().Changed("json") && a.Config.Defaults.Output.JSON {
				c.jsonOutput = true
			}
			if !cmd.Flags().Changed("pretty") && a.Config.Defaults.Output.Pretty {
				c.pretty = true
			}

			// Wire config timeout into CLI flag when not explicitly
			// overridden by command-line flags.
			if !cmd.Flags().Changed("timeout") && a.Config.Defaults.Timeout != "" {
				if d, err := time.ParseDuration(a.Config.Defaults.Timeout); err == nil {
					c.timeout = d
				}
			}

			return nil
		},
	}

	// Global persistent flags.
	pf := root.PersistentFlags()
	pf.StringVarP(&c.store, "store", "s", "", "target store (name or domain)")
	_ = root.RegisterFlagCompletionFunc("store", c.completeStoreNames)
	pf.BoolVar(&c.jsonOutput, "json", false, "force compact JSON output")
	pf.BoolVar(&c.pretty, "pretty", false, "force pretty-printed JSON")
	pf.StringVar(&c.configPath, "config", "", "config directory path")
	pf.DurationVar(&c.timeout, "timeout", 30*time.Second, "request timeout")

	// Register all commands.
	root.AddCommand(
		c.newConfigCmd(),
		c.newSearchCmd(),
		c.newProductCmd(),
		c.newReviewsCmd(),
		c.newOffersCmd(),
		c.newVariantsCmd(),
		c.newCartCmd(),
		c.newCheckoutCmd(),
		c.newOrderCmd(),
		c.newLoginCmd(),
		c.newLogoutCmd(),
		c.newWhoAmICmd(),
		c.newAddressesCmd(),
		c.newPaymentsCmd(),
		c.newStoresCmd(),
		c.newStoreCmd(),
		c.newCapabilitiesCmd(),
		c.newSkillCmd(),
	)

	return root
}

// Execute runs the root command and handles exit codes via the run() pattern
// so deferred functions execute on all exit paths.
func Execute() {
	if err := run(); err != nil {
		code := outputError(err)
		os.Exit(code)
	}
}

func run() error {
	root := New()

	return root.Execute()
}

// resolveStore validates the --store flag, creates a timeout context, and
// resolves the store in one call. Replaces the repeated requireStore +
// timeoutCtx + Resolve pattern. The caller must defer cancel().
//
// Resolution order: explicit flag > env var > config default.
func (c *CLI) resolveStore(cmd *cobra.Command) (context.Context, context.CancelFunc, shop.Store, error) {
	if c.store == "" {
		c.store = os.Getenv("SHOP_STORE")
	}
	if c.store == "" && c.app != nil && c.app.Config.Defaults.Store != "" {
		c.store = c.app.Config.Defaults.Store
	}
	if c.store == "" {
		return nil, nil, nil, shop.Errorf(shop.ErrInvalidInput,
			"--store is required (set a default: shop config set defaults.store amazon)")
	}

	ctx, cancel := c.timeoutCtx(cmd)

	s, err := c.app.Resolve(ctx, c.store)
	if err != nil {
		cancel()

		return nil, nil, nil, err
	}

	return ctx, cancel, s, nil
}

// timeoutCtx wraps the command's context with the --timeout duration.
func (c *CLI) timeoutCtx(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	return context.WithTimeout(cmd.Context(), c.timeout)
}

// completeStoreNames provides tab completion for the --store flag using
// the store names from the registry.
func (c *CLI) completeStoreNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	if c.app == nil || c.app.Registry == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names := make([]string, len(c.app.Registry.Stores))
	for i, entry := range c.app.Registry.Stores {
		names[i] = entry.Alias
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}
