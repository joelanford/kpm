package cli

import (
	"github.com/spf13/cobra"

	experimentalcli "github.com/joelanford/kpm/internal/experimental/cli"
)

func Root(name string) *cobra.Command {
	cmd := &cobra.Command{
		Use: name,
	}
	cmd.AddCommand(
		Build(),
		hidden(experimentalcli.Root("experimental")),
	)
	return cmd
}

func hidden(cmd *cobra.Command) *cobra.Command {
	cmd.Hidden = true
	return cmd
}
