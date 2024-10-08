package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/containers/image/v5/docker/reference"
	"github.com/joelanford/kpm/internal/action"
	specsv1 "github.com/joelanford/kpm/internal/api/specs/v1"
	"github.com/joelanford/kpm/internal/fsutil"
	"github.com/joelanford/kpm/internal/kpm"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
	"sigs.k8s.io/yaml"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"

	stdaction "github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/action/migrations"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/operator-framework/operator-registry/alpha/template/basic"
	semvertemplate "github.com/operator-framework/operator-registry/alpha/template/semver"
	"github.com/operator-framework/operator-registry/pkg/cache"
	"github.com/operator-framework/operator-registry/pkg/containertools"
)

func BuildCatalog() *cobra.Command {
	var (
		outputFile string
	)
	cmd := &cobra.Command{
		Use:   "catalog <catalogSpecFile> | <bundleRoot> <tagRef>",
		Short: "Build a catalog",
		Long: `Build a kpm catalog from a spec file or a directory of bundles.

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
`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			specFileName := args[0]
			if err := buildCatalogFromSpec(ctx, specFileName, outputFile); err != nil {
				cmd.PrintErrf("failed to build catalog: %v\n", err)
				os.Exit(1)
			}
		},
	}
	cmd.Flags().StringVarP(&outputFile, "output", "o", "",
		"Output file (default: <repoBaseName>-<tag>.kpm)")
	return cmd
}

// buildCatalogFromSpec reads a spec file, builds a kpm catalog from the spec, and writes it to an output file.
//
// TODO: Move this logic outside the CLI package to make it easier to test and more reusable.
func buildCatalogFromSpec(ctx context.Context, specFileName, outputFile string) error {
	spec, err := readCatalogSpec(specFileName)
	if err != nil {
		return fmt.Errorf("failed to read spec file: %w", err)
	}

	tagRef, err := getCatalogRef(spec.Tag)
	if err != nil {
		return fmt.Errorf("failed to get tagged reference from spec file: %w", err)
	}

	if spec.MigrationLevel == "" {
		spec.MigrationLevel = migrations.NoMigrations
	}
	m, err := migrations.NewMigrations(spec.MigrationLevel)
	if err != nil {
		return fmt.Errorf("failed to create migrations: %w", err)
	}

	rootDir := filepath.Dir(specFileName)

	var fbc *declcfg.DeclarativeConfig
	switch spec.Source.SourceType {
	case specsv1.CatalogSpecSourceTypeBundles:
		fbc, err = renderBundlesDir(ctx, filepath.Join(rootDir, spec.Source.Bundles.BundleRoot))
	case specsv1.CatalogSpecSourceTypeFBC:
		fbc, err = declcfg.LoadFS(ctx, os.DirFS(filepath.Join(rootDir, spec.Source.FBC.CatalogRoot)))
	case specsv1.CatalogSpecSourceTypeFBCTemplate:
		templateSchema, templateData, err := getTemplateData(rootDir, spec.Source.FBCTemplate.TemplateFile)
		if err != nil {
			return fmt.Errorf("failed to get template data: %v", err)
		}
		fbc, err = renderTemplate(ctx, templateSchema, templateData)
	default:
		return fmt.Errorf("unsupported source type %q", spec.Source.SourceType)
	}
	if err != nil {
		return fmt.Errorf("failed to load or generate FBC: %w", err)
	}

	if err := m.Migrate(fbc); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	tmpFBCDir, err := os.MkdirTemp("", "kpm-build-catalog-fbc-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary fbc dir: %w", err)
	}
	defer os.RemoveAll(tmpFBCDir)
	if err := declcfg.WriteFS(*fbc, tmpFBCDir, declcfg.WriteYAML, ".yaml"); err != nil {
		return fmt.Errorf("failed to write FBC: %w", err)
	}
	fbcFsys, err := fsutil.Prefix(os.DirFS(tmpFBCDir), "/catalog")
	if err != nil {
		return fmt.Errorf("failed to create FBC filesystem: %v", err)
	}

	if spec.CacheFormat == "" {
		spec.CacheFormat = "json"
	}
	switch spec.CacheFormat {
	case "json", "pogreb.v1":
		break // all other cases return within the switch
	case "none":
		return writeCatalog(fbcFsys, nil, tagRef, outputFile)
	default:
		return fmt.Errorf("unsupported cache format %q", spec.CacheFormat)
	}

	tmpCacheDir, err := os.MkdirTemp("", "kpm-build-catalog-cache-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary cache dir: %w", err)
	}
	defer os.RemoveAll(tmpCacheDir)

	c, err := cache.New(tmpCacheDir, cache.WithBackendName(spec.CacheFormat))
	if err != nil {
		return fmt.Errorf("failed to create cache: %w", err)
	}
	if err := c.Build(ctx, fbcFsys); err != nil {
		return fmt.Errorf("failed to build cache: %w", err)
	}
	if err := c.Close(); err != nil {
		return fmt.Errorf("failed to close cache: %w", err)
	}
	cacheFsys, err := fsutil.Prefix(os.DirFS(tmpCacheDir), "/tmp/cache")
	if err != nil {
		return fmt.Errorf("failed to create cache fs: %v", err)
	}

	return writeCatalog(fbcFsys, cacheFsys, tagRef, outputFile)
}

