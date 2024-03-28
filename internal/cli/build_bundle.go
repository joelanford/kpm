package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/joelanford/kpm/action"
	v1 "github.com/joelanford/kpm/api/v1"
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
		Use:   "bundle [spec-file]",
		Short: "Build a bundle",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()

			run := func(ctx context.Context, pushFunc action.PushFunc) (string, ocispec.Descriptor, error) {
				var (
					specReader io.Reader
					message    string
				)
				if len(args) == 1 {
					var err error
					specReader, err = os.Open(args[0])
					if err != nil {
						return "", ocispec.Descriptor{}, err
					}
					message = fmt.Sprintf("Building bundle for spec file %q", args[0])
				} else {
					specReader = v1.DefaultRegistryV1Spec
					message = fmt.Sprintf("Building registry+v1 bundle from directory %q", workingDirectory)
				}

				bb := action.BuildBundle{
					SpecFileWorkingFS: os.DirFS(workingDirectory),
					SpecFileReader:    specReader,
					PushFunc:          pushFunc,
				}
				console.Secondaryf("⏳  %s", message)
				return bb.Run(ctx)
			}

			handleError(dest.push(ctx, run))
		},
	}
	dest.bindSelfRequired(cmd)
	cmd.Flags().StringVar(&workingDirectory, "working-dir", ".", "working directory used to resolve relative paths in the spec file for bundle contents")

	return cmd
}
