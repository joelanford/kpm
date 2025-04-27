package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/google/renameio/v2"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	"github.com/joelanford/kpm/internal/experimental/builder"
	_ "github.com/joelanford/kpm/internal/experimental/builder/registryv1"
	"github.com/joelanford/kpm/internal/experimental/loader"
	"github.com/joelanford/kpm/internal/tar"
)

func main() {
	var (
		imageRef string
		values   []string
	)

	cmd := cobra.Command{
		Use:  "bb <kpmspecFile>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			templateData := loader.GoTemplateData{
				Values: map[string]any{},
			}
			for _, value := range values {
				k, v, ok := strings.Cut(value, "=")
				if !ok {
					return fmt.Errorf("invalid set-value %q", value)
				}
				templateData.Values[k] = v
			}
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			// load input variables
			//   - default files (or files from flag-based override)
			//   - values from flag-based overrides

			// load kpm spec file
			//   - from cli arg

			// evaluate spec file

			// convert spec to builder

			// get NVR from builder

			// build it

			// tag it

			// write it

			l := loader.DefaultGoTemplate
			b, err := l.LoadSpecFile(args[0], templateData)
			if err != nil {
				return err
			}

			return writeFile(cmd.Context(), b, imageRef)
		},
	}
	cmd.Flags().StringSliceVar(&values, "set-value", nil, "set values for templating the spec file (e.g. key=value)")
	cmd.Flags().StringVar(&imageRef, "ref", "", "image reference to tag into kpm file")
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeFile(ctx context.Context, w builder.Builder, ref string) error {
	id, manifest, err := w.Build(ctx)
	if err != nil {
		return err
	}
	kpmManifest := builder.NewKPMManifest(manifest, *id)

	tmpDir, err := os.MkdirTemp("", "kpm-build-bundle-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	pusher, err := oci.NewWithContext(ctx, tmpDir)
	if err != nil {
		return err
	}

	config, layers, err := kpmManifest.PushConfigAndLayers(ctx, pusher)
	if err != nil {
		return err
	}

	man := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: kpmManifest.ArtifactType(),
		Config:       config,
		Layers:       layers,
		Subject:      kpmManifest.Subject(),
		Annotations:  kpmManifest.Annotations(),
	}
	manData, err := json.Marshal(man)
	if err != nil {
		return err
	}
	manDesc, err := oras.PushBytes(ctx, pusher, ocispec.MediaTypeImageManifest, manData)
	if err != nil {
		return err
	}

	if err := pusher.Tag(ctx, manDesc, ref); err != nil {
		return err
	}

	pf, err := renameio.NewPendingFile(id.Filename())
	if err != nil {
		return err
	}
	defer pf.Cleanup()

	if err := tar.Directory(pf, os.DirFS(tmpDir)); err != nil {
		return err
	}
	return pf.CloseAtomicallyReplace()
}
