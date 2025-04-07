package main

import (
	"encoding/json"
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

			if filter != "" {
				f, err := graph.ParseFilter(filter)
				if err != nil {
					return err
				}
				if err := f.Apply(cmd.Context(), &g); err != nil {
					return err
				}
			}

			switch output {
			case "json":
				err = write.JSON(os.Stdout, &g)
			case "mermaid":
				err = write.Mermaid(os.Stdout, &g)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "tag-based CEL expression filter to apply to the graph")
	cmd.Flags().StringVarP(&output, "output", "o", "json", "output format (one of: [json,mermaid])")
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