func writeCatalog(fbcFsys fs.FS, cacheFsys fs.FS, tagRef reference.NamedTagged, outputFile string) error {
	annotations := map[string]string{
		containertools.ConfigsLocationLabel: "/configs",
	}

	configsFsys, err := fsutil.Prefix(fbcFsys, "/configs")
	if err != nil {
		return fmt.Errorf("failed to create configs fs: %v", err)
	}
	layers := []fs.FS{configsFsys}
	if cacheFsys != nil {
		layers = append(layers, cacheFsys)
	}

	// Open output file for writing
	if outputFile == "" {
		pathParts := strings.Split(reference.Path(reference.TrimNamed(tagRef)), "/")
		baseName := pathParts[len(pathParts)-1]
		outputFile = fmt.Sprintf("%s-%s.catalog.kpm", baseName, tagRef.Tag())
	}
	kpmFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}

	// Write it!
	desc, err := kpm.WriteImageManifest(kpmFile, tagRef, layers, annotations)
	if err != nil {
		// Clean up the file if we failed to write it
		_ = os.Remove(outputFile)
		return fmt.Errorf("failed to write kpm file: %v", err)
	}

	fmt.Printf("Catalog written to %s with tag %q (digest: %s)\n", outputFile, tagRef, desc.Digest)
	return nil
}

func readCatalogSpec(specFile string) (*specsv1.Catalog, error) {
	specData, err := os.ReadFile(specFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file: %w", err)
	}

	var spec specsv1.Catalog
	if err := yaml.Unmarshal(specData, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse catalog spec: %w", err)
	}
	expectedGVK := specsv1.GroupVersion.WithKind(specsv1.KindCatalog)
	if spec.GroupVersionKind() != expectedGVK {
		return nil, fmt.Errorf("unsupported spec API found: %s, expected %s", spec.GroupVersionKind(), expectedGVK)
	}
	return &spec, nil
}

func getCatalogRef(refStr string) (reference.NamedTagged, error) {
	ref, err := reference.ParseNamed(refStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tag %q from spec file: %v", refStr, err)
	}
	tagRef, ok := ref.(reference.NamedTagged)
	if !ok {
		return reference.WithTag(ref, "latest")
	}
	return tagRef, nil
}

