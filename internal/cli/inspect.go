package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/pkg/docker/config"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"

	"github.com/joelanford/kpm/action"
	"github.com/joelanford/kpm/internal/console"
)

func Inspect() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "inspect <package-reference>",
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			if err := runInspect(ctx, runInspectOptions{
				packageRef: args[0],
			}); err != nil {
				console.Fatalf(1, "💥 %s", err)
			}
		},
	}
	return cmd
}

type runInspectOptions struct {
	packageRef string
}

func runInspect(ctx context.Context, opts runInspectOptions) error {
	if _, err := os.Stat(opts.packageRef); err != nil {
		return inspectRegistry(ctx, opts)
	} else {
		return inspectFile(ctx, opts)
	}
}

func inspectRegistry(ctx context.Context, opts runInspectOptions) error {
	ref, err := reference.ParseNamed(opts.packageRef)
	if err != nil {
		return err
	}
	tag := ""
	switch r := ref.(type) {
	case reference.NamedTagged:
		tag = r.Tag()
	case reference.Digested:
		tag = r.Digest().String()
	default:
		return fmt.Errorf("invalid kpm reference: expected tag or digest")
	}

	repo, err := remote.NewRepository(ref.Name())
	if err != nil {
		return err
	}
	repo.Client = &auth.Client{Credential: func(ctx context.Context, _ string) (auth.Credential, error) {
		dockerAuthConfig, err := config.GetCredentialsForRef(nil, ref)
		if err != nil {
			return auth.Credential{}, err
		}
		return auth.Credential{
			Username:     dockerAuthConfig.Username,
			Password:     dockerAuthConfig.Password,
			RefreshToken: dockerAuthConfig.IdentityToken,
		}, nil
	}}

	inspect := action.Inspect{
		Target: repo,
		Tag:    tag,
		Output: os.Stdout,
	}
	return inspect.Run(ctx)
}

func inspectFile(ctx context.Context, opts runInspectOptions) error {
	store, err := oci.NewFromTar(ctx, opts.packageRef)
	if err != nil {
		return err
	}
	tag := ""
	if err := store.Tags(ctx, "", func(tags []string) error {
		numTags := len(tags)
		if numTags == 0 || numTags > 1 {
			return fmt.Errorf("invalid kpm file: expected exactly one tag, found %d", numTags)
		}
		tag = tags[0]
		return nil
	}); err != nil {
		return err
	}

	inspect := action.Inspect{
		Target: store,
		Tag:    tag,
		Output: os.Stdout,
	}
	return inspect.Run(ctx)
}
