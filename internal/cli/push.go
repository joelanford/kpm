package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2"

	"github.com/joelanford/kpm/internal/pkg/kpm"
)

func Push() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push <kpm-file>... <remoteNamespace>",
		Short: "Push one or more kpm files to a remote repository",
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()

			kpmFiles := args[0 : len(args)-1]
			remoteNamespace := args[len(args)-1]

			eg, ctx := errgroup.WithContext(ctx)
			eg.SetLimit(4)

			for _, kpmPath := range kpmFiles {
				eg.Go(func() error {
					bundle, err := kpm.OpenBundle(ctx, kpmPath)
					if err != nil {
						return fmt.Errorf("failed to open kpm file %q: %v", kpmPath, err)
					}

					ref, err := bundle.Push(ctx, remoteNamespace, oras.CopyGraphOptions{Concurrency: 3})
					if err != nil {
						return fmt.Errorf("failed to push kpm file %q: %v", kpmPath, err)
					}
					cmd.Printf("pushed %q to %q (digest: %s)\n", kpmPath, ref, bundle.Descriptor().Digest)
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
