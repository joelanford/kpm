package cli

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/joelanford/kpm/action"
	"github.com/joelanford/kpm/internal/console"
)

func BuildBundle() *cobra.Command {
	var (
		workingDirectory string
	)
	cmd := &cobra.Command{
		Use:  "bundle <spec-file> <output-file>",
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			console.Secondaryf("⏳  Building bundle for %s", args[0])
			if err := runBuildBundle(ctx, runBundleOptions{
				specFile:         args[0],
				outputFile:       args[1],
				workingDirectory: workingDirectory,
			}); err != nil {
				console.Fatalf(1, "💥 %s", err)
			}
			console.Primaryf("📦 %s created!", args[1])
		},
	}
	cmd.Flags().StringVarP(&workingDirectory, "working-directory", "C", "", "working directory used to resolve relative paths for bundle content")
	return cmd
}

type runBundleOptions struct {
	specFile         string
	outputFile       string
	workingDirectory string
}

func runBuildBundle(ctx context.Context, opts runBundleOptions) error {
	specReader, err := os.Open(opts.specFile)
	if err != nil {
		return err
	}

	outputWriter, err := os.Create(opts.outputFile)
	if err != nil {
		return err
	}
	defer outputWriter.Close()

	if opts.workingDirectory == "" {
		opts.workingDirectory = "."
	}

	bb := action.BuildBundle{
		SpecFileReader:    specReader,
		SpecFileWorkingFS: os.DirFS(opts.workingDirectory),
		BundleWriter:      outputWriter,
	}
	if err := bb.Run(ctx); err != nil {
		defer os.Remove(opts.outputFile)
		return err
	}
	return nil
}
