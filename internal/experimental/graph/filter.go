package graph

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"github.com/opencontainers/go-digest"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
)

func ParseFilter(in string) (*TagSelector, error) {
	scope, celExpression, ok := strings.Cut(in, ":")
	if !ok {
		return nil, fmt.Errorf(`invalid filter %q: format is <scope>:<key>=<value>`, in)
	}

	envOpts := []cel.EnvOption{
		cel.Declarations(
			decls.NewVar("tag", decls.NewMapType(decls.String, decls.String)),
		),
	}

	e, err := cel.NewEnv(envOpts...)
	if err != nil {
		return nil, err
	}
	ast, issues := e.Compile(celExpression)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}

	if !ast.OutputType().IsExactType(types.BoolType) {
		return nil, fmt.Errorf(`invalid filter %q: output type is %s, expected bool`, in, ast.OutputType().String())
	}

	prg, err := e.Program(ast, cel.InterruptCheckFrequency(1024))
	if err != nil {
		return nil, err
	}

	return &TagSelector{
		Scope:      scope,
		Expression: prg,
	}, nil
}

type TagSelector struct {
	Scope      string
	Expression cel.Program
}

func (f *TagSelector) Apply(ctx context.Context, g *graphv1.Graph) error {
	keepDigests := map[digest.Digest]struct{}{}
	for _, tag := range g.Tags {
		if f.Scope == tag.Scope || f.Scope == "*" {
			env := map[string]any{
				"tag": map[string]string{
					tag.Key: tag.Value,
				},
			}
			out, _, err := f.Expression.ContextEval(ctx, env)
			if err != nil {
				return err
			}
			if out.Value().(bool) {
				keepDigests[tag.Reference] = struct{}{}
			}
		}
	}

	deleteUnusedMetadata(g, f.Scope, keepDigests)
	return nil
}

func deleteUnusedMetadata(g *graphv1.Graph, scope string, keepDigests map[digest.Digest]struct{}) {
	nodeScope := scope == graphv1.ScopeNode || scope == "*"
	edgeScope := scope == graphv1.ScopeEdge || scope == "*"

	referencedNodes := map[digest.Digest]struct{}{}
	if edgeScope {
		// Delete non-matching edges
		maps.DeleteFunc(g.Edges, func(digest digest.Digest, edge graphv1.Edge) bool {
			_, keep := keepDigests[digest]
			if keep {
				referencedNodes[edge.From] = struct{}{}
				referencedNodes[edge.To] = struct{}{}
			}
			return !keep
		})
	}

	if nodeScope {
		// Move non-matching nodes to "ReferenceOnlyNodes"
		for nodeDigest, node := range g.Nodes {
			if _, keep := keepDigests[nodeDigest]; !keep {
				delete(g.Nodes, nodeDigest)
				if _, referenced := referencedNodes[nodeDigest]; referenced {
					if g.ReferenceOnlyNodes == nil {
						g.ReferenceOnlyNodes = map[digest.Digest]graphv1.Node{}
					}
					g.ReferenceOnlyNodes[nodeDigest] = node
				}
			}
		}
	}

	// Delete node tags matching the scope that now have 0 matches
	maps.DeleteFunc(g.Tags, func(_ digest.Digest, tag graphv1.Tag) bool {
		inScope := scope == tag.Scope || scope == "*"
		_, foundNode := g.Nodes[tag.Reference]
		_, foundEdge := g.Edges[tag.Reference]
		return inScope && !foundNode && !foundEdge
	})
}
