package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/containers/image/v5/docker/reference"
	specsv1 "github.com/joelanford/kpm/internal/api/specs/v1"
	"github.com/joelanford/kpm/internal/bundle"
	"github.com/joelanford/kpm/internal/kpm"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
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
	b, err := bundle.NewRegistry(os.DirFS(bundleDir))
	if err != nil {
		return fmt.Errorf("failed to load registry bundle: %v", err)
	}

	tagRef, err := getBundleRef(spec.RegistryNamespace, b)
	if err != nil {
		return fmt.Errorf("failed to get tagged reference from spec file: %w", err)
	}

	// Open output file for writing
	if outputFile == "" {
		outputFile = fmt.Sprintf("%s-v%s.bundle.kpm", b.PackageName(), b.Version())
	}
	kpmFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}

	// Write it!
	desc, err := kpm.WriteImageManifest(kpmFile, tagRef, []fs.FS{b.FS()}, b.Annotations())
	if err != nil {
		// Clean up the file if we failed to write it
		_ = os.Remove(outputFile)
		return fmt.Errorf("failed to write kpm file: %v", err)
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

func getBundleRef(registryNamespace string, b bundle.Bundle) (reference.NamedTagged, error) {
	repoShortName := fmt.Sprintf("%s-bundle", b.PackageName())
	repoName := fmt.Sprintf("%s/%s", registryNamespace, repoShortName)
	nameRef, err := reference.ParseNamed(repoName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository name %q: %v", err)
	}
	tag := fmt.Sprintf("v%s", b.Version())
	return reference.WithTag(nameRef, tag)
}
