package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joelanford/kpm/internal/builder"
	_ "github.com/joelanford/kpm/internal/builder/registryv1"
	"github.com/joelanford/kpm/internal/loader"
)

func BuildBundle() *cobra.Command {
	var (
		values     []string
		reportFile string
	)

	cmd := &cobra.Command{
		Use:  "bundle <spec-file>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			// load input variables
			//   - values from flags
			//   - TODO: default files (or files from flag-based override)
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

			// 1. load kpm spec file
			//   - from cli arg
			// 2. evaluate spec file
			// 3. convert spec to builder
			l := loader.DefaultGoTemplate
			b, err := l.LoadSpecFile(args[0], templateData)
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
	cmd.Flags().StringSliceVar(&values, "set-value", nil, "set values for templating the spec file (e.g. key=value)")
	cmd.Flags().StringVar(&reportFile, "report-file", "", "if specified, path to write build report")
	return cmd
}
