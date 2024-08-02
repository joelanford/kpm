package cli

import (
	"context"
	"errors"
	"fmt"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/console"
	"github.com/joelanford/kpm/internal/remote"
	kpmoci "github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func Push() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push <kpm-file>...",
		Short: "Push one or more kpm files to their origin repositories",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			if err := runPush(ctx, runPushOptions{
				filesToPush: args,
			}); err != nil {
				console.Fatalf(1, "%s", err)
			}
		},
	}
	return cmd
}

type runPushOptions struct {
	filesToPush []string
}

type pushMeta struct {
	filename   string
	repo       string
	tag        string
	descriptor ocispec.Descriptor
}

func runPush(ctx context.Context, opts runPushOptions) error {
	errChan := make(chan error, len(opts.filesToPush))
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(3)
	for _, file := range opts.filesToPush {
		// We will never return an error here because we want _all_ pushes to be attempted.
		// Instead, we will collect the errors and return them at the end.
		eg.Go(func() error {
			kpm, err := v1.LoadKPM(egCtx, file)
			if err != nil {
				errChan <- fmt.Errorf("failed to load file %q: %w", file, err)
				return nil
			}

			target, err := remote.NewRepository(kpm.OriginReference.String())
			if err != nil {
				errChan <- fmt.Errorf("failed to configure destination %q for %q: %w", kpm.OriginReference.String(), file, err)
				return nil
			}

			desc, err := kpmoci.Push(egCtx, kpm.Artifact, target, kpmoci.PushOptions{})
			if err != nil {
				errChan <- fmt.Errorf("failed to push %q to %q: %w", file, kpm.OriginReference.String(), err)
				return nil
			}

			if err := target.Tag(egCtx, desc, kpm.Artifact.Tag()); err != nil {
				errChan <- fmt.Errorf("failed to tag %q in %q: %w", file, kpm.OriginReference.String(), err)
				return nil
			}
			fmt.Printf("pushed %q to %s:%s (digest: %s)\n", file, kpm.OriginReference.String(), kpm.Artifact.Tag(), desc.Digest.Encoded())
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		// This should never happen because we are not returning errors from the goroutines.
		return err
	}
	close(errChan)

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
