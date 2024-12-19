package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing/fstest"
	"text/template"

	mmsemver "github.com/Masterminds/semver/v3"
	"github.com/blang/semver/v4"
	"github.com/containers/image/v5/docker/reference"
	"github.com/dschmidt/go-layerfs"
	sprig "github.com/go-sprout/sprout/sprigin"
	_ "github.com/mattn/go-sqlite3"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/operator-registry/alpha/action/migrations"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/operator-framework/operator-registry/alpha/template/basic"
	semvertemplate "github.com/operator-framework/operator-registry/alpha/template/semver"
	"github.com/operator-framework/operator-registry/pkg/cache"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/operator-framework/operator-registry/pkg/image"
	"github.com/operator-framework/operator-registry/pkg/registry"
	"github.com/operator-framework/operator-registry/pkg/sqlite"

	"github.com/joelanford/kpm/internal/action"
	specsv1 "github.com/joelanford/kpm/internal/api/specs/v1"
	"github.com/joelanford/kpm/internal/bundle"
	"github.com/joelanford/kpm/internal/fsutil"
	"github.com/joelanford/kpm/internal/kpm"
)

func BuildCatalog() *cobra.Command {
	var (
		outputDirectory string
	)
	cmd := &cobra.Command{
		Use:   "catalog <catalogSpecFile>",
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
`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			specFileName := args[0]
			if err := buildCatalogFromSpec(ctx, specFileName, outputDirectory); err != nil {
				cmd.PrintErrf("failed to build catalog: %v\n", err)
				os.Exit(1)
			}
		},
	}
	cmd.Flags().StringVarP(&outputDirectory, "output", "o", "",
		"Output directory (default: current working directory)")
	return cmd
}

// buildCatalogFromSpec reads a spec file, builds a kpm catalog from the spec, and writes it to an output file.
//
// TODO: Move this logic outside the CLI package to make it easier to test and more reusable.
func buildCatalogFromSpec(ctx context.Context, specFileName, outputDirectory string) error {
	specFileDir := filepath.Dir(specFileName)
	spec, err := readCatalogSpec(specFileName)
	if err != nil {
		return fmt.Errorf("failed to read spec file: %w", err)
	}

	tagRef, err := parseTagRef(spec.ImageReference)
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

	var fbc *declcfg.DeclarativeConfig
	switch spec.Source.SourceType {
	case specsv1.CatalogSpecSourceTypeBundles:
		fbc, err = renderBundlesDir(ctx, pathForSpecPath(specFileDir, spec.Source.Bundles.BundleRoot))
	case specsv1.CatalogSpecSourceTypeFBC:
		fbc, err = declcfg.LoadFS(ctx, os.DirFS(pathForSpecPath(specFileDir, spec.Source.FBC.CatalogRoot)))
	case specsv1.CatalogSpecSourceTypeFBCTemplate:
		var (
			templateSchema string
			templateData   []byte
		)
		templateSchema, templateData, err = getTemplateData(specFileDir, spec.Source.FBCTemplate.TemplateFile)
		if err != nil {
			return fmt.Errorf("failed to get template data: %v", err)
		}
		fbc, err = renderTemplate(ctx, templateSchema, templateData)
	case specsv1.CatalogSpecSourceTypeFBCGoTemplate:
		logrus.SetOutput(io.Discard)
		bundleSpecGlobs := mapSlice(spec.Source.FBCGoTemplate.BundleSpecGlobs, func(glob string) string {
			return pathForSpecPath(specFileDir, glob)
		})
		templateFile := pathForSpecPath(specFileDir, spec.Source.FBCGoTemplate.TemplateFile)
		templateHelperGlobs := mapSlice(spec.Source.FBCGoTemplate.TemplateHelperGlobs, func(glob string) string {
			return pathForSpecPath(specFileDir, glob)
		})
		valuesFiles := mapSlice(spec.Source.FBCGoTemplate.ValuesFiles, func(vf string) string {
			return pathForSpecPath(specFileDir, vf)
		})
		fbc, err = renderFBCGoTemplate(ctx, bundleSpecGlobs, templateFile, templateHelperGlobs, valuesFiles, outputDirectory)
	case specsv1.CatalogSpecSourceTypeLegacy:
		fbc, err = renderLegacyBundlesDir(ctx,
			pathForSpecPath(specFileDir, spec.Source.Legacy.BundleRoot),
			spec.Source.Legacy.BundleImageReference,
			outputDirectory)
	default:
		return fmt.Errorf("unsupported source type %q", spec.Source.SourceType)
	}
	if err != nil {
		return fmt.Errorf("failed to render FBC: %w", err)
	}

	if _, err := declcfg.ConvertToModel(*fbc); err != nil {
		return fmt.Errorf("failed to validate FBC: %w", err)
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
	fbcFsys := os.DirFS(tmpFBCDir)

	if spec.CacheFormat == "" {
		spec.CacheFormat = "json"
	}
	switch spec.CacheFormat {
	case "json", "pogreb.v1":
		break // all other cases return within the switch
	case "none":
		return writeCatalog(fbcFsys, nil, tagRef, outputDirectory)
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
	cacheFsys, err := fsutil.Prefix("/tmp/cache", os.DirFS(tmpCacheDir))
	if err != nil {
		return fmt.Errorf("failed to create cache fs: %v", err)
	}

	return writeCatalog(fbcFsys, cacheFsys, tagRef, outputDirectory)
}

func mapSlice[I, O any](in []I, mapFunc func(I) O) []O {
	out := make([]O, 0, len(in))
	for _, i := range in {
		out = append(out, mapFunc(i))
	}
	return out
}

func writeCatalog(fbcFsys fs.FS, cacheFsys fs.FS, tagRef reference.NamedTagged, outputDirectory string) error {
	annotations := map[string]string{
		containertools.ConfigsLocationLabel: "/configs",
	}

	configsFsys, err := fsutil.Prefix("/configs", fbcFsys)
	if err != nil {
		return fmt.Errorf("failed to create configs fs: %v", err)
	}
	layers := []fs.FS{configsFsys}
	if cacheFsys != nil {
		layers = append(layers, cacheFsys)
	}

	// Open output file for writing
	pathParts := strings.Split(reference.Path(reference.TrimNamed(tagRef)), "/")
	baseName := pathParts[len(pathParts)-1]
	outputFile := filepath.Join(outputDirectory, fmt.Sprintf("%s-%s.catalog.kpm", baseName, tagRef.Tag()))
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

func parseTagRef(imageReference string) (reference.NamedTagged, error) {
	namedRef, err := reference.ParseNamed(imageReference)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference %q: %v", imageReference, err)
	}

	tagRef, ok := namedRef.(reference.NamedTagged)
	if !ok {
		return nil, fmt.Errorf("image reference %q is not a tagged reference", imageReference)
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

		r := action.Render{}
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
			name:    bundle.Name,
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

func renderLegacyBundlesDir(ctx context.Context, bundleRoot, bundleImageReference, outputDirectory string) (*declcfg.DeclarativeConfig, error) {
	logrus.SetOutput(io.Discard)

	db, err := sql.Open("sqlite3", "file::memory:?mode=memory&cache=shared")
	if err != nil {
		return nil, err
	}

	m, err := sqlite.NewSQLLiteMigrator(db)
	if err != nil {
		return nil, err
	}
	if err := m.Migrate(ctx); err != nil {
		return nil, err
	}

	loader, err := sqlite.NewSQLLiteLoader(db)
	if err != nil {
		return nil, err
	}

	querier := sqlite.NewSQLLiteQuerierFromDb(db)

	isPkgManDir, err := isPackageManifestsDir(bundleRoot)
	if err != nil {
		return nil, err
	}
	if isPkgManDir {
		err = populateFromLegacyPackageManifestDir(loader, bundleRoot, bundleImageReference, outputDirectory)
	} else {
		var graphLoader registry.GraphLoader
		graphLoader, err = sqlite.NewSQLGraphLoaderFromDB(db)
		if err != nil {
			return nil, err
		}
		err = populateFromLegacyBundlesDirectory(loader, querier, graphLoader, bundleRoot, bundleImageReference, outputDirectory)
	}
	model, err := sqlite.ToModel(ctx, querier)
	if err != nil {
		return nil, err
	}
	fbc := declcfg.ConvertFromModel(model)

	if err := populateDBRelatedImages(ctx, &fbc, db); err != nil {
		return nil, err
	}

	return &fbc, err
}

func isPackageManifestsDir(bundleRoot string) (bool, error) {
	entries, err := os.ReadDir(bundleRoot)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".package.yaml") {
			return true, nil
		}
	}
	return false, nil
}

func readPackageManifestsFile(bundleRoot string) (*registry.PackageManifest, []string, error) {
	entries, err := os.ReadDir(bundleRoot)
	if err != nil {
		return nil, nil, err
	}
	var (
		pm         *registry.PackageManifest
		bundleDirs []string
		errs       []error
	)
	for _, entry := range entries {
		path := filepath.Join(bundleRoot, entry.Name())
		if entry.IsDir() {
			bundleDirs = append(bundleDirs, path)
			continue
		}
		fileData, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var tryPM registry.PackageManifest
		if err := yaml.Unmarshal(fileData, &tryPM); err != nil {
			continue
		}
		if tryPM.PackageName != "" {
			if pm != nil {
				return nil, nil, fmt.Errorf("multiple packages manifest files found in %q, only 1 allowed", bundleRoot)
			}
			pm = &tryPM
		}
	}
	if pm != nil {
		return pm, bundleDirs, nil
	}
	if len(errs) > 0 {
		return nil, nil, fmt.Errorf("failed trying to find package manifests layout: %v", errs)
	}
	return nil, nil, nil
}

func populateFromLegacyPackageManifestDir(loader registry.Load, bundleRoot, bundleImageReference, outputDirectory string) error {
	pm, bundleDirs, err := readPackageManifestsFile(bundleRoot)
	if err != nil {
		return err
	}
	for _, bundleDir := range bundleDirs {
		annotations := map[string]map[string]string{
			"annotations": {
				"operators.operatorframework.io.bundle.package.v1":   pm.PackageName,
				"operators.operatorframework.io.bundle.mediatype.v1": "registry+v1",
				"operators.operatorframework.io.bundle.manifests.v1": "manifests/",
				"operators.operatorframework.io.bundle.metadata.v1":  "metadata/",
			},
		}
		annotationsYAML, err := yaml.Marshal(annotations)
		if err != nil {
			return err
		}
		manifestsFS, err := fsutil.Prefix("manifests", os.DirFS(bundleDir))
		if err != nil {
			return err
		}
		metadataFS, err := fsutil.Prefix("metadata", &fstest.MapFS{
			"annotations.yaml": &fstest.MapFile{
				Data: annotationsYAML,
				Mode: 0644,
			},
		})
		if err != nil {
			return err
		}

		bundleFS := layerfs.New(manifestsFS, metadataFS)
		if _, _, _, err = buildBundle(bundleFS, bundleImageReference, outputDirectory); err != nil {
			return err
		}
	}

	dl := sqlite.NewSQLLoaderForDirectory(loader, bundleRoot)
	return dl.Populate()
}

func populateFromLegacyBundlesDirectory(loader registry.Load, querier registry.Query, graphLoader registry.GraphLoader, bundleRoot, bundleImageReference, outputDirectory string) error {
	updateGraphMode, err := getLegacyUpdateGraphMode(bundleRoot)
	if err != nil {
		return err
	}

	imgDirMap := map[image.Reference]string{}
	dirEntries, err := os.ReadDir(bundleRoot)
	if err != nil {
		return err
	}
	for _, dirEntry := range dirEntries {
		if !dirEntry.IsDir() {
			continue
		}
		bundleDir := filepath.Join(bundleRoot, dirEntry.Name())
		_, tagRef, desc, err := buildBundle(os.DirFS(bundleDir), bundleImageReference, outputDirectory)
		if err != nil {
			return err
		}
		digestRef, err := reference.WithDigest(tagRef, desc.Digest)
		if err != nil {
			return err
		}
		imgDirMap[digestRef] = bundleDir
	}
	dp := registry.NewDirectoryPopulator(loader, graphLoader, querier, imgDirMap, nil)
	return dp.Populate(updateGraphMode)
}

func buildBundle(bundleFS fs.FS, bundleImageReference string, outputDirectory string) (string, reference.NamedTagged, ocispec.Descriptor, error) {
	b, err := bundle.NewRegistry(bundleFS)
	if err != nil {
		return "", nil, ocispec.Descriptor{}, err
	}

	outputFile := filepath.Join(outputDirectory, fmt.Sprintf("%s-%s.bundle.kpm", b.PackageName(), b.Version()))
	imageRef, err := bundle.StringFromBundleTemplate(bundleImageReference)(b)
	if err != nil {
		return "", nil, ocispec.Descriptor{}, err
	}
	tagRef, desc, err := bundle.BuildFile(outputFile, b, imageRef)
	if err != nil {
		return "", nil, ocispec.Descriptor{}, err
	}
	fmt.Printf("Bundle written to %s with tag %q (digest: %s)\n", outputFile, tagRef, desc.Digest)
	return outputFile, tagRef, desc, nil
}

func getLegacyUpdateGraphMode(bundleRoot string) (registry.Mode, error) {
	type ciCfg struct {
		UpdateGraph string `json:"updateGraph"`
	}
	var ci ciCfg
	ciCfgData, err := os.ReadFile(filepath.Join(bundleRoot, "ci.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return registry.ReplacesMode, nil
		}
		return -1, err
	}
	if err := yaml.Unmarshal(ciCfgData, &ci); err != nil {
		return -1, err
	}
	return registry.GetModeFromString(strings.TrimSuffix(ci.UpdateGraph, "-mode"))
}

func getTemplateData(specFileDir, templateFile string) (string, []byte, error) {
	if !filepath.IsAbs(templateFile) {
		templateFile = pathForSpecPath(specFileDir, templateFile)
	}
	templateData, err := os.ReadFile(templateFile)
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

func populateDBRelatedImages(ctx context.Context, cfg *declcfg.DeclarativeConfig, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "SELECT image, operatorbundle_name FROM related_image")
	if err != nil {
		return err
	}
	defer rows.Close()

	images := map[string]sets.Set[string]{}
	for rows.Next() {
		var (
			img        sql.NullString
			bundleName sql.NullString
		)
		if err := rows.Scan(&img, &bundleName); err != nil {
			return err
		}
		if !img.Valid || !bundleName.Valid {
			continue
		}
		m, ok := images[bundleName.String]
		if !ok {
			m = sets.New[string]()
		}
		m.Insert(img.String)
		images[bundleName.String] = m
	}

	for i, b := range cfg.Bundles {
		ris, ok := images[b.Name]
		if !ok {
			continue
		}
		for _, ri := range b.RelatedImages {
			if ris.Has(ri.Image) {
				ris.Delete(ri.Image)
			}
		}
		for _, ri := range sets.List(ris) {
			cfg.Bundles[i].RelatedImages = append(cfg.Bundles[i].RelatedImages, declcfg.RelatedImage{Image: ri})
		}
	}
	return nil
}

func renderFBCGoTemplate(ctx context.Context, bundleSpecGlobs []string, templateFile string, templateHelperGlobs, valuesFiles []string, outputDirectory string) (*declcfg.DeclarativeConfig, error) {
	var bundleKpmFiles []string
	for _, bundleSpecGlob := range bundleSpecGlobs {
		globMatches, err := filepath.Glob(bundleSpecGlob)
		if err != nil {
			return nil, err
		}
		bundleKpmFiles = append(bundleKpmFiles, globMatches...)
	}

	bundlesMetas := map[string]declcfg.Meta{}
	for _, bundleKpmFile := range bundleKpmFiles {
		outputFile, tagRef, desc, err := bundle.BuildFromSpecFile(bundleKpmFile,
			bundle.StringFromBundleTemplate(filepath.Join(outputDirectory, "{.PackageName}-v{.Version}.bundle.kpm")),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to build bundle: %w", err)
		}
		fmt.Printf("Bundle written to %s with tag %q (digest: %s)\n", outputFile, tagRef, desc.Digest)

		r := action.Render{}
		fileFbc, err := r.Render(ctx, outputFile)
		if err != nil {
			return nil, err
		}
		pr, pw := io.Pipe()
		go func() {
			pw.CloseWithError(declcfg.WriteJSON(*fileFbc, pw))
		}()

		var metas []declcfg.Meta
		declcfg.WalkMetasReader(pr, func(m *declcfg.Meta, err error) error {
			if err != nil {
				return err
			}
			metas = append(metas, *m)
			return nil
		})
		if len(metas) != 1 {
			return nil, fmt.Errorf("expected 1 object that represents a bundle, got %d", len(metas))
		}
		bundlesMetas[metas[0].Name] = metas[0]
	}

	tmpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return nil, err
	}
	tmplName := tmpl.Templates()[0].Name()
	tmpl = tmpl.Funcs(sprig.HermeticTxtFuncMap())

	sortSemver := func(in []any) []any {
		slices.SortFunc(in, func(a, b any) int {
			aV, bV := a.(*mmsemver.Version), b.(*mmsemver.Version)
			return aV.Compare(bV)
		})
		return in
	}
	tmpl = tmpl.Funcs(map[string]any{"sortSemver": sortSemver})

	for _, glob := range templateHelperGlobs {
		tmpl, err = tmpl.ParseGlob(glob)
		if err != nil {
			return nil, err
		}
	}

	values := map[string]any{}
	for _, valuesFile := range valuesFiles {
		vfData, err := os.ReadFile(valuesFile)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := yaml.Unmarshal(vfData, &m); err != nil {
			return nil, err
		}
		values = mergeMaps(values, m)
	}

	data := map[string]any{
		"Values":  values,
		"Bundles": bundlesMetas,
	}

	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(tmpl.ExecuteTemplate(pw, tmplName, data))
	}()
	return declcfg.LoadReader(pr)
}

func mergeMaps(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if v, ok := v.(map[string]any); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]any); ok {
					out[k] = mergeMaps(bv, v)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

func pathForSpecPath(specFileDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(specFileDir, path)
}
