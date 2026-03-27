package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/config"
)

// configurable keys and their descriptions. Add new keys here to make them
// available via `shop config set/get`.
var configKeys = map[string]string{
	"defaults.store":         "default store for all commands (avoids -s flag)",
	"defaults.timeout":       "default request timeout (e.g. 30s, 1m)",
	"defaults.output.json":   "always output compact JSON (true/false)",
	"defaults.output.pretty": "always pretty-print JSON (true/false)",
}

func (c *CLI) newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration",
	}

	cmd.AddCommand(
		c.newConfigSetCmd(),
		c.newConfigGetCmd(),
		c.newConfigListCmd(),
	)

	return cmd
}

func (c *CLI) newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: `Set a configuration value. Available keys:
  defaults.store           Default store for all commands
  defaults.timeout         Default request timeout (e.g. 30s, 1m)
  defaults.output.json     Always output compact JSON (true/false)
  defaults.output.pretty   Always pretty-print JSON (true/false)`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			key, value := args[0], args[1]

			if _, ok := configKeys[key]; !ok {
				return shop.Errorf(shop.ErrInvalidInput, "unknown config key %q (see: shop config list)", key)
			}

			cfg := c.app.Config

			switch key {
			case "defaults.store":
				cfg.Defaults.Store = value
			case "defaults.timeout":
				cfg.Defaults.Timeout = value
			case "defaults.output.json":
				cfg.Defaults.Output.JSON = value == "true"
			case "defaults.output.pretty":
				cfg.Defaults.Output.Pretty = value == "true"
			}

			if err := config.Save(c.app.ConfigDir, cfg); err != nil {
				return shop.Errorf(shop.ErrConfigError, "save config: %v", err)
			}

			return c.outputJSON(map[string]string{"key": key, "value": value})
		},
	}
}

func (c *CLI) newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			key := args[0]

			if _, ok := configKeys[key]; !ok {
				return shop.Errorf(shop.ErrInvalidInput, "unknown config key %q (see: shop config list)", key)
			}

			cfg := c.app.Config
			var value string

			switch key {
			case "defaults.store":
				value = cfg.Defaults.Store
			case "defaults.timeout":
				value = cfg.Defaults.Timeout
			case "defaults.output.json":
				value = fmt.Sprintf("%t", cfg.Defaults.Output.JSON)
			case "defaults.output.pretty":
				value = fmt.Sprintf("%t", cfg.Defaults.Output.Pretty)
			}

			if value == "" {
				value = "(not set)"
			}

			return c.outputJSON(map[string]string{"key": key, "value": value})
		},
	}
}

func (c *CLI) newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configuration values",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg := c.app.Config

			// Build a flat key-value map of current config state.
			values := map[string]string{
				"defaults.store":         cfg.Defaults.Store,
				"defaults.timeout":       cfg.Defaults.Timeout,
				"defaults.output.json":   fmt.Sprintf("%t", cfg.Defaults.Output.JSON),
				"defaults.output.pretty": fmt.Sprintf("%t", cfg.Defaults.Output.Pretty),
			}

			dir := c.configPath
			if dir == "" {
				dir = config.DefaultDir()
			}
			_, _ = fmt.Fprintf(os.Stderr, "config: %s/config.json\n", dir)

			return c.outputJSON(values)
		},
	}
}
