package cli

import (
	"fmt"
	buildv1 "github.com/joelanford/kpm/build/v1"
	"github.com/joelanford/kpm/transport"
	"io"
	"os"
	"strings"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/console"
	"github.com/spf13/cobra"
)

func BuildBundle() *cobra.Command {
	var specFile string

	cmd := &cobra.Command{
		Use:   "bundle <bundleDir> <destination>",
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

			target, err := transport.TargetFor(args[1])
			if err != nil {
				handleError(fmt.Errorf("get transport for destination %q: %w", args[1], err))
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
				specReader = strings.NewReader(v1.DefaultRegistryV1Spec)
			}

			log := func(format string, args ...any) {
				console.Secondaryf("   - "+format, args...)
			}
			bb := buildv1.NewBundleBuilder(bundleFS,
				buildv1.WithSpecReader(specReader),
				buildv1.WithLog(log),
			)
			bundle, err := bb.BuildArtifact(ctx)
			if err != nil {
				handleError(fmt.Errorf("failed to build bundle: %w", err))
			}

			tag, desc, err := target.Push(ctx, bundle)
			if err != nil {
				handleError(fmt.Errorf("failed to push bundle: %w", err))
			}
			console.Primaryf("📦 Successfully pushed bundle\n   🏷️%s:%s\n   📍 %s@%s", target.String(), tag, target.String(), desc.Digest.String())
		},
	}
	cmd.Flags().StringVar(&specFile, "spec-file", "", "spec file to use for building the bundle")

	return cmd
}
