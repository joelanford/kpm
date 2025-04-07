package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/operator-framework/operator-registry/alpha/declcfg"

	"github.com/joelanford/kpm/internal/experimental/graph"
)

func main() {
	cmd := cobra.Command{
		Use:  "convert <fbcDirectory>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			fbc, err := declcfg.LoadFS(cmd.Context(), os.DirFS(args[0]))
			if err != nil {
				return fmt.Errorf("loading declarative config: %w", err)
			}
			g, err := graph.FromFBC(*fbc)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false)
			return enc.Encode(g)
		},
	}
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
