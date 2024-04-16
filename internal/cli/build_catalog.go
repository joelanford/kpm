package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/joelanford/kpm/action"
	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/console"
	"github.com/spf13/cobra"
)

func BuildCatalog() *cobra.Command {
	var specFile string

	cmd := &cobra.Command{
		Use:   "catalog [spec-file]",
		Short: "Build a catalog",
		Long: `Build a catalog

This command builds a catalog based on an optional spec file. If a spec file is not provided
the default FBC spec is used. The spec file can be in JSON or YAML format.

The --destination flag is required and dictates where the bundle is pushed. Options are:
- oci-archive:path/to/local/file.oci.tar
- docker://registry.example.com/namespace/repo

`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()

			catalogDirectory := args[0]
			catalogFS := os.DirFS(catalogDirectory)

			dest, err := newDestination(args[1])
			if err != nil {
				handleError(fmt.Errorf("parse destination: %w", err))
			}
			pushFunc, err := dest.pushFunc()
			if err != nil {
				handleError(fmt.Errorf("get push function from destination: %w", err))
			}

			var specReader io.Reader
			if specFile != "" {
				console.Secondaryf("🏗️Building catalog for spec file %q", specFile)
				var err error
				specReader, err = os.Open(specFile)
				if err != nil {
					handleError(fmt.Errorf("open spec file: %w", err))
				}
			} else {
				console.Secondaryf("🏗️Building FBC catalog for directory %q", catalogDirectory)
				specReader = v1.DefaultFBCSpec
			}

			bb := action.BuildCatalog{
				CatalogFS:      catalogFS,
				SpecFileReader: specReader,
				PushFunc:       pushFunc,
				Log: func(format string, args ...any) {
					console.Secondaryf("   - "+format, args...)
				},
			}
			tag, desc, err := bb.Run(ctx)
			if err != nil {
				handleError(fmt.Errorf("build catalog: %w", err))
			}
			dest.logSuccessFunc()(tag, desc)
		},
	}
	cmd.Flags().StringVar(&specFile, "spec-file", "", "spec file to use for building the catalog")

	return cmd
}
