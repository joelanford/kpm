package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/joelanford/kpm/internal/pkg/builder"
	_ "github.com/joelanford/kpm/internal/pkg/builder/registryv1"
	"github.com/joelanford/kpm/internal/pkg/loader"
)

func BuildBundle() *cobra.Command {
	var reportFile string

	cmd := &cobra.Command{
		Use:  "bundle <spec-file>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			// 1. load kpm spec file
			//   - from cli arg
			// 2. TODO: evaluate spec file? (gotemplate, hcl, starlark, etc.)
			// 3. convert spec to builder
			l := loader.DefaultYAML
			b, err := l.LoadSpecFile(args[0])
			if err != nil {
				return err
			}

			// build it
			id, manifest, err := b.Build(ctx)
			if err != nil {
				return err
			}

			// write it
			// tag it
			// tar it
			report, err := builder.WriteKpmManifest(ctx, *id, manifest)
			if err != nil {
				return err
			}

			if reportFile != "" {
				if err := report.WriteFile(reportFile); err != nil {
					return err
				}
			}

			fmt.Printf("Bundle written to %s with tag %q (digest: %s)\n",
				report.OutputFile,
				report.Image.Reference,
				report.Image.Descriptor.Digest,
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&reportFile, "report-file", "", "if specified, path to write build report")
	return cmd
}
