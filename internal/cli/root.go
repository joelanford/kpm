package cli

import (
	"github.com/spf13/cobra"
)

func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use: "kpm",
	}
	cmd.AddCommand(
		Build(),
	)
	return cmd
}
