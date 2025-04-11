package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	"github.com/spf13/cobra"

	"github.com/joelanford/kpm/internal/experimental/spec"
)

func main() {
	var (
		values              []string
		imageOverrideValues []string
	)

	cmd := cobra.Command{
		Use:  "bb <kpmspecFile>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			templateData := map[string]any{}
			for _, value := range values {
				k, v, ok := strings.Cut(value, "=")
				if !ok {
					return fmt.Errorf("invalid set-value %q", value)
				}
				templateData[k] = v
			}

			imageOverrides := map[string]string{}
			for _, override := range imageOverrideValues {
				k, v, ok := strings.Cut(override, "=")
				if !ok {
					return fmt.Errorf("invalid set-image %q", override)
				}
				imageOverrides[k] = v
			}

			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			l := spec.DefaultLoader
			spec, err := l.LoadSpecFile(args[0], templateData, imageOverrides)
			if err != nil {
				return err
			}
			r, err := reference.ParseNamed("quay.io/joelanford/test:202504111202")
			if err != nil {
				return err
			}
			nt, ok := r.(reference.NamedTagged)
			if !ok {
				return fmt.Errorf("expected a tagged reference, got %T", r)
			}
			return spec.Build(nt, ".")
		},
	}
	cmd.Flags().StringSliceVar(&values, "set-value", nil, "set values for templating the spec file (e.g. key=value)")
	cmd.Flags().StringSliceVar(&imageOverrideValues, "set-image", nil, "set image overrides (e.g. name=newImageRef)")
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
