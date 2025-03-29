package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/joelanford/kpm/internal/bundle"
)

func BuildBundle() *cobra.Command {
	var (
		fileTemplate string
		reportFile   string
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
			fmt.Printf("Bundle written to %s with tag %q (digest: %s)\n", res.FilePath, fmt.Sprintf("%s:%s", res.Repository, res.Tag), res.Descriptor.Digest)

			if reportFile != "" {
				f, err := os.Create(reportFile)
				if err != nil {
					cmd.PrintErrf("failed to create report file: %v\n", err)
					os.Exit(1)
				}
				defer f.Close()

				enc := json.NewEncoder(f)
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				if err := enc.Encode(res); err != nil {
					cmd.PrintErrf("failed to write report for result to %s: %v", reportFile, errors.Join(err, os.Remove(reportFile)))
					os.Exit(1)
				}
			}
		},
	}
	cmd.Flags().StringVarP(&fileTemplate, "file", "f", "{.PackageName}-v{.Version}.bundle.kpm",
		"Templated path for output file name (use {.Package} and/or {.Version} to automatically inject package name and version)")
	cmd.Flags().StringVar(&reportFile, "report-file", "", "Optionally, a file in which to write a JSON report of the build result.")
	return cmd
}
