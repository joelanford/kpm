package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"github.com/opencontainers/go-digest"
	"google.golang.org/protobuf/proto"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
	"github.com/joelanford/kpm/internal/experimental/api/graph/v1/pb"
	"github.com/joelanford/kpm/internal/experimental/graph/celfunc"
)

type Selector struct {
	Expression cel.Program
}

func ParseSelector(in string) (*Selector, error) {
	envOpts := []cel.EnvOption{
		celfunc.EntryInDistro(),
		celfunc.EntryInPackage(),
		celfunc.EntryInChannel(),
		celfunc.EntryHasTag(),
		celfunc.SemverMatches(),
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

func tagMapToEntryTags(m map[string][]string) map[string]*pb.TagValues {
	entryTags := make(map[string]*pb.TagValues)
	for k, values := range m {
		vals := &pb.TagValues{}
		for _, v := range values {
			vals.Values = append(vals.Values, v)
		}
		entryTags[k] = vals
	}
	return entryTags
}

func (f *Selector) MatchNode(ctx context.Context, g *graphv1.Graph, dgst digest.Digest) (bool, error) {
	n, ok := g.Nodes[dgst]
	if !ok {
		return false, errors.New("node not found")
	}
	env := map[string]any{
		"entry": &pb.Entry{
			Tags: tagMapToEntryTags(g.Tags[dgst]),
			Kind: &pb.Entry_Node{Node: &pb.Node{
				Name:    n.Name,
				Version: n.Version.String(),
				Release: n.Release,
			}},
		},
	}
	out, _, err := f.Expression.ContextEval(ctx, env)
	if err != nil {
		return false, err
	}
	return out.Value().(bool), nil
}

func (f *Selector) MatchEdge(ctx context.Context, g *graphv1.Graph, dgst digest.Digest) (bool, error) {
	e, ok := g.Edges[dgst]
	if !ok {
		return false, errors.New("edge not found")
	}

	from := g.Nodes[e.From].NVR
	to := g.Nodes[e.To].NVR

	env := map[string]any{
		"entry": &pb.Entry{
			Tags: tagMapToEntryTags(g.Tags[dgst]),
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
		return false, err
	}
	return out.Value().(bool), nil
}

//
//func (f *Selector) Apply(ctx context.Context, i *mem.Index) error {
//	refNodes := map[digest.Digest]struct{}{}
//	for dgst, edge := range i.graph.Edges {
//		match, err := f.MatchEdge(ctx, i.graph, dgst)
//		if err != nil {
//			return err
//		}
//		if match {
//			refNodes[edge.From] = struct{}{}
//			refNodes[edge.To] = struct{}{}
//		} else {
//			delete(i.graph.Edges, dgst)
//			delete(i.graph.Tags, dgst)
//		}
//	}
//
//	for dgst, node := range i.graph.Nodes {
//		match, err := f.MatchNode(ctx, i.graph, dgst)
//		if err != nil {
//			return err
//		}
//		if !match {
//			delete(i.graph.Nodes, dgst)
//			delete(i.graph.Tags, dgst)
//			i.nvrToNodes[node.NVR] = slices.DeleteFunc(i.nvrToNodes[node.NVR], func(d digest.Digest) bool {
//				return d == dgst
//			})
//			if _, ok := refNodes[dgst]; ok {
//				i.graph.ReferenceOnlyNodes[dgst] = node
//			}
//		}
//	}
//	return nil
//}
