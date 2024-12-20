package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2"

	"github.com/joelanford/kpm/internal/kpm"
)

func Push() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push <kpm-file>...",
		Short: "Push one or more kpm files to their origin repositories",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()

			eg, ctx := errgroup.WithContext(ctx)
			eg.SetLimit(4)

			for _, kpmPath := range args {
				eg.Go(func() error {
					kpmFile, err := kpm.Open(ctx, kpmPath)
					if err != nil {
						return fmt.Errorf("failed to open kpm file %q: %v", kpmPath, err)
					}

					if err := kpmFile.Push(ctx, oras.CopyGraphOptions{Concurrency: 3}); err != nil {
						return fmt.Errorf("failed to push kpm file %q: %v", kpmPath, err)
					}
					cmd.Printf("pushed %q to %q (digest: %s)\n", kpmPath, kpmFile.Reference, kpmFile.Descriptor.Digest)
					return nil
				})
			}
			if err := eg.Wait(); err != nil {
				cmd.PrintErr(err)
				os.Exit(1)
			}
		},
	}
	return cmd
}
