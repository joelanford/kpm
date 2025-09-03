package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/operator-framework/kpm/internal/pkg/spec"
)

func Build() *cobra.Command {
	var reportFile string

	cmd := &cobra.Command{
		Use:  "build <spec-file>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			// load kpm spec file from cli arg
			//   TODO: evaluate/render spec file? (gotemplate, hcl, starlark, etc.)
			specFileLoader := spec.DefaultYAML
			specFile, err := specFileLoader.LoadSpecFile(args[0])
			if err != nil {
				return err
			}

			// write it
			report, err := spec.Build(ctx, specFile)
			if err != nil {
				return err
			}

			if reportFile != "" {
				if err := report.WriteFile(reportFile); err != nil {
					return err
				}
			}

			fmt.Printf("%s written to %s (digest: %s)\n",
				report.ID,
				report.OutputFile,
				report.Descriptor.Digest,
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&reportFile, "report-file", "", "if specified, path to write build report")
	return cmd
}
