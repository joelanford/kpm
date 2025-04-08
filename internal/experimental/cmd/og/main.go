package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/renameio"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	action2 "github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/action/migrations"

	"github.com/joelanford/kpm/internal/action"
	"github.com/joelanford/kpm/internal/experimental/graph"
	"github.com/joelanford/kpm/internal/experimental/graph/write"
)

func main() {
	logrus.SetOutput(io.Discard)
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "og",
		Short: "OLM Upgrade Graph CLI",
	}

	rootCmd.AddCommand(
		newAddCmd(),
		newTagCmd(),
		newQueryCmd(),
	)

	return rootCmd
}

func newAddCmd() *cobra.Command {
	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add nodes or edges to the graph",
	}

	addCmd.AddCommand(
		newAddFBCCmd(),
		newAddNodeCmd(),
		newAddEdgeCmd(),
	)
	return addCmd
}

func newAddFBCCmd() *cobra.Command {
	var graphDir string
	var tags []string

	cmd := &cobra.Command{
		Use:   "fbc <fbcReference> -g <graphDir>",
		Short: "Initialize an upgrade graph directory",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			fbcReference := args[0]
			tagMap := parseTags(tags)

			idx, err := openIndex(graphDir, false)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			fbcRefMask := action2.RefDCImage | action2.RefDCDir | action2.RefSqliteFile
			if err := addReferences(cmd.Context(), idx, fbcRefMask, tagMap, fbcReference); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			if err := saveIndex(idx, graphDir); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVarP(&graphDir, "graph-dir", "g", ".", "Graph directory")
	cmd.Flags().StringArrayVarP(&tags, "tag", "t", nil, "Tag to attach to node(s), e.g. --tag key=value")

	return cmd
}

func newAddNodeCmd() *cobra.Command {
	var graphDir string
	var tags []string

	cmd := &cobra.Command{
		Use:   "node <bundleRef>...",
		Short: "Add bundle node(s) to the graph",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			bundleRefs := args
			tagMap := parseTags(tags)

			idx, err := openIndex(graphDir, false)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			fbcRefMask := action2.RefBundleImage
			if err := addReferences(cmd.Context(), idx, fbcRefMask, tagMap, bundleRefs...); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			if err := saveIndex(idx, graphDir); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVarP(&graphDir, "graph-dir", "g", ".", "Graph directory")
	cmd.Flags().StringArrayVarP(&tags, "tag", "t", nil, "Tag to attach to edge, e.g. --tag key=value")
	return cmd
}

func newAddEdgeCmd() *cobra.Command {
	var graphDir string
	var tags []string

	cmd := &cobra.Command{
		Use:   "edge <fromNodeExpression> <toNodeExpression>",
		Short: "Add edges to the graph",
		Long:  "Add edges to the graph for all combinations of from/to nodes that match the provided from/to expressions",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			fromExpr := args[0]
			toExpr := args[1]
			tagMap := parseTags(tags)

			fromSelector, err := graph.ParseSelector(fromExpr)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			toSelector, err := graph.ParseSelector(toExpr)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			idx, err := openIndex(graphDir, true)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			var (
				fromNodes []digest.Digest
				toNodes   []digest.Digest
			)
			for dgst, node := range idx.Graph().Nodes {
				matchFrom, err := fromSelector.MatchNode(cmd.Context(), idx.Graph(), dgst)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				matchTo, err := toSelector.MatchNode(cmd.Context(), idx.Graph(), dgst)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}

				if matchFrom == matchTo {
					fmt.Fprintf(os.Stdout, "invalid edge generated: from and to expressions matched node %v\n", node.NVR)
					os.Exit(1)
				}
				fromNodes = append(fromNodes, dgst)
				toNodes = append(toNodes, dgst)
			}
			for _, from := range fromNodes {
				for _, to := range toNodes {
					if _, err := idx.AddEdge(graph.NewEdge(from, to), tagMap); err != nil {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(1)
					}
				}
			}
		},
	}

	cmd.Flags().StringVarP(&graphDir, "graph-dir", "g", ".", "Graph directory")
	cmd.Flags().StringArrayVarP(&tags, "tag", "t", nil, "Tag to attach to edge, e.g. --tag key=value")
	return cmd
}

