package cli

import (
	"os"

	"github.com/joelanford/kpm/internal/kpm"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
)

func Push() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push <kpm-file>...",
		Short: "Push one or more kpm files to their origin repositories",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()

			for _, kpmPath := range args {
				kpmFile, err := kpm.Open(ctx, kpmPath)
				if err != nil {
					cmd.PrintErrf("failed to open kpm file %s: %v\n", kpmPath, err)
					os.Exit(1)
				}

				if err := kpmFile.Push(ctx, oras.CopyGraphOptions{Concurrency: 3}); err != nil {
					cmd.PrintErrf("failed to push kpm file %s: %v\n", kpmPath, err)
					os.Exit(1)
				}
				cmd.Printf("pushed %q to %q (digest: %s)\n", kpmPath, kpmFile.Reference, kpmFile.Descriptor.Digest)
			}
		},
	}
	return cmd
}
