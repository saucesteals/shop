package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/saucesteals/shop"
)

func (c *CLI) newSkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the SKILL.md for use with AI agents (OpenClaw, Claude, etc.)",
		Long: `Print the embedded SKILL.md to stdout.

Pipe it into your AI agent's skill directory to enable autonomous shopping:

  # OpenClaw
  mkdir -p ~/.openclaw/workspace/skills/shop
  shop skill > ~/.openclaw/workspace/skills/shop/SKILL.md

  # Claude Code
  shop skill > .claude/shop.md`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprint(cmd.OutOrStdout(), shop.SkillMD)
			return nil
		},
	}
}
