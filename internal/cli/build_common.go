package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/joelanford/kpm/action"
	"github.com/joelanford/kpm/internal/console"
	"github.com/joelanford/kpm/internal/remote"
	kpmoci "github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type destination interface {
	pushFunc() (action.PushFunc, error)
	logSuccessFunc() func(string, ocispec.Descriptor)
}

func newDestination(s string) (destination, error) {
	transport, ref, ok := strings.Cut(s, ":")
	if !ok {
		return nil, fmt.Errorf("invalid destination %q: expected format <transport>:<reference>", s)
	}
	switch transport {
	case "oci-archive":
		return &ociArchiveDestination{ref: ref}, nil
	case "docker":
		ref = strings.TrimPrefix(ref, "//")
		return &dockerDestination{ref: ref}, nil
	default:
		return nil, fmt.Errorf("unsupported transport %q", transport)
	}
}

//func (d *destination) String() string {
//	if d == nil || (d.transport == "" && d.ref == "") {
//		return ""
//	}
//	return fmt.Sprintf("%s:%s", d.transport, d.ref)
//}
//
//func (d *destination) push(ctx context.Context, buildFunc func(context.Context, action.PushFunc) (string, ocispec.Descriptor, error)) error {
//	var pushFunc action.PushFunc
//	var cleanup = func() error { return nil }
//	var log = func(tag string, desc ocispec.Descriptor) {}
//
//	switch d.transport {
//	case "oci-archive":
//		outputWriter, err := os.Create(d.ref)
//		if err != nil {
//			return err
//		}
//		defer outputWriter.Close()
//
//		pushFunc = action.Write(outputWriter)
//		cleanup = func() error { return os.Remove(d.ref) }
//		log = func(_ string, _ ocispec.Descriptor) {
//			console.Primaryf("📦 %s created!", d.ref)
//		}
//	case "docker":
//		pushRepo, err := remote.NewRepository(d.ref)
//		if err != nil {
//			return err
//		}
//		pushFunc = action.Push(pushRepo, kpmoci.PushOptions{})
//		log = func(tag string, desc ocispec.Descriptor) {
//			console.Primaryf("📦 Successfully pushed bundle \n    🏷️%s:%s\n    📍 %s@%s", d.ref, tag, d.ref, desc.Digest.String())
//		}
//	}
//
//	tag, desc, err := buildFunc(ctx, pushFunc)
//	if err != nil {
//		return errors.Join(err, cleanup())
//	}
//
//	log(tag, desc)
//	return nil
//}

type dockerDestination struct {
	ref string
}

func (d *dockerDestination) pushFunc() (action.PushFunc, error) {
	pushRepo, err := remote.NewRepository(d.ref)
	if err != nil {
		return nil, err
	}
	console.Secondaryf("➡️ Pushing catalog to destination")
	return action.Push(pushRepo, kpmoci.PushOptions{}), nil
}

func (d *dockerDestination) logSuccessFunc() func(string, ocispec.Descriptor) {
	return func(tag string, desc ocispec.Descriptor) {
		console.Primaryf("📦 Successfully pushed image\n    🏷️%s:%s\n    📍 %s@%s", d.ref, tag, d.ref, desc.Digest.String())
	}
}

type ociArchiveDestination struct {
	ref string
}

func (d *ociArchiveDestination) pushFunc() (action.PushFunc, error) {
	outputWriter, err := os.Create(d.ref)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, artifact kpmoci.Artifact) (string, ocispec.Descriptor, error) {
		defer outputWriter.Close()
		console.Secondaryf("➡️ Pushing catalog to destination")
		tag, desc, err := action.Write(outputWriter)(ctx, artifact)
		if err != nil {
			os.Remove(d.ref)
		}
		return tag, desc, err
	}, nil
}

func (d *ociArchiveDestination) logSuccessFunc() func(string, ocispec.Descriptor) {
	return func(_ string, desc ocispec.Descriptor) {
		console.Primaryf("📦 Successfully created file\n    📄 %s\n    📍 %s", d.ref, desc.Digest)
	}
}
