package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/containerd/containerd/images"
	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/docker/docker/pkg/jsonmessage"
	dockerprogress "github.com/docker/docker/pkg/progress"
	"github.com/docker/docker/pkg/streamformatter"
	"github.com/joelanford/kpm/internal/tar"
	"github.com/mattn/go-isatty"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
)

type PushOptions struct {
	ProgressWriter io.Writer
}

func Write(ctx context.Context, w io.Writer, a Artifact) (string, ocispec.Descriptor, error) {
	tmpDir, err := os.MkdirTemp("", "kpm-")
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}
	defer os.RemoveAll(tmpDir)

	tmpStore, err := oci.NewWithContext(ctx, tmpDir)
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}

	desc, err := Push(ctx, a, tmpStore, PushOptions{})
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}

	tag := a.Tag()
	if err := tmpStore.Tag(ctx, desc, tag); err != nil {
		return "", ocispec.Descriptor{}, err
	}

	if err := tar.Directory(w, os.DirFS(tmpDir)); err != nil {
		return "", ocispec.Descriptor{}, err
	}
	return tag, desc, nil
}

func Push(ctx context.Context, artifact Artifact, target oras.Target, opts PushOptions) (ocispec.Descriptor, error) {
	tmpDir, err := os.MkdirTemp("", "olm-oci-push-")
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("create temporary OCI staging directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	localStagingStore, err := oci.NewWithContext(ctx, tmpDir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("create OCI content store: %w", err)
	}
	desc, err := push(ctx, artifact, localStagingStore)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("stage artifact graph locally: %v", err)
	}

	if opts.ProgressWriter == nil || opts.ProgressWriter == io.Discard {
		if err := oras.CopyGraph(ctx, localStagingStore, target, desc, oras.CopyGraphOptions{Concurrency: runtime.NumCPU()}); err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("push artifact graph: %v", err)
		}
	} else {
		if err := CopyGraphWithProgress(ctx, localStagingStore, target, desc, opts.ProgressWriter); err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("push artifact graph: %v", err)
		}
	}

	return desc, nil
}

type orderedDesc struct {
	pos  int
	desc ocispec.Descriptor
}

func push(ctx context.Context, artifact Artifact, store oras.Target) (ocispec.Descriptor, error) {
	eg, egCtx := errgroup.WithContext(ctx)

	subArtifacts := artifact.SubArtifacts()
	blobs := artifact.Blobs()
	numLayers := len(subArtifacts) + len(blobs)
	layerDescChan := make(chan orderedDesc, numLayers)
	pushBlobs(egCtx, eg, layerDescChan, blobs, store, 0)
	pushSubArtifacts(egCtx, eg, layerDescChan, subArtifacts, store, len(blobs))

	configDescChan := make(chan orderedDesc, 1)
	if config := artifact.Config(); config != nil {
		pushBlobs(egCtx, eg, configDescChan, []Blob{config}, store, 0)
	} else {
		configDescChan <- orderedDesc{
			pos:  0,
			desc: ocispec.DescriptorEmptyJSON,
		}
	}

	if err := eg.Wait(); err != nil {
		return ocispec.Descriptor{}, err
	}
	close(layerDescChan)
	close(configDescChan)

	layerDescs := make([]ocispec.Descriptor, numLayers)
	for desc := range layerDescChan {
		layerDescs[desc.pos] = desc.desc
	}
	configDesc := <-configDescChan

	annotations, err := artifact.Annotations()
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("get annotations: %w", err)
	}

	var data []byte
	switch artifact.MediaType() {
	case ocispec.MediaTypeImageManifest:
		data, _ = json.Marshal(ocispec.Manifest{
			Versioned:    specs.Versioned{SchemaVersion: 2},
			MediaType:    artifact.MediaType(),
			ArtifactType: artifact.ArtifactType(),
			Config:       configDesc.desc,
			Layers:       layerDescs,
			Annotations:  annotations,
		})
	case images.MediaTypeDockerSchema2Manifest:
		var dockerLayers []distribution.Descriptor
		for _, desc := range layerDescs {
			dockerLayers = append(dockerLayers, distribution.Descriptor{
				MediaType:   desc.MediaType,
				Digest:      desc.Digest,
				Size:        desc.Size,
				URLs:        desc.URLs,
				Annotations: desc.Annotations,
				Platform:    desc.Platform,
			})
		}
		data, _ = json.Marshal(schema2.Manifest{
			Versioned: manifest.Versioned{
				MediaType:     artifact.MediaType(),
				SchemaVersion: 2,
			},
			Config: distribution.Descriptor{
				MediaType:   configDesc.desc.MediaType,
				Digest:      configDesc.desc.Digest,
				Size:        configDesc.desc.Size,
				URLs:        configDesc.desc.URLs,
				Annotations: configDesc.desc.Annotations,
				Platform:    configDesc.desc.Platform,
			},
			Layers: dockerLayers,
		})
	}

	desc := content.NewDescriptorFromBytes(artifact.MediaType(), data)
	desc.ArtifactType = artifact.ArtifactType()

	if err := pushIfNotExist(ctx, store, desc, bytes.NewBuffer(data)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push artifact %q with digest %s failed: %w", artifact.ArtifactType(), desc.Digest, err)
	}
	return desc, nil
}

