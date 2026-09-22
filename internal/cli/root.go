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
	text       bool
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
			if err := c.selectOutput(cmd); err != nil {
				return err
			}

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
			if !outputFlagsChanged(cmd) && os.Getenv("SHOP_OUTPUT") == "" {
				c.jsonOutput = cfg.Defaults.Output.JSON
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
	pf.BoolVar(&c.text, "text", false, "human-readable output (not for parsing)")
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

// Execute runs the root command and preserves the structured error exit codes.
func Execute() {
	root := New()
	if err := root.Execute(); err != nil {
		text, _ := root.PersistentFlags().GetBool("text")
		if !outputFlagsChanged(root) {
			text = os.Getenv("SHOP_OUTPUT") == "text"
		}
		code := outputError(err, text)
		os.Exit(code)
	}
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

// outputFlagsChanged includes explicit false values: --text=false opts out of
// the environment default as well as --text=true opting into it.
func outputFlagsChanged(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("text") || cmd.Flags().Changed("json") || cmd.Flags().Changed("pretty")
}

// selectOutput applies flags > environment > saved defaults. Saved defaults are
// loaded separately so errors reading config can still use the requested mode.
func (c *CLI) selectOutput(cmd *cobra.Command) error {
	if outputFlagsChanged(cmd) {
		if c.text && (c.jsonOutput || c.pretty) {
			return shop.Errorf(shop.ErrInvalidInput, "--text cannot be combined with --json or --pretty")
		}

		return nil
	}
	switch os.Getenv("SHOP_OUTPUT") {
	case "":
	case "text":
		c.text = true
	case "json":
		c.jsonOutput = true
	case "pretty":
		c.pretty = true
	default:
		return shop.Errorf(shop.ErrInvalidInput, "SHOP_OUTPUT must be text, json, or pretty")
	}

	return nil
}
