package celfunc

import (
	"slices"

	"github.com/Masterminds/semver/v3"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"

	"github.com/joelanford/kpm/internal/experimental/api/graph/v1/pb"
)

func EntryInDistro() cel.EnvOption {
	return cel.Function("inDistro",
		cel.MemberOverload(
			"pbEntry_inDistro_string",
			[]*cel.Type{cel.ObjectType("pb.Entry"), cel.StringType},
			cel.BoolType,
			cel.FunctionBinding(func(args ...ref.Val) ref.Val {
				if len(args) != 2 {
					return types.NewErr("expected 2 arguments")
				}
				entry := args[0].Value().(*pb.Entry)
				distroName := args[1].Value().(string)

				distros, ok := entry.Tags["distro"]
				if !ok {
					return types.False
				}
				return types.Bool(slices.Contains(distros.Values, distroName))
			}),
		),
	)
}

func EntryInPackage() cel.EnvOption {
	return cel.Function("inPackage",
		cel.MemberOverload(
			"pbEntry_inPackage_string",
			[]*cel.Type{cel.ObjectType("pb.Entry"), cel.StringType},
			cel.BoolType,
			cel.FunctionBinding(func(args ...ref.Val) ref.Val {
				if len(args) != 2 {
					return types.NewErr("expected 2 arguments")
				}
				entry := args[0].Value().(*pb.Entry)
				pkgName := args[1].Value().(string)

				if e := entry.GetEdge(); e != nil {
					if e.To.Name == pkgName {
						return types.True
					}
				}
				if n := entry.GetNode(); n != nil {
					if n.Name == pkgName {
						return types.True
					}
				}
				return types.False
			}),
		),
	)
}

func EntryInChannel() cel.EnvOption {
	return cel.Function("inChannel",
		cel.MemberOverload(
			"pbEntry_inChannel_string",
			[]*cel.Type{cel.ObjectType("pb.Entry"), cel.StringType},
			cel.BoolType,
			cel.FunctionBinding(func(args ...ref.Val) ref.Val {
				if len(args) != 2 {
					return types.NewErr("expected 2 arguments")
				}
				entry := args[0].Value().(*pb.Entry)
				chName := args[1].Value().(string)

				channels, ok := entry.Tags["channel"]
				if !ok {
					return types.False
				}
				return types.Bool(slices.Contains(channels.Values, chName))
			}),
		),
	)
}

func SemverMatches() cel.EnvOption {
	return cel.Function("semverMatches",
		cel.Overload("semver_matches_string_string",
			[]*cel.Type{cel.StringType, cel.StringType},
			cel.BoolType,
			cel.BinaryBinding(func(lhs ref.Val, rhs ref.Val) ref.Val {
				versionStr, ok := lhs.(types.String)
				if !ok {
					return types.NewErr("expected string for version")
				}

				constraintStr, ok := rhs.(types.String)
				if !ok {
					return types.NewErr("expected string for constraint")
				}

				version, err := semver.NewVersion(string(versionStr))
				if err != nil {
					return types.NewErr("invalid semver version %q: %v", versionStr, err)
				}

				constraint, err := semver.NewConstraint(string(constraintStr))
				if err != nil {
					return types.NewErr("invalid semver constraint %q: %v", constraintStr, err)
				}
				return types.Bool(constraint.Check(version))
			}),
		),
	)
}
func EntryHasTag() cel.EnvOption {
	return cel.Function("hasTag",
		cel.MemberOverload(
			"pbEntry_hasTag_string_string",
			[]*cel.Type{cel.ObjectType("pb.Entry"), cel.StringType, cel.StringType},
			cel.BoolType,
			cel.FunctionBinding(func(args ...ref.Val) ref.Val {
				if len(args) != 3 {
					return types.NewErr("expected 3 arguments")
				}
				entry := args[0].Value().(*pb.Entry)
				key := args[1].Value().(string)
				value := args[2].Value().(string)
				values, ok := entry.Tags[key]
				if !ok {
					return types.False
				}
				return types.Bool(slices.Contains(values.Values, value))
			}),
		),
	)
}
