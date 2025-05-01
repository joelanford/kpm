package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"

	"github.com/joelanford/kpm/internal/experimental/catalog"
)

func BuildCatalog() *cobra.Command {
	var (
		outputDirectory string
		reportFile      string
	)
	cmd := &cobra.Command{
		Use:   "catalog <catalogSpecFile>",
		Short: "Build a catalog",
		Long:  longDescription,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			specFileName := args[0]
			res, err := catalog.BuildFromSpecFile(ctx, specFileName, outputDirectory)
			if err != nil {
				cmd.PrintErrf("failed to build catalog: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Catalog written to %s with tag %q (digest: %s)\n", res.FilePath, fmt.Sprintf("%s:%s", res.Repository, res.Tag), res.Descriptor.Digest)

			if reportFile != "" {
				f, err := os.Create(reportFile)
				if err != nil {
					cmd.PrintErrf("failed to create report file: %v\n", err)
					os.Exit(1)
				}
				defer f.Close()

				enc := json.NewEncoder(f)
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				if err := enc.Encode(res); err != nil {
					cmd.PrintErrf("failed to write report for result to %s: %v", reportFile, errors.Join(err, os.Remove(reportFile)))
					os.Exit(1)
				}
			}
		},
	}
	cmd.Flags().StringVarP(&outputDirectory, "output", "o", "",
		"Output directory (default: current working directory)")
	cmd.Flags().StringVar(&reportFile, "report-file", "", "Optionally, a file in which to write a JSON report of the build result.")
	return cmd
}

const longDescription = `Build a kpm catalog from a spec file or a directory of bundles.

Usage: kpm build catalog <catalogSpecFile>


SPEC APIS

catalogs.specs.kpm.io/v1

  Currently, the only supported spec API is v1. This API enables building an OLM
  catalog from a directory of bundles, an FBC directory, or an FBC template.

  It includes fields to configure a tag reference, the migration level, the cache
  format, and extra annotations to use when building the catalog.

  Supported migration levels:
    - none [default]
    - bundle-object-to-csv-metadata
    - all (currently the same as bundle-object-to-csv-metadata)

  Supported cache formats:
    - json [default]
    - pogreb.v1
    - none (to exclude the cache from the catalog)

    NOTE: the pogreb.v1 cache format is not deterministic. Rebuilding a catalog with
    the same content will always produce a different cache layer.

  SOURCE TYPES

    bundles

    The simplest way to build a catalog is to provide a directory of bundles. In this
    mode, kpm will automatically generate a catalog from the bundles in the specified
    directory where newer versions of a bundle can upgrade from all previous versions.

    Most users should use this source type to build catalogs.

    To use this source type, create a spec file like this:

      apiVersion: specs.kpm.io/v1
      kind: Catalog
      tag: "quay.io/myorg/my-catalog:latest"
      cacheFormat: none
      source:
        sourceType: bundles
        bundles:
          bundleRoot: ./path/to/dir/containing/kpm/bundles/

    fbc

    The FBC format is a directory of declarative config files. To build a catalog from
    an existing FBC directory, use a spec file like this:

      apiVersion: specs.kpm.io/v1
      kind: Catalog
      tag: "quay.io/myorg/my-catalog:latest"
      migrationLevel: bundle-object-to-csv-metadata
      cacheFormat: json
      source:
        sourceType: fbc
        fbc:
          catalogRoot: ./path/to/fbc/root/

    fbcTemplate

    The FBC template format is a single file that contains a template specification
    for generating FBC. kpm supports OLM's semver and basic templates. To build a
    catalog from an FBC template, use a spec file like this:

      apiVersion: specs.kpm.io/v1
      kind: Catalog
      tag: "quay.io/myorg/my-catalog:latest"
      migrationLevel: all
      cacheFormat: pogreb.v1
      source:
        sourceType: fbcTemplate
        fbcTemplate:
          templateFile: semver.yaml

    legacy (deprecated)

    !!! No new packages should begin using this format !!!

    The legacy format uses the deprecated sqlite-based catalog building techniques that
    are based on each bundle's upgrade graph and channel membership metadata. This format
    is highly discouraged because it does not support commonly used features like
    retroactive changes to channel membership and channel-specific upgrade edges.

    To use this source type, create a spec file like this:

      apiVersion: specs.kpm.io/v1
      kind: Catalog
      imageReference: quay.io/myorg/my-catalog:latest

      cacheFormat: none
      source:
        sourceType: legacy
        legacy:
          bundleRoot: ./path/to/legacy/packagemanifests/root
          bundleRegistryNamespace: quay.io/myorg
`
