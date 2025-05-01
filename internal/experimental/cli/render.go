package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/operator-framework/operator-registry/alpha/action/migrations"
	"github.com/operator-framework/operator-registry/alpha/declcfg"

	"github.com/joelanford/kpm/internal/experimental/action"
)

func Render() *cobra.Command {
	var (
		migrationLevel string
		output         string
	)
	cmd := &cobra.Command{
		Use:   "render <kpm-file>",
		Short: "Render catalog metadata from one or more kpm files",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()

			m, err := migrations.NewMigrations(migrationLevel)
			if err != nil {
				cmd.PrintErrf("failed to configure migrations: %v\n", err)
				os.Exit(1)
			}

			r := action.Render{
				Migrations: m,
			}

			var writeFunc func(declcfg.DeclarativeConfig, io.Writer) error
			switch output {
			case "json":
				writeFunc = declcfg.WriteJSON
			case "yaml":
				writeFunc = declcfg.WriteYAML
			default:
				cmd.PrintErrf("invalid output format %q\n", output)
			}

			for _, ref := range args {
				fbc, err := r.Render(ctx, ref)
				if err != nil {
					cmd.PrintErrf("failed to render %s: %v\n", ref, err)
					os.Exit(1)
				}
				if err := writeFunc(*fbc, os.Stdout); err != nil {
					cmd.PrintErrf("failed to write metadata: %v\n", err)
					os.Exit(1)
				}
			}
		},
	}
	cmd.Flags().StringVar(&migrationLevel, "migration-level", migrations.NoMigrations, "migration level to use when rendering metadata\n"+migrations.HelpText())
	cmd.Flags().StringVarP(&output, "output", "o", "json", "output format for rendered metadata (json or yaml)")
	return cmd
}
