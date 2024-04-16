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

func BuildBundle() *cobra.Command {
	var specFile string

	cmd := &cobra.Command{
		Use:   "bundle <bundle-directory> <destination>",
		Short: "Build a bundle",
		Long: `Build a bundle

This command builds a bundle based on an optional spec file. If a spec file is not provided
the default registry+v1 spec is used. The spec file can be in JSON or YAML format.

The destination argument dictates where the bundle is pushed. Options are:
- oci-archive:path/to/local/file.oci.tar
- docker://registry.example.com/namespace/repo

`,

		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()

			bundleDirectory := args[0]
			bundleFS := os.DirFS(bundleDirectory)

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
				console.Secondaryf("🏗️Building bundle for spec file %q", specFile)
				var err error
				specReader, err = os.Open(specFile)
				if err != nil {
					handleError(fmt.Errorf("open spec file: %w", err))
				}
			} else {
				console.Secondaryf("🏗️Building registry+v1 bundle for directory %q", bundleDirectory)
				specReader = v1.DefaultRegistryV1Spec
			}

			bb := action.BuildBundle{
				BundleFS:       bundleFS,
				SpecFileReader: specReader,
				PushFunc:       pushFunc,
				Log: func(format string, args ...any) {
					console.Secondaryf("   - "+format, args...)
				},
			}
			tag, desc, err := bb.Run(ctx)
			if err != nil {
				handleError(fmt.Errorf("build bundle: %w", err))
			}
			dest.logSuccessFunc()(tag, desc)
		},
	}
	cmd.Flags().StringVar(&specFile, "spec-file", "", "spec file to use for building the bundle")

	return cmd
}
