package v1alpha1

import "go.starlark.net/starlark"

func RegistryV1() (starlark.StringDict, error) {
	buildFunc := starlark.NewBuiltin("build", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return nil, nil
	})
	bundleDirectoryFunc := starlark.NewBuiltin("bundle_directory", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return nil, nil
	})
	return starlark.StringDict{
		"build":            buildFunc,
		"bundle_directory": bundleDirectoryFunc,
	}, nil
}