func newTagCmd() *cobra.Command {
	var graphDir string

	cmd := &cobra.Command{
		Use:   "tag <celMatchExpression> [key=value...]",
		Short: "Tag matching graph elements",
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			expr := args[0]
			tags := args[1:]
			tagMap := parseTags(tags)

			f, err := graph.ParseSelector(expr)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			idx, err := openIndex(graphDir, true)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			var matchingDigests []digest.Digest
			for dgst := range idx.Graph().Nodes {
				match, err := f.MatchNode(cmd.Context(), idx.Graph(), dgst)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				if match {
					matchingDigests = append(matchingDigests, dgst)
				}
			}
			for dgst := range idx.Graph().Edges {
				match, err := f.MatchEdge(cmd.Context(), idx.Graph(), dgst)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				if match {
					matchingDigests = append(matchingDigests, dgst)
				}
			}
			for _, dgst := range matchingDigests {
				for k, vals := range tagMap {
					for _, v := range vals {
						idx.AddTag(dgst, k, v)
					}
				}
			}

			if err := saveIndex(idx, graphDir); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVarP(&graphDir, "graph-dir", "g", ".", "Graph directory")
	return cmd
}

func newQueryCmd() *cobra.Command {
	var (
		graphDir string
		filter   string
		output   string
	)
	cmd := &cobra.Command{
		Use:  "query",
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true

			idx, err := openIndex(graphDir, true)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			if filter != "" {
				f, err := graph.ParseSelector(filter)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				if err := f.Apply(cmd.Context(), idx); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
			}

			switch output {
			case "json":
				err = write.JSON(os.Stdout, idx.Graph())
			case "mermaid":
				err = write.Mermaid(os.Stdout, idx.Graph())
			case "mermaidurl":
				err = write.MermaidURL(os.Stdout, idx.Graph())
			default:
				fmt.Fprintf(os.Stderr, "unknown output format %q", output)
				os.Exit(1)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
	}
	cmd.Flags().StringVarP(&graphDir, "graph-dir", "g", ".", "Graph directory")
	cmd.Flags().StringVarP(&filter, "filter", "f", "", "tag-based CEL expression filter to apply to the graph")
	cmd.Flags().StringVarP(&output, "output", "o", "json", "output format (one of: [json,mermaid,mermaidurl])")
	return cmd
}

func parseTags(tagPairs []string) map[string][]string {
	tagMap := map[string][]string{}
	for _, pair := range tagPairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue // skip invalid tags
		}
		key, value := parts[0], parts[1]
		tagMap[key] = append(tagMap[key], value)
	}
	return tagMap
}

func openIndex(graphDir string, requireExists bool) (*graph.Index, error) {
	graphFile := filepath.Join(graphDir, "graph.json")
	g := graph.NewGraph()
	exists := false
	if s, err := os.Stat(graphFile); err == nil && !s.IsDir() {
		exists = true
		graphData, err := os.ReadFile(graphFile)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(graphData, g); err != nil {
			return nil, err
		}
	}
	if !exists && requireExists {
		return nil, fmt.Errorf("graph file %q does not exist", graphFile)
	}
	idx := graph.NewIndexFromGraph(g)
	return idx, nil
}

func saveIndex(idx *graph.Index, graphDir string) error {
	graphFile := filepath.Join(graphDir, "graph.json")
	cleanupGraphDir := func() error { return nil }
	if _, err := os.Stat(graphDir); os.IsNotExist(err) {
		if err := os.MkdirAll(graphDir, 0755); err != nil {
			return err
		}
		cleanupGraphDir = func() error { return os.RemoveAll(graphDir) }
	}

	t, err := renameio.TempFile(graphDir, graphFile)
	if err != nil {
		return errors.Join(err, cleanupGraphDir())
	}
	defer t.Cleanup()

	if err := write.JSON(t, idx.Graph()); err != nil {
		return errors.Join(err, cleanupGraphDir())
	}
	if err := t.CloseAtomicallyReplace(); err != nil {
		return errors.Join(err, cleanupGraphDir())
	}
	return nil
}

func addReferences(ctx context.Context, idx *graph.Index, allowedRefMask action2.RefType, tagMap map[string][]string, refs ...string) error {
	m, err := migrations.NewMigrations(migrations.AllMigrations)
	if err != nil {
		return err
	}

	r := action.Render{
		Migrations:     m,
		AllowedRefMask: allowedRefMask,
	}
	for _, ref := range refs {
		fbc, err := r.Render(ctx, ref)
		if err != nil {
			return err
		}
		if err := idx.AddFBC(*fbc, tagMap); err != nil {
			return err
		}
	}
	return nil
}
