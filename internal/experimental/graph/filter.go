package graph

import (
	"context"
	"fmt"
	"slices"

	"github.com/gogo/protobuf/proto"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"github.com/opencontainers/go-digest"

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

func getEntryTags(tags map[digest.Digest]map[string][]string, dgst digest.Digest) map[string]*pb.TagValues {
	itemTags, ok := tags[dgst]
	if !ok {
		return nil
	}

	entryTags := make(map[string]*pb.TagValues)
	for k, values := range itemTags {
		vals := &pb.TagValues{}
		for _, v := range values {
			vals.Values = append(vals.Values, v)
		}
		entryTags[k] = vals
	}
	return entryTags
}

func (f *Selector) Apply(ctx context.Context, i *Index) error {
	refNodes := map[digest.Digest]struct{}{}
	for dgst, edge := range i.graph.Edges {
		from := i.graph.Nodes[edge.From].NVR
		to := i.graph.Nodes[edge.To].NVR

		env := map[string]any{
			"entry": &pb.Entry{
				Tags: getEntryTags(i.graph.Tags, dgst),
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
		if out.Value().(bool) {
			refNodes[edge.From] = struct{}{}
			refNodes[edge.To] = struct{}{}
		} else {
			delete(i.graph.Edges, dgst)
			delete(i.graph.Tags, dgst)
		}
	}

	for dgst, node := range i.graph.Nodes {
		env := map[string]any{
			"entry": &pb.Entry{
				Tags: getEntryTags(i.graph.Tags, dgst),
				Kind: &pb.Entry_Node{Node: &pb.Node{
					Name:    node.Name,
					Version: node.Version.String(),
					Release: node.Release,
				}},
			},
		}
		out, _, err := f.Expression.ContextEval(ctx, env)
		if err != nil {
			return err
		}
		if !out.Value().(bool) {
			delete(i.graph.Nodes, dgst)
			delete(i.graph.Tags, dgst)
			i.nvrToNodes[node.NVR] = slices.DeleteFunc(i.nvrToNodes[node.NVR], func(d digest.Digest) bool {
				return d == dgst
			})
			if _, ok := refNodes[dgst]; ok {
				i.graph.ReferenceOnlyNodes[dgst] = node
			}
		}
	}
	return nil
}
