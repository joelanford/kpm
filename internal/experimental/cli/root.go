package cli

import (
	"github.com/spf13/cobra"
)

func Root(name string) *cobra.Command {
	cmd := &cobra.Command{
		Use: name,
	}
	cmd.AddCommand(
		Build(),
		Extract(),
		Push(),
		Render(),
	)
	return cmd
}
