package main

import (
	"encoding/json"
	"fmt"
	"log"
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

			fbcs := make(map[string]declcfg.DeclarativeConfig, len(args))
			for _, arg := range args {
				dist, fbcDir, ok := strings.Cut(arg, ":")
				if !ok {
					return fmt.Errorf("invalid argument %q", arg)
				}
				fbc, err := declcfg.LoadFS(cmd.Context(), os.DirFS(fbcDir))
				if err != nil {
					return fmt.Errorf("loading declarative config: %w", err)
				}
				fbcs[dist] = *fbc
			}

			g, err := graph.FromFBC(fbcs)
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
