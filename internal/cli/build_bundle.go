package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	specsv1 "github.com/joelanford/kpm/internal/api/specs/v1"
	"github.com/joelanford/kpm/internal/bundle"
)

func BuildBundle() *cobra.Command {
	var (
		outputFile string
	)
	cmd := &cobra.Command{
		Use:   "bundle <bundleSpecFile>",
		Short: "Build a bundle",
		Long: `Build a kpm bundle from the specified bundle directory.
`,

		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			bundleSpecFile := args[0]
			if err := buildBundle(bundleSpecFile, outputFile); err != nil {
				cmd.PrintErrf("failed to build bundle: %v\n", err)
				os.Exit(1)
			}
		},
	}
	cmd.Flags().StringVarP(&outputFile, "output", "o", "",
		"Output file (default: <packageName>-<version>.bundle.kpm)")

	return cmd
}

// buildBundle reads a bundle directory, builds a kpm bundle, and writes it to an output file.
func buildBundle(specFileName, outputFile string) error {
	spec, err := readBundleSpec(specFileName)
	if err != nil {
		return fmt.Errorf("failed to read spec file: %w", err)
	}

	// Load the bundle
	wd := filepath.Dir(specFileName)
	bundleDir := filepath.Join(wd, spec.BundleRoot)
	if filepath.IsAbs(spec.BundleRoot) {
		bundleDir = spec.BundleRoot
	}
	b, err := bundle.NewRegistry(os.DirFS(bundleDir))
	if err != nil {
		return fmt.Errorf("failed to load registry bundle: %v", err)
	}

	if outputFile == "" {
		outputFile = fileForBundle(b)
	}

	tagRef, desc, err := bundle.BuildFile(outputFile, b, spec.RegistryNamespace)
	if err != nil {
		return errors.Join(
			fmt.Errorf("failed to build kpm bundle: %v", err),
			os.Remove(outputFile),
		)
	}

	fmt.Printf("Bundle written to %s with tag %q (digest: %s)\n", outputFile, tagRef, desc.Digest)
	return nil
}

func readBundleSpec(specFile string) (*specsv1.Bundle, error) {
	specData, err := os.ReadFile(specFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file: %w", err)
	}

	var spec specsv1.Bundle
	if err := yaml.Unmarshal(specData, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse catalog spec: %w", err)
	}
	expectedGVK := specsv1.GroupVersion.WithKind(specsv1.KindBundle)
	if spec.GroupVersionKind() != expectedGVK {
		return nil, fmt.Errorf("unsupported spec API found: %s, expected %s", spec.GroupVersionKind(), expectedGVK)
	}
	return &spec, nil
}

func fileForBundle(b bundle.Bundle) string {
	return fmt.Sprintf("%s-%s.bundle.kpm", b.PackageName(), b.Version())
}
