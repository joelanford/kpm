package cli

import (
	"context"
	"os"

	"github.com/joelanford/kpm/action"
	"github.com/joelanford/kpm/internal/console"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
)

func BuildLegacyRegistryBundle() *cobra.Command {
	var dest destination
	cmd := &cobra.Command{
		Use:   "legacy-registry-bundle <registry-root-dir>",
		Short: "Build a legacy registry bundle",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			rootDir := args[0]

			run := func(ctx context.Context, pushFunc action.PushFunc) (string, ocispec.Descriptor, error) {
				bb := action.BuildLegacyRegistryBundle{
					RootFS:   os.DirFS(rootDir),
					PushFunc: pushFunc,
				}
				console.Secondaryf("⏳  Building legacy registry bundle for %s", rootDir)
				return bb.Run(ctx)
			}

			handleError(dest.push(ctx, run))
		},
	}
	dest.bindSelfRequired(cmd)

	return cmd
}
