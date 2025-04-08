package graph

import (
	"context"
	"fmt"
	"maps"

	"github.com/gogo/protobuf/proto"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"github.com/opencontainers/go-digest"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
	"github.com/joelanford/kpm/internal/experimental/api/graph/v1/pb"
)

type Selector struct {
	Expression cel.Program
}

func compileExpression(expr string, envOpts []cel.EnvOption) (cel.Program, error) {
	e, err := cel.NewEnv(envOpts...)
	if err != nil {
		return nil, err
	}
	ast, issues := e.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}

	if !ast.OutputType().IsExactType(types.BoolType) {
		return nil, fmt.Errorf(`invalid expression %q: output type is %s, expected bool`, expr, ast.OutputType().String())
	}

	prg, err := e.Program(ast, cel.InterruptCheckFrequency(1024))
	if err != nil {
		return nil, err
	}
	return prg, nil
}

func ParseSelector(in string) (*Selector, error) {
	envOpts := []cel.EnvOption{
		cel.Types(proto.Message(&pb.Entry{})),
		cel.Declarations(
			decls.NewVar("entry", decls.NewObjectType("pb.Entry")),
		),
	}
	prg, err := compileExpression(in, envOpts)
	if err != nil {
		return nil, err
	}

	return &Selector{
		Expression: prg,
	}, nil
}

func (f *Selector) Apply(ctx context.Context, g *graphv1.Graph) error {
	nodeTags := make(map[digest.Digest]map[string]*pb.TagValues)
	edgeTags := make(map[digest.Digest]map[string]*pb.TagValues)

	keepNodes := make(map[digest.Digest]struct{}, len(g.Nodes))
	keepEdges := make(map[digest.Digest]struct{}, len(g.Edges))
	for d := range g.Nodes {
		keepNodes[d] = struct{}{}
	}
	for d := range g.Edges {
		keepEdges[d] = struct{}{}
	}

	for _, tag := range g.Tags {
		switch tag.Scope {
		case graphv1.ScopeNode:
			tags, ok := nodeTags[tag.Reference]
			if !ok {
				tags = make(map[string]*pb.TagValues)
			}
			vals, ok := tags[tag.Key]
			if !ok {
				vals = &pb.TagValues{}
			}
			vals.Values = append(vals.Values, tag.Value)
			tags[tag.Key] = vals
			nodeTags[tag.Reference] = tags
		case graphv1.ScopeEdge:
			tags, ok := edgeTags[tag.Reference]
			if !ok {
				tags = make(map[string]*pb.TagValues)
			}
			vals, ok := tags[tag.Key]
			if !ok {
				vals = &pb.TagValues{}
			}
			vals.Values = append(vals.Values, tag.Value)
			tags[tag.Key] = vals
			edgeTags[tag.Reference] = tags
		}
	}

	for nodeDigest, tags := range nodeTags {
		nvr := g.Nodes[nodeDigest].NVR
		env := map[string]any{
			"entry": &pb.Entry{
				Tags: tags,
				Kind: &pb.Entry_Node{Node: &pb.Node{
					Name:    nvr.Name,
					Version: nvr.Version.String(),
					Release: nvr.Release,
				}},
			},
		}
		out, _, err := f.Expression.ContextEval(ctx, env)
		if err != nil {
			return err
		}
		if !out.Value().(bool) {
			delete(keepNodes, nodeDigest)
		}
	}
	for edgeDigest, tags := range edgeTags {
		edge := g.Edges[edgeDigest]
		from := g.Nodes[edge.From].NVR
		to := g.Nodes[edge.To].NVR

		env := map[string]any{
			"entry": &pb.Entry{
				Tags: tags,
				Kind: &pb.Entry_Edge{Edge: &pb.Edge{
					From: &pb.Node{
						Name:    from.Name,
						Version: from.Version.String(),
						Release: from.Release,
					},
					To: &pb.Node{
						Name:    to.Name,
						Version: to.Version.String(),
						Release: to.Release,
					},
				}},
			},
		}
		out, _, err := f.Expression.ContextEval(ctx, env)
		if err != nil {
			return err
		}
		if !out.Value().(bool) {
			delete(keepEdges, edgeDigest)
		}
	}

	deleteUnusedMetadata(g, keepNodes, keepEdges)
	return nil
}

func deleteUnusedMetadata(g *graphv1.Graph, keepNodes, keepEdges map[digest.Digest]struct{}) {
	referencedNodes := map[digest.Digest]struct{}{}

	// Delete non-matching edges, otherwise track referenced nodes.
	maps.DeleteFunc(g.Edges, func(edgeDigest digest.Digest, edge graphv1.Edge) bool {
		_, keep := keepEdges[edgeDigest]
		if keep {
			referencedNodes[edge.From] = struct{}{}
			referencedNodes[edge.To] = struct{}{}
		}
		return !keep
	})

	// Delete non-matching nodes (or move them to ReferenceOnlyNodes if still referenced by edges)
	for nodeDigest, node := range g.Nodes {
		if _, keep := keepNodes[nodeDigest]; !keep {
			delete(g.Nodes, nodeDigest)
			if _, referenced := referencedNodes[nodeDigest]; referenced {
				if g.ReferenceOnlyNodes == nil {
					g.ReferenceOnlyNodes = map[digest.Digest]graphv1.Node{}
				}
				g.ReferenceOnlyNodes[nodeDigest] = node
			}
		}
	}

	// Delete node tags matching the scope that now have 0 matches
	maps.DeleteFunc(g.Tags, func(_ digest.Digest, tag graphv1.Tag) bool {
		nodeScope := tag.Scope == graphv1.ScopeNode
		edgeScope := tag.Scope == graphv1.ScopeEdge
		_, foundNode := g.Nodes[tag.Reference]
		_, foundEdge := g.Edges[tag.Reference]
		return !(foundNode && nodeScope) && !(foundEdge && edgeScope)
	})
}
