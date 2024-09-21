package cli

import (
	"bytes"
	"context"
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

	stdaction "github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/action/migrations"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/operator-framework/operator-registry/alpha/template/basic"
	semvertemplate "github.com/operator-framework/operator-registry/alpha/template/semver"
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

USING A DIRECTORY OF BUNDLES

  Usage: kpm build catalog <bundleRoot> <tagRef>

  The simplest way to build a catalog is to provide a directory of bundles and a
  tag reference for the catalog. The tag should be a fully qualified image reference.
  In this mode, kpm will automatically generate a catalog from the bundles in the
  specified directory where newer versions of a bundle can upgrade from all previous
  versions.

  Most users should use this mode to build catalogs.

USING A SPEC FILE

  Usage: kpm build catalog <catalogSpecFile>

  When more configuration is needed to build a catalog, a spec file can be used. 
  kpm supports building catalogs using OLM's FBC and FBC template formats. With a
  spec file, users are exposed to more advanced features like template rendering and
  raw FBC.

  This mode is NOT RECOMMENDED for most users. It is intended for advanced users who
  are migrating their existing OLMv0 catalogs to kpm or who nned more control over
  the catalog generation process.

  
  EXAMPLE: spec file to build a catalog from raw FBC

    The FBC format is a directory of declarative config files. To build a catalog from
    an existing FBC directory, use a spec file like this:
  
      apiVersion: specs.kpm.io/v1
      kind: Catalog
      tag: "quay.io/myorg/my-catalog:latest"
      source:
        sourceType: fbc
        fbc:
          catalogRoot: ./path/to/fbc/root/

  EXAMPLE: spec file to build a catalog from a template

    The FBC template format is a single file that contains a template specification
    for generating FBC. kpm supports OLM's semver and basic templates. To build a
    catalog from an FBC template, use a spec file like this:

      apiVersion: specs.kpm.io/v1
      kind: Catalog
      tag: "quay.io/myorg/my-catalog:latest"
      source:
        sourceType: fbcTemplate
        fbcTemplate:
          templateFile: semver.yaml
`,
		Args: cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			switch len(args) {
			case 1:
				specFileName := args[0]
				if err := buildCatalogFromSpec(ctx, specFileName, outputFile); err != nil {
					cmd.PrintErrf("failed to build catalog: %v\n", err)
					os.Exit(1)
				}
			case 2:
				bundleRoot := args[0]
				tagRef, err := getCatalogRef(args[1])
				if err != nil {
					cmd.PrintErrf("failed to get tag ref: %v\n", err)
					os.Exit(1)
				}
				if err := buildCatalogFromBundles(ctx, bundleRoot, tagRef, outputFile); err != nil {
					cmd.PrintErrf("failed to build catalog: %v\n", err)
					os.Exit(1)
				}
			}
		},
	}
	cmd.Flags().StringVarP(&outputFile, "output", "o", "",
		"Output file (default: <repoBaseName>-<tag>.kpm)")
	return cmd
}

// buildCatalogFromBundles reads bundle files recursively from a directory, generates a catalog, and writes it to an output file.
//
// TODO: Move this logic outside the CLI package to make it easier to test and more reusable.
func buildCatalogFromBundles(ctx context.Context, bundleRoot string, tagRef reference.NamedTagged, outputFile string) error {
	tmpCatalogDir, err := os.MkdirTemp("", "kpm-tmp-fbc-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary catalog file: %w", err)
	}
	defer os.Remove(tmpCatalogDir)

	tmpCatalogFile, err := os.Create(filepath.Join(tmpCatalogDir, "catalog.yaml"))
	if err != nil {
		return fmt.Errorf("failed to create temporary catalog file: %w", err)
	}
	defer tmpCatalogFile.Close()

	type bundleMeta struct {
		name    string
		version semver.Version
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
	bundles := map[string][]bundleMeta{}
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

		m, _ := migrations.NewMigrations(migrations.AllMigrations)
		logrus.SetOutput(io.Discard)
		r := action.Render{
			Migrations:     m,
			AllowedRefMask: stdaction.RefBundleDir,
		}
		fbc, err := r.Render(ctx, path)
		if err != nil {
			return fmt.Errorf("failed to render bundle %q: %w", path, err)
		}
		vers, err := parseVersion(&fbc.Bundles[0])
		if err != nil {
			return fmt.Errorf("failed to parse bundle version: %w", err)
		}
		bundles[fbc.Bundles[0].Package] = append(bundles[fbc.Bundles[0].Package], bundleMeta{
			name:    fbc.Bundles[0].Name,
			version: vers,
		})
		if err := declcfg.WriteYAML(*fbc, tmpCatalogFile); err != nil {
			return fmt.Errorf("failed to write bundle to catalog: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	pkgNames := maps.Keys(bundles)
	slices.Sort(pkgNames)

	for _, pkgName := range pkgNames {
		metas := bundles[pkgName]
		slices.SortFunc(metas, func(i, j bundleMeta) int {
			return j.version.Compare(i.version)
		})
		fbcPkg := declcfg.Package{
			Schema:         declcfg.SchemaPackage,
			Name:           pkgName,
			DefaultChannel: "default",
			// TODO: fill in icon from latest version
			Icon: nil,
		}
		fbcCh := declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Package: pkgName,
			Name:    "default",
			Entries: make([]declcfg.ChannelEntry, len(metas)),
		}
		for i, meta := range metas {
			entry := declcfg.ChannelEntry{
				Name:      meta.name,
				SkipRange: fmt.Sprintf("<%s", meta.version.String()),
			}
			if i < len(metas)-1 {
				entry.Replaces = metas[i+1].name
			}
			fbcCh.Entries[i] = entry
		}
		fbc := declcfg.DeclarativeConfig{
			Packages: []declcfg.Package{fbcPkg},
			Channels: []declcfg.Channel{fbcCh},
		}
		if err := declcfg.WriteYAML(fbc, tmpCatalogFile); err != nil {
			return fmt.Errorf("failed to write package to catalog: %w", err)
		}
	}
	return writeCatalogFsys(os.DirFS(tmpCatalogDir), tagRef, outputFile)
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

	rootDir := filepath.Dir(specFileName)

	var (
		fbcFsys fs.FS
	)
	switch spec.Source.SourceType {
	case specsv1.CatalogSpecSourceTypeFBC:
		fbcFsys = os.DirFS(filepath.Join(rootDir, spec.Source.FBC.CatalogRoot))
		fbc, err := declcfg.LoadFS(ctx, fbcFsys)
		if err != nil {
			return fmt.Errorf("failed to load catalog: %v", err)
		}
		if _, err := declcfg.ConvertToModel(*fbc); err != nil {
			return fmt.Errorf("failed to validate catalog: %v", err)
		}
	case specsv1.CatalogSpecSourceTypeFBCTemplate:
		templateSchema, templateData, err := getTemplateData(rootDir, spec.Source.FBCTemplate.TemplateFile)
		if err != nil {
			return fmt.Errorf("failed to get template data: %v", err)
		}
		templateOutputTmpDir, err := renderTemplate(ctx, templateSchema, templateData, spec.Source.FBCTemplate.MigrationLevel)
		if err != nil {
			return fmt.Errorf("failed to render template: %v", err)
		}
		defer os.RemoveAll(templateOutputTmpDir)
		fbcFsys = os.DirFS(templateOutputTmpDir)
	default:
		return fmt.Errorf("unsupported source type %q", spec.Source.SourceType)
	}
	return writeCatalogFsys(fbcFsys, tagRef, outputFile)
}

func writeCatalogFsys(fbcFsys fs.FS, tagRef reference.NamedTagged, outputFile string) error {
	annotations := map[string]string{
		containertools.ConfigsLocationLabel: "/configs",
	}

	configsFsys, err := fsutil.Prefix(fbcFsys, "/configs")
	if err != nil {
		return fmt.Errorf("failed to create configs fs: %v", err)
	}
	layers := []fs.FS{
		configsFsys,
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

func renderTemplate(ctx context.Context, templateSchema string, templateData []byte, migrationLevel string) (string, error) {
	if migrationLevel == "" {
		migrationLevel = migrations.NoMigrations
	}
	m, err := migrations.NewMigrations(migrationLevel)
	if err != nil {
		return "", fmt.Errorf("failed to configure migrations: %w", err)
	}

	renderBundle := func(ctx context.Context, ref string) (*declcfg.DeclarativeConfig, error) {
		r := action.Render{
			Migrations: m,
		}
		return r.Render(ctx, ref)
	}

	logrus.SetOutput(io.Discard)

	var fbc *declcfg.DeclarativeConfig
	switch templateSchema {
	case "olm.template.basic":
		basicTemplate := basic.Template{
			RenderBundle: renderBundle,
		}
		fbc, err = basicTemplate.Render(ctx, bytes.NewReader(templateData))
	case "olm.semver":
		semverTemplate := semvertemplate.Template{
			Data:         bytes.NewReader(templateData),
			RenderBundle: renderBundle,
		}
		fbc, err = semverTemplate.Render(ctx)
	default:
		return "", fmt.Errorf("unsupported template schema %q", templateSchema)
	}
	if err != nil {
		return "", err
	}
	templateOutputTmpDir, err := os.MkdirTemp("", "kpm-build-catalog-template-output")
	if err != nil {
		return "", fmt.Errorf("failed to create template output dir: %w", err)
	}
	if err := declcfg.WriteFS(*fbc, templateOutputTmpDir, declcfg.WriteYAML, ".yaml"); err != nil {
		return "", fmt.Errorf("failed to write rendered template to fs: %w", err)
	}
	return templateOutputTmpDir, nil
}
