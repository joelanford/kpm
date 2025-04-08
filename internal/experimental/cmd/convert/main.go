package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/operator-framework/operator-registry/alpha/declcfg"

	"github.com/joelanford/kpm/internal/experimental/graph"
)

func main() {
	cmd := cobra.Command{
		Use:  "convert <fbcDirectory>...",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			idx := graph.NewIndex()
			for _, arg := range args {
				dist, fbcDir, ok := strings.Cut(arg, ":")
				if !ok {
					return fmt.Errorf("invalid argument %q", arg)
				}
				fbc, err := declcfg.LoadFS(cmd.Context(), os.DirFS(fbcDir))
				if err != nil {
					return fmt.Errorf("loading declarative config: %w", err)
				}
				if err := idx.AddFBC(*fbc, map[string][]string{"dist": {dist}}); err != nil {
					return fmt.Errorf("adding FBC: %w", err)
				}
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false)
			return enc.Encode(idx.Graph())
		},
	}
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
}
