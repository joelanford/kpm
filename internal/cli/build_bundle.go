package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	v1 "github.com/joelanford/kpm/api/v1"
	buildv1 "github.com/joelanford/kpm/build/v1"
	"github.com/joelanford/kpm/transport"
	"github.com/spf13/cobra"
)

func BuildBundle() *cobra.Command {
	var (
		specFile string
	)

	cmd := &cobra.Command{
		Use:   "bundle <bundleDir> <originRepository>",
		Short: "Build a bundle",
		Long: `Build a bundle

This command builds a bundle based on an optional spec file. If a spec file is not provided
the default registry+v1 spec is used. The spec file can be in JSON or YAML format.

The originRepository argument is a reference to an image repository. The origin reference is
stored in the bundle and is used by other tools to determine the origin of the bundle, for
example, when the kpm file is pushed to a registry or when it needs to be referenced when
building a catalog.
`,

		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()

			bundleDirectory := args[0]
			bundleFS := os.DirFS(bundleDirectory)

			originRepository := args[1]
			originRepoRef, err := reference.ParseNamed(originRepository)
			if err != nil {
				handleError(fmt.Sprintf("Failed to build bundle from directory %q: parse origin repository %q: %w", originRepository, err))
			}

			var specReader io.Reader
			if specFile != "" {
				var err error
				specReader, err = os.Open(specFile)
				if err != nil {
					handleError(fmt.Sprintf("Failed to build bundle from directory %q: open spec file: %w", bundleDirectory, err))
				}
			} else {
				specReader = strings.NewReader(v1.DefaultRegistryV1Spec)
			}

			bb := buildv1.NewBundleBuilder(bundleFS,
				buildv1.WithSpecReader(specReader),
			)
			bundle, err := bb.BuildArtifact(ctx)
			if err != nil {
				handleError(fmt.Sprintf("Failed to build bundle from directory %q: %w", bundleDirectory, err))
			}

			outputFileName := fmt.Sprintf("%s.bundle.kpm", bundle.Tag())
			target := &transport.OCIArchiveTarget{
				Filename:        outputFileName,
				OriginReference: originRepoRef,
			}

			tag, desc, err := target.Push(ctx, bundle)
			if err != nil {
				handleError(fmt.Sprintf("Failed to build bundle %q from %q: failed to write bundle file: %w", outputFileName, bundleDirectory, err))
			}

			fmt.Printf("Successfully built bundle %q from %q (tag: %q, digest %q)\n", outputFileName, bundleDirectory, tag, desc.Digest.String())
		},
	}
	cmd.Flags().StringVar(&specFile, "spec-file", "", "spec file to use for building the bundle")

	return cmd
}