func pushSubArtifacts(ctx context.Context, eg *errgroup.Group, descChan chan<- orderedDesc, subArtifacts []Artifact, store oras.Target, startIdx int) {
	for i, sa := range subArtifacts {
		i, sa := i, sa
		eg.Go(func() error {
			manifestDesc, err := push(ctx, sa, store)
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case descChan <- orderedDesc{pos: i + startIdx, desc: manifestDesc}:
			}
			return nil
		})
	}
}

func pushBlobs(ctx context.Context, eg *errgroup.Group, descChan chan<- orderedDesc, blobs []Blob, store oras.Target, startIdx int) {
	for i, blob := range blobs {
		blob := blob
		eg.Go(func() error {
			rc, err := blob.Data()
			if err != nil {
				return err
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return err
			}

			desc := content.NewDescriptorFromBytes(blob.MediaType(), data)
			if err := pushIfNotExist(ctx, store, desc, bytes.NewReader(data)); err != nil {
				return fmt.Errorf("push blob %q with digest %s failed: %w", desc.MediaType, desc.Digest, err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case descChan <- orderedDesc{pos: i + startIdx, desc: desc}:
				return nil
			}
		})
	}
}

func pushIfNotExist(ctx context.Context, store oras.Target, desc ocispec.Descriptor, r io.Reader) error {
	if err := store.Push(ctx, desc, r); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return err
	}
	return nil
}

func CopyGraphWithProgress(ctx context.Context, src content.ReadOnlyStorage, dst oras.Target, desc ocispec.Descriptor, progressWriter io.Writer) error {
	pr, pw := io.Pipe()

	var (
		fd    uintptr
		isTTY bool
	)
	if progressWriter != nil {
		if f, ok := progressWriter.(*os.File); ok {
			fd = f.Fd()
			isTTY = isatty.IsTerminal(fd)
		}
	}

	out := streamformatter.NewJSONProgressOutput(pw, !isTTY)
	progressStore := newProgressStore(src, out)
	errChan := make(chan error, 1)
	go func() {
		errChan <- jsonmessage.DisplayJSONMessagesStream(pr, os.Stdout, fd, isTTY, nil)
	}()
	opts := oras.CopyGraphOptions{
		Concurrency: runtime.NumCPU(),
		OnCopySkipped: func(ctx context.Context, desc ocispec.Descriptor) error {
			return out.WriteProgress(dockerprogress.Progress{
				ID:     idForDesc(desc),
				Action: "Artifact is up to date",
			})
		},
		PostCopy: func(_ context.Context, desc ocispec.Descriptor) error {
			return out.WriteProgress(dockerprogress.Progress{
				ID:      idForDesc(desc),
				Action:  "Complete",
				Current: desc.Size,
				Total:   desc.Size,
			})
		},
	}
	if err := oras.CopyGraph(ctx, progressStore, dst, desc, opts); err != nil {
		return fmt.Errorf("copy artifact graph for descriptor %q: %v", desc.Digest, err)
	}
	if err := pw.Close(); err != nil {
		return fmt.Errorf("close progress writer: %v", err)
	}
	if err := <-errChan; err != nil {
		return fmt.Errorf("display progress: %v", err)
	}
	return nil
}
