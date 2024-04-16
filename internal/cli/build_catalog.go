package cli

import (
	"context"
	"fmt"
	"github.com/joelanford/kpm/transport"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/joelanford/kpm/action"
	v1 "github.com/joelanford/kpm/api/v1"
	buildv1 "github.com/joelanford/kpm/build/v1"
	"github.com/joelanford/kpm/internal/console"
	"github.com/joelanford/kpm/oci"
	"github.com/spf13/cobra"
)

func BuildCatalog() *cobra.Command {
	var specFile string
	var fromBundles bool
	var bundleRepo string
	var pushBundles bool

	cmd := &cobra.Command{
		Use:   "catalog <catalogDir> <destination>",
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

			target, err := transport.TargetFor(args[1])
			if err != nil {
				handleError(fmt.Errorf("failed to get transport for reference %q: %w", args[1], err))
			}

			log := func(format string, args ...any) {
				console.Secondaryf("   - "+format, args...)
			}

			var specReader io.Reader
			if specFile != "" {
				console.Secondaryf("🏗️Building catalog for spec file %q", specFile)
				var err error
				specReader, err = os.Open(specFile)
				if err != nil {
					handleError(fmt.Errorf("open spec file: %w", err))
				}
			} else if fromBundles {
				console.Secondaryf("🏗️Building semver catalog from bundle directories rooted in %q", catalogDirectory)
				bundleArtifacts, err := getBundleArtifacts(ctx, os.DirFS(catalogDirectory))
				if err != nil {
					handleError(fmt.Errorf("get bundle artifacts: %w", err))
				}
				log("found %d bundle artifacts", len(bundleArtifacts))
				gc := action.GenerateCatalog{
					BundleRepository: bundleRepo,
					Bundles:          bundleArtifacts,
					PushBundles:      pushBundles,
					Log:              log,
				}
				catalogFS, err = gc.Run(ctx)
				if err != nil {
					handleError(fmt.Errorf("get catalog FS: %w", err))
				}
				specReader = strings.NewReader(v1.DefaultFBCSpec)
			} else {
				console.Secondaryf("🏗️Building FBC catalog for directory %q", catalogDirectory)
				specReader = strings.NewReader(v1.DefaultFBCSpec)
			}

			cb := buildv1.NewCatalogBuilder(catalogFS,
				buildv1.WithSpecReader(specReader),
				buildv1.WithLog(log),
			)

			log("building catalog")
			catalog, err := cb.BuildArtifact(ctx)
			if err != nil {
				handleError(fmt.Errorf("failed to build catalog: %w", err))
			}

			log("pushing catalog")
			tag, desc, err := target.Push(ctx, catalog)
			if err != nil {
				handleError(fmt.Errorf("failed to push catalog: %w", err))
			}
			console.Primaryf("📦 Successfully pushed catalog\n   🏷️%s:%s\n   📍 %s@%s", target.String(), tag, target.String(), desc.Digest.String())
		},
	}
	cmd.Flags().StringVar(&specFile, "spec-file", "", "spec file to use for building the catalog")
	cmd.Flags().BoolVar(&fromBundles, "from-bundles", false, "build a semver catalog using bundle directories found in the catalog directory")
	cmd.Flags().StringVar(&bundleRepo, "bundle-repo", "", "repository to use for bundle image references in the catalog")
	cmd.Flags().BoolVar(&pushBundles, "push-bundles", false, "push bundles to --bundle-repo")

	cmd.MarkFlagsMutuallyExclusive("spec-file", "from-bundles")
	cmd.MarkFlagsMutuallyExclusive("spec-file", "push-bundles")
	cmd.MarkFlagsRequiredTogether("from-bundles", "bundle-repo")

	return cmd
}

func getBundleArtifacts(ctx context.Context, catalogFS fs.FS) ([]oci.Artifact, error) {
	var artifacts []oci.Artifact

	if err := fs.WalkDir(catalogFS, ".", func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() || path == "." {
			return nil
		}
		pathFS, err := fs.Sub(catalogFS, path)
		if err != nil {
			return err
		}
		if !isBundleRoot(pathFS) {
			return nil
		}
		bundleArtifact, err := buildv1.NewBundleBuilder(pathFS).BuildArtifact(ctx)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, bundleArtifact)
		return fs.SkipDir
	}); err != nil {
		return nil, err
	}

	return artifacts, nil
}

func isBundleRoot(fsys fs.FS) bool {
	manifestsInfo, err := fs.Stat(fsys, "manifests")
	if err != nil || !manifestsInfo.IsDir() {
		return false
	}
	metadataInfo, err := fs.Stat(fsys, "metadata")
	if err != nil || !metadataInfo.IsDir() {
		return false
	}
	return true
}
