package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joelanford/kpm/action"
	"github.com/joelanford/kpm/internal/console"
	"github.com/joelanford/kpm/internal/remote"
	kpmoci "github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var _ pflag.Value = (*destination)(nil)

type destination struct {
	transport string
	ref       string
}

func (d *destination) String() string {
	if d == nil || (d.transport == "" && d.ref == "") {
		return ""
	}
	return fmt.Sprintf("%s:%s", d.transport, d.ref)
}

func (d *destination) Set(s string) error {
	transport, ref, ok := strings.Cut(s, ":")
	if !ok {
		return fmt.Errorf("invalid destination %q: expected format <transport>:<reference>", s)
	}
	switch transport {
	case "oci-archive":
	case "docker":
		ref = strings.TrimPrefix(ref, "//")
	default:
		return fmt.Errorf("unsupported transport %q", transport)
	}
	d.transport = transport
	d.ref = ref
	return nil
}

func (d *destination) Type() string {
	return "string"
}

func (d *destination) bindSelfRequired(cmd *cobra.Command) {
	cmd.Flags().Var(d, "destination", "destination for the bundle (e.g., oci-archive:/path/to/bundle or docker://myrepo/mybundle)")
	cmd.MarkFlagRequired("destination")
}

func (d *destination) push(ctx context.Context, buildFunc func(context.Context, action.PushFunc) (string, ocispec.Descriptor, error)) error {
	var pushFunc action.PushFunc
	var cleanup = func() error { return nil }
	var log = func(tag string, desc ocispec.Descriptor) {}

	switch d.transport {
	case "oci-archive":
		outputWriter, err := os.Create(d.ref)
		if err != nil {
			return err
		}
		defer outputWriter.Close()

		pushFunc = action.Write(outputWriter)
		cleanup = func() error { return os.Remove(d.ref) }
		log = func(_ string, _ ocispec.Descriptor) {
			console.Primaryf("📦 %s created!", d.ref)
		}
	case "docker":
		pushRepo, err := remote.NewRepository(d.ref)
		if err != nil {
			return err
		}
		pushFunc = action.Push(pushRepo, kpmoci.PushOptions{})
		log = func(tag string, desc ocispec.Descriptor) {
			console.Primaryf("📦 Successfully pushed bundle \n    🏷️%s:%s\n    📍 %s@%s!", d.ref, tag, d.ref, desc.Digest.String())
		}
	}

	tag, desc, err := buildFunc(ctx, pushFunc)
	if err != nil {
		return errors.Join(err, cleanup())
	}

	log(tag, desc)
	return nil
}
