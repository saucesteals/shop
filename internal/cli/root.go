// Package cli defines the cobra command tree for the shop CLI.
package cli

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/config"
)

// Version is set at build time via ldflags. Defaults to "dev" for
// development builds.
var Version = "dev"

// CLI holds shared state across all commands.
type CLI struct {
	client *shop.Client
	config *config.Config

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
			cfg, err := config.Load(dir)
			if err != nil {
				return shop.Errorf(shop.ErrConfigError, "load config: %v", err)
			}
			c.config = cfg
			c.configPath = dir
			if !cmd.Flags().Changed("json") {
				c.jsonOutput = cfg.Defaults.Output.JSON
			}
			if !cmd.Flags().Changed("pretty") {
				c.pretty = cfg.Defaults.Output.Pretty
			}

			// Config commands must remain usable to repair a bad timeout setting.
			if cmd.Parent() != nil && cmd.Parent().Name() == "config" {
				return nil
			}
			if !cmd.Flags().Changed("timeout") && cfg.Defaults.Timeout != "" {
				c.timeout, err = time.ParseDuration(cfg.Defaults.Timeout)
				if err != nil || c.timeout < 0 {
					return shop.Errorf(shop.ErrConfigError, "invalid configured timeout")
				}
			}
			if c.timeout < 0 {
				return shop.Errorf(shop.ErrInvalidInput, "timeout must not be negative")
			}
			c.client, err = shop.New(shop.Options{
				ConfigDir: dir,
				// Context deadlines are applied by the CLI, once per operation.
				HTTPClient: &http.Client{},
			})
			if err != nil {
				return err
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
		c.newTrackCmd(),
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
// resolves the store in one call. The caller must defer cancel().
//
// Resolution order: explicit flag > env var > config default.
func (c *CLI) resolveStore(cmd *cobra.Command) (context.Context, context.CancelFunc, shop.Store, error) {
	if c.store == "" {
		c.store = os.Getenv("SHOP_STORE")
	}
	if c.store == "" && c.config != nil && c.config.Defaults.Store != "" {
		c.store = c.config.Defaults.Store
	}
	if c.store == "" {
		return nil, nil, nil, shop.Errorf(shop.ErrInvalidInput,
			"--store is required (set a default: shop config set defaults.store amazon)")
	}

	ctx, cancel := c.timeoutCtx(cmd)

	s, err := c.client.Store(ctx, c.store)
	if err != nil {
		cancel()

		return nil, nil, nil, err
	}

	return ctx, cancel, s, nil
}

// timeoutCtx wraps the command's context with the --timeout duration.
func (c *CLI) timeoutCtx(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	if c.timeout == 0 {
		return context.WithCancel(cmd.Context())
	}

	return context.WithTimeout(cmd.Context(), c.timeout)
}

// completeStoreNames provides tab completion for the --store flag using
// the store names from the registry.
func (c *CLI) completeStoreNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	dir := c.configPath
	if dir == "" {
		dir = config.DefaultDir()
	}
	registry, err := config.LoadRegistry(dir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make([]string, len(registry.Stores))
	for i, entry := range registry.Stores {
		names[i] = entry.Alias
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}