// renderBundlesDir reads bundle files recursively from a directory, generates a catalog, and writes it to an output file.
//
// TODO: Move this logic outside the CLI package to make it easier to test and more reusable.
func renderBundlesDir(ctx context.Context, bundleRoot string) (*declcfg.DeclarativeConfig, error) {
	type bundleMeta struct {
		name    string
		version semver.Version
	}
	type packageMeta struct {
		maxVersion  semver.Version
		icon        *declcfg.Icon
		description string
		bundles     []bundleMeta
	}

	parseVersion := func(b *declcfg.Bundle) (semver.Version, error) {
		for _, p := range b.Properties {
			if p.Type != property.TypePackage {
				continue
			}
			var pkg property.Package
			if err := json.Unmarshal(p.Value, &pkg); err != nil {
				return semver.Version{}, err
			}
			return semver.Parse(pkg.Version)
		}
		return semver.Version{}, fmt.Errorf("no package property found")
	}

	fbc := &declcfg.DeclarativeConfig{}

	// operator-registry's bundle parsing library logs unnecessary warnings, so we disable it.
	logrus.SetOutput(io.Discard)

	packageMetas := map[string]packageMeta{}
	if err := filepath.WalkDir(bundleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
		}
		if d.IsDir() {
			return nil
		}

		// Skip non-bundle files
		if !strings.HasSuffix(path, ".bundle.kpm") {
			return nil
		}

		r := action.Render{
			AllowedRefMask: stdaction.RefBundleDir,
		}
		bundleFBC, err := r.Render(ctx, path)
		if err != nil {
			return fmt.Errorf("failed to render bundle %q: %w", path, err)
		}

		bundle := bundleFBC.Bundles[0]
		fbc.Bundles = append(fbc.Bundles, bundle)

		vers, err := parseVersion(&bundle)
		if err != nil {
			return fmt.Errorf("failed to parse bundle version: %w", err)
		}

		pm, ok := packageMetas[bundle.Package]
		if !ok {
			pm = packageMeta{
				icon:    nil,
				bundles: []bundleMeta{},
			}
		}
		pm.bundles = append(pm.bundles, bundleMeta{
			name:    fbc.Bundles[0].Name,
			version: vers,
		})
		if vers.GT(pm.maxVersion) {
			var csv v1alpha1.ClusterServiceVersion
			if err := json.Unmarshal([]byte(fbc.Bundles[0].CsvJSON), &csv); err != nil {
				return fmt.Errorf("failed to parse CSV for bundle %q: %w", path, err)
			}

			var icon *declcfg.Icon
			if len(csv.Spec.Icon) > 0 {
				iconData, err := base64.StdEncoding.DecodeString(csv.Spec.Icon[0].Data)
				if err != nil {
					return fmt.Errorf("failed to decode icon data for bundle %q: %w", path, err)
				}
				icon = &declcfg.Icon{
					Data:      iconData,
					MediaType: csv.Spec.Icon[0].MediaType,
				}
			}

			pm.maxVersion = vers
			pm.icon = icon
			pm.description = csv.Spec.Description
		}
		packageMetas[bundle.Package] = pm
		return nil
	}); err != nil {
		return nil, err
	}

	pkgNames := maps.Keys(packageMetas)
	slices.Sort(pkgNames)

	for _, pkgName := range pkgNames {
		pm := packageMetas[pkgName]
		slices.SortFunc(pm.bundles, func(i, j bundleMeta) int {
			return j.version.Compare(i.version)
		})
		fbcPkg := declcfg.Package{
			Schema:         declcfg.SchemaPackage,
			Name:           pkgName,
			DefaultChannel: "default",
			Icon:           pm.icon,
			Description:    pm.description,
		}
		fbcCh := declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Package: pkgName,
			Name:    "default",
			Entries: make([]declcfg.ChannelEntry, len(pm.bundles)),
		}
		for i, meta := range pm.bundles {
			entry := declcfg.ChannelEntry{
				Name:      meta.name,
				SkipRange: fmt.Sprintf("<%s", meta.version.String()),
			}
			if i < len(pm.bundles)-1 {
				entry.Replaces = pm.bundles[i+1].name
			}
			fbcCh.Entries[i] = entry
		}
		fbc.Packages = append(fbc.Packages, fbcPkg)
		fbc.Channels = append(fbc.Channels, fbcCh)
	}

	return fbc, nil
}

func getTemplateData(rootDir, templateFile string) (string, []byte, error) {
	templateData, err := os.ReadFile(filepath.Join(rootDir, templateFile))
	if err != nil {
		return "", nil, fmt.Errorf("failed to read template file: %v", err)
	}

	var (
		schema    string
		metaCount int
	)
	if err := declcfg.WalkMetasReader(bytes.NewReader(templateData), func(m *declcfg.Meta, err error) error {
		if err != nil {
			return err
		}
		metaCount++
		if metaCount > 1 {
			return fmt.Errorf("template file contains more than one template spec")
		}
		schema = m.Schema
		return nil
	}); err != nil {
		return "", nil, fmt.Errorf("failed to read meta from template file: %w", err)
	}
	return schema, templateData, nil
}

func renderTemplate(ctx context.Context, templateSchema string, templateData []byte) (*declcfg.DeclarativeConfig, error) {
	renderBundle := func(ctx context.Context, ref string) (*declcfg.DeclarativeConfig, error) {
		r := action.Render{}
		return r.Render(ctx, ref)
	}

	logrus.SetOutput(io.Discard)

	switch templateSchema {
	case "olm.template.basic":
		basicTemplate := basic.Template{
			RenderBundle: renderBundle,
		}
		return basicTemplate.Render(ctx, bytes.NewReader(templateData))
	case "olm.semver":
		semverTemplate := semvertemplate.Template{
			Data:         bytes.NewReader(templateData),
			RenderBundle: renderBundle,
		}
		return semverTemplate.Render(ctx)
	}
	return nil, fmt.Errorf("unsupported template schema %q", templateSchema)
}
