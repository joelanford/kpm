package cli

import (
	"context"
	"os"

	"github.com/joelanford/kpm/action"
	"github.com/joelanford/kpm/internal/console"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
)

func BuildBundle() *cobra.Command {
	var (
		dest             destination
		workingDirectory string
	)
	cmd := &cobra.Command{
		Use:   "bundle <spec-file>",
		Short: "Build a bundle",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			specFile := args[0]

			run := func(ctx context.Context, pushFunc action.PushFunc) (string, ocispec.Descriptor, error) {
				specReader, err := os.Open(specFile)
				if err != nil {
					return "", ocispec.Descriptor{}, err
				}

				bb := action.BuildBundle{
					SpecFileWorkingFS: os.DirFS(workingDirectory),
					SpecFileReader:    specReader,
					PushFunc:          pushFunc,
				}
				console.Secondaryf("⏳  Building bundle for %s", specFile)
				return bb.Run(ctx)
			}

			handleError(dest.push(ctx, run))
		},
	}
	dest.bindSelfRequired(cmd)
	cmd.Flags().StringVar(&workingDirectory, "working-dir", ".", "working directory used to resolve relative paths in the spec file for bundle contents")

	return cmd
}
