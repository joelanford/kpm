package main

import (
	"fmt"
	"maps"
	"os"

	"github.com/spf13/cobra"
	"go.starlark.net/starlark"

	"github.com/joelanford/kpm/internal/experimental/cmd/bbs/starlark/module/v1alpha1"
)

func main() {
	var (
		systemDir string
		exprs     []string
	)

	cmd := cobra.Command{
		Use:  "bbs <kpmspecStarlarkFile>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			// Sandbox and print redirection
			globals := starlark.StringDict{}
			thread := &starlark.Thread{
				Name: args[0],
				Print: func(_ *starlark.Thread, msg string) {
					fmt.Printf("[kpm][%s] %s\n", args[0], msg)
				},
				Load: func(thread *starlark.Thread, module string) (starlark.StringDict, error) {
					switch module {
					case "@kpm/v1alpha1/cfg":
						return v1alpha1.LoadAllConfig(thread, globals)
					case "@kpm/v1alpha1/registry_v1":
						return v1alpha1.RegistryV1()
					}
					return nil, fmt.Errorf("unknown module %q", module)
				},
			}

			results, err := starlark.ExecFile(thread, args[0], nil, globals)
			if err != nil {
				return err
			}
			for k, v := range results {
				fmt.Println(k, "=", v)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&systemDir, "kpmdefs", "./kpmdefs.d", "directory containing KPM build definitions")
	cmd.Flags().StringSliceVarP(&exprs, "expr", "e", nil, "expression(s) to evaluate")
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func loadExpressions(expressions []string, thread *starlark.Thread, predeclared starlark.StringDict) (starlark.StringDict, error) {
	defs := maps.Clone(predeclared)
	for i, expr := range expressions {
		results, err := starlark.ExecFile(thread, fmt.Sprintf("<expr[%d]>", i), expr, defs)
		if err != nil {
			return nil, err
		}
		maps.Copy(defs, results)
	}
	return defs, nil
}
