package cli

import (
	"fmt"
	"os"

	"github.com/joelanford/kpm/internal/bundle"
	"github.com/spf13/cobra"
)

func BuildBundle() *cobra.Command {
	var (
		outputFileTemplate string
	)
	cmd := &cobra.Command{
		Use:   "bundle <bundleSpecFile>",
		Short: "Build a bundle",
		Long: `Build a kpm bundle from the specified bundle directory.
`,

		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			bundleSpecFile := args[0]

			outputFile, tagRef, desc, err := bundle.BuildFromSpecFile(bundleSpecFile, bundle.FilenameFromTemplate(outputFileTemplate))
			if err != nil {
				cmd.PrintErrf("failed to build bundle: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Bundle written to %s with tag %q (digest: %s)\n", outputFile, tagRef, desc.Digest)
		},
	}
	cmd.Flags().StringVarP(&outputFileTemplate, "output-file", "o", "{.PackageName}-v{.Version}.bundle.kpm",
		"Output file name (use {.Package} and/or {.Version} to automatically inject package name and version)")

	return cmd
}
