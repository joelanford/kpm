package v1alpha1

import (
	"github.com/joelanford/kpm/internal/experimental/cmd/bbs/starlark/module"
	"maps"

	"go.starlark.net/starlark"
)

const (
	version = "v1alpha1"
	name    = "cfg.star"
)

func LoadSystemConfig(thread *starlark.Thread, predeclared starlark.StringDict) (starlark.StringDict, error) {
	return starlark.ExecFile(thread, module.SystemPath(version, name), nil, predeclared)
}

func LoadUserConfig(thread *starlark.Thread, predeclared starlark.StringDict) (starlark.StringDict, error) {
	return starlark.ExecFile(thread, module.UserPath(version, name), nil, predeclared)
}

func LoadAllConfig(thread *starlark.Thread, predeclared starlark.StringDict) (starlark.StringDict, error) {
	defs := maps.Clone(predeclared)
	system, err := LoadSystemConfig(thread, defs)
	if err != nil {
		return nil, err
	}
	defs = mergeMaps(defs, system)

	user, err := LoadUserConfig(thread, defs)
	if err != nil {
		return nil, err
	}
	defs = mergeMaps(defs, user)

	defMap := starlark.NewDict(len(defs))
	for k, v := range defs {
		if err := defMap.SetKey(starlark.String(k), v); err != nil {
			return nil, err
		}
	}

	return starlark.StringDict{
		"cfg": defMap,
	}, nil
}

func mergeMaps(a, b starlark.StringDict) starlark.StringDict {
	result := maps.Clone(a)
	maps.Copy(result, b)
	return result
}
