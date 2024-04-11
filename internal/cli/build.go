package cli

import (
	"github.com/spf13/cobra"
)

func Build() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build artifacts",
	}
	cmd.AddCommand(
		BuildBundle(),
		BuildCatalog(),
	)
	return cmd
}
