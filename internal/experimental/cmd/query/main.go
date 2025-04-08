package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
	"github.com/joelanford/kpm/internal/experimental/graph"
	"github.com/joelanford/kpm/internal/experimental/graph/write"
)

func main() {
	var filter string
	var output string
	cmd := cobra.Command{
		Use:  "query <graphFile>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			graphData, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}

			var g graphv1.Graph
			if err := json.Unmarshal(graphData, &g); err != nil {
				return err
			}
			idx := graph.NewIndexFromGraph(&g)

			if filter != "" {
				f, err := graph.ParseSelector(filter)
				if err != nil {
					return err
				}
				if err := f.Apply(cmd.Context(), idx); err != nil {
					return err
				}
			}

			switch output {
			case "json":
				err = write.JSON(os.Stdout, &g)
			case "mermaid":
				err = write.Mermaid(os.Stdout, &g)
			case "mermaidurl":
				err = write.MermaidURL(os.Stdout, &g)
			default:
				return fmt.Errorf("unknown output format %q", output)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "tag-based CEL expression filter to apply to the graph")
	cmd.Flags().StringVarP(&output, "output", "o", "json", "output format (one of: [json,mermaid,mermaidurl])")
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
