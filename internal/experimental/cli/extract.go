package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/joelanford/kpm/internal/experimental/action"
)

func Extract() *cobra.Command {
	var (
		outputDir string
	)
	cmd := &cobra.Command{
		Use:   "extract <kpm-file>",
		Short: "Extract the contents of a KPM file",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			kpmFilePath := args[0]
			if err := action.Extract(ctx, args[0], outputDir); err != nil {
				cmd.PrintErrf("failed to extract %q: %v\n", kpmFilePath, err)
				os.Exit(1)
			}
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output-directory", "o", "", "output directory in which to extract KPM file")
	return cmd
}
