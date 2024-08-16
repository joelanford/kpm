package cli

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	"github.com/joelanford/kpm/action"
	v1 "github.com/joelanford/kpm/api/v1"
	buildv1 "github.com/joelanford/kpm/build/v1"
	"github.com/joelanford/kpm/transport"
	"github.com/spf13/cobra"
)

func BuildCatalog() *cobra.Command {
	var (
		specFile string
		rawFBC   bool
	)

	cmd := &cobra.Command{
		Use:   "catalog <catalogDir> <originRepository>",
		Short: "Build a catalog",
		Long: `Build a catalog

This command builds a catalog based on an optional spec file. If a spec file is not provided
the default FBC spec is used. The spec file can be in JSON or YAML format.

The originRepository argument is a reference to an image repository. The origin reference is
stored in the bundle and is used by other tools to determine the origin of the bundle, for
example, when the kpm file is pushed to a registry or when it needs to be referenced when
building a catalog.
`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()

			catalogDirectory := args[0]
			originRepository := args[1]
			originRepoRef, err := reference.ParseNamed(originRepository)
			if err != nil {
				handleError(fmt.Sprintf("parse origin repository %q: %w", originRepository, err))
			}

			var (
				specReader io.Reader
				catalogFS  fs.FS
			)
			if specFile != "" {
				catalogFS = os.DirFS(catalogDirectory)

				var err error
				specReader, err = os.Open(specFile)
				if err != nil {
					handleError(fmt.Sprintf("open spec file: %w", err))
				}
			} else if rawFBC {
				catalogFS = os.DirFS(catalogDirectory)
				specReader = strings.NewReader(v1.DefaultFBCSpec)
			} else {
				bundles, err := getBundleArtifacts(ctx, catalogDirectory)
				if err != nil {
					handleError(fmt.Sprintf("get bundle artifacts: %w", err))
				}
				gc := action.GenerateCatalog{
					Bundles: bundles,
				}
				catalogFS, err = gc.Run(ctx)
				if err != nil {
					handleError(fmt.Sprintf("get catalog FS: %w", err))
				}
				specReader = strings.NewReader(v1.DefaultFBCSpec)
			}

			cb := buildv1.NewCatalogBuilder(catalogFS,
				buildv1.WithSpecReader(specReader),
			)

			catalog, err := cb.BuildArtifact(ctx)
			if err != nil {
				handleError(fmt.Sprintf("failed to build catalog: %w", err))
			}

			outputFileName := fmt.Sprintf("%s.catalog.kpm", catalog.Tag())
			target := &transport.OCIArchiveTarget{
				Filename:        outputFileName,
				OriginReference: originRepoRef,
			}

			tag, desc, err := target.Push(ctx, catalog)
			if err != nil {
				handleError(fmt.Sprintf("failed to write catalog file: %w", err))
			}
			fmt.Printf("Successfully built catalog %q from %q (tag: %q, digest %q)\n", outputFileName, catalogDirectory, tag, desc.Digest.String())
		},
	}
	cmd.Flags().StringVar(&specFile, "spec-file", "", "spec file to use for building the catalog")
	cmd.Flags().BoolVar(&rawFBC, "raw-fbc", false, "assume the catalog directory is raw FBC and build it directly")

	cmd.MarkFlagsMutuallyExclusive("spec-file", "raw-fbc")
	return cmd
}

func getBundleArtifacts(ctx context.Context, catalogDir string) ([]v1.KPM, error) {
	bundleFiles, err := filepath.Glob(filepath.Join(catalogDir, "*.bundle.kpm"))
	if err != nil {
		return nil, err
	}

	bundles := make([]v1.KPM, 0, len(bundleFiles))
	for _, bundleFile := range bundleFiles {
		bundle, err := v1.LoadKPM(ctx, bundleFile)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, *bundle)
	}

	return bundles, nil
}
