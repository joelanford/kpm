package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	"github.com/joelanford/kpm/internal/action"
	specsv1 "github.com/joelanford/kpm/internal/api/specs/v1"
	"github.com/joelanford/kpm/internal/fsutil"
	"github.com/joelanford/kpm/internal/kpm"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/operator-framework/operator-registry/alpha/action/migrations"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/template/basic"
	"github.com/operator-framework/operator-registry/alpha/template/semver"
	"github.com/operator-framework/operator-registry/pkg/containertools"
)

func BuildCatalog() *cobra.Command {
	var (
		outputFile string
	)
	cmd := &cobra.Command{
		Use:   "catalog <catalogSpecFile>",
		Short: "Build a catalog",
		Long: `Build a kpm catalog from the specified catalog directory.
`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			specFileName := args[0]
			if err := buildCatalog(ctx, specFileName, outputFile); err != nil {
				cmd.PrintErrf("failed to build catalog: %v\n", err)
				os.Exit(1)
			}
		},
	}
	cmd.Flags().StringVarP(&outputFile, "output", "o", "",
		"Output file (default: <repoBaseName>-<tag>.kpm)")
	return cmd
}

// buildCatalog reads a spec file, builds a kpm catalog from the spec, and writes it to an output file.
//
// TODO: Move this logic outside the CLI package to make it easier to test and more reusable.
func buildCatalog(ctx context.Context, specFileName, outputFile string) error {
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
		semverTemplate := semver.Template{
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
