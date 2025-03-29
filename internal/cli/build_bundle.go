package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/joelanford/kpm/internal/bundle"
)

func BuildBundle() *cobra.Command {
	var (
		fileTemplate string
		reportFormat string
	)
	cmd := &cobra.Command{
		Use:   "bundle <bundleSpecFile>",
		Short: "Build a bundle",
		Long: `Build a kpm bundle from the specified bundle directory.
`,

		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			bundleSpecFile := args[0]

			res, err := bundle.BuildFromSpecFile(ctx, bundleSpecFile, bundle.StringFromBundleTemplate(fileTemplate))
			if err != nil {
				cmd.PrintErrf("failed to build bundle: %v\n", err)
				os.Exit(1)
			}

			switch reportFormat {
			case "":
				fmt.Printf("Bundle written to %s with tag %q (digest: %s)\n", res.FilePath, fmt.Sprintf("%s:%s", res.Repository, res.Tag), res.Descriptor.Digest)
			case "json":
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				if err := enc.Encode(res); err != nil {
					cmd.PrintErrf("failed to write report for result: %v", err)
					os.Exit(1)
				}
			}
		},
	}
	cmd.Flags().StringVarP(&fileTemplate, "file", "f", "{.PackageName}-v{.Version}.bundle.kpm",
		"Templated path for output file name (use {.Package} and/or {.Version} to automatically inject package name and version)")
	cmd.Flags().StringVar(&reportFormat, "report-format", "", "The report format. Default is human-readable text. Options are [json].")
	return cmd
}
