package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"regexp"

	"github.com/blang/semver/v4"
	"github.com/google/renameio/v2"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"

	"github.com/joelanford/kpm/internal/util/tar"
)

const releasePattern = `^[A-Za-z0-9]+([.+-][A-Za-z0-9]+)*$`

var releaseRegex = regexp.MustCompile(releasePattern)

func NewRelease(raw string) (*Release, error) {
	if !releaseRegex.MatchString(raw) {
		return nil, fmt.Errorf("invalid release %q: does not match pattern %s", raw, releasePattern)
	}
	return &Release{raw: raw}, nil
}

type Release struct {
	raw string
}

func (r Release) String() string {
	return r.raw
}

func (r Release) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

type Builder interface {
	Build(ctx context.Context) (*ID, Manifest, error)
}

type ID struct {
	Name    string         `json:"name"`
	Version semver.Version `json:"version"`
	Release Release        `json:"release"`
}

const (
	AnnotationName    = "io.operatorframework.kpm.name"
	AnnotationVersion = "io.operatorframework.kpm.version"
	AnnotationRelease = "io.operatorframework.kpm.release"
)

func (i ID) Annotations() map[string]string {
	return map[string]string{
		AnnotationName:    i.Name,
		AnnotationVersion: i.Version.String(),
		AnnotationRelease: i.Release.String(),
	}
}

func (i ID) Filename() string {
	return fmt.Sprintf("%s.v%s-%s.bundle.kpm", i.Name, i.Version.String(), i.Release.String())
}

func (i ID) UnqualifiedReference() string {
	return fmt.Sprintf("%s:v%s-%s", i.Name, i.Version.String(), i.Release.String())
}

type Report struct {
	ID         ID          `json:"id"`
	Image      ReportImage `json:"image"`
	OutputFile string      `json:"outputFile"`
}

type ReportImage struct {
	Descriptor ocispec.Descriptor `json:"descriptor"`
	Reference  string             `json:"reference"`
}

func (r Report) WriteFile(reportFile string) error {
	reportData, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %v", err)
	}
	if reportFile == "-" || reportFile == "/dev/stdout" {
		fmt.Println(string(reportData))
	}
	return renameio.WriteFile(reportFile, reportData, 0644)
}

type Manifest interface {
	ArtifactType() string
	Annotations() map[string]string
	Subject() *ocispec.Descriptor

	PushConfigAndLayers(context.Context, content.Pusher) (ocispec.Descriptor, []ocispec.Descriptor, error)
}

type kpmManifest struct {
	wrapped Manifest
	id      ID
}

func (i kpmManifest) ArtifactType() string {
	return i.wrapped.ArtifactType()
}

func (i kpmManifest) Annotations() map[string]string {
	wrappedAnnotations := i.wrapped.Annotations()
	annotations := make(map[string]string, len(wrappedAnnotations)+3)
	maps.Copy(annotations, wrappedAnnotations)
	maps.Copy(annotations, i.id.Annotations())
	return annotations
}

func (i kpmManifest) Subject() *ocispec.Descriptor {
	return i.wrapped.Subject()
}

func (i kpmManifest) PushConfigAndLayers(ctx context.Context, pusher content.Pusher) (ocispec.Descriptor, []ocispec.Descriptor, error) {
	return i.wrapped.PushConfigAndLayers(ctx, pusher)
}

var _ Manifest = &kpmManifest{}

func WriteKpmManifest(ctx context.Context, id ID, manifest Manifest) (*Report, error) {
	tmpDir, err := os.MkdirTemp("", "kpm-build-bundle-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	pusher, err := oci.NewWithContext(ctx, tmpDir)
	if err != nil {
		return nil, err
	}

	manifest = &kpmManifest{wrapped: manifest, id: id}
	config, layers, err := manifest.PushConfigAndLayers(ctx, pusher)
	if err != nil {
		return nil, err
	}

	man := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: manifest.ArtifactType(),
		Config:       config,
		Layers:       layers,
		Subject:      manifest.Subject(),
		Annotations:  manifest.Annotations(),
	}
	manData, err := json.Marshal(man)
	if err != nil {
		return nil, err
	}
	manDesc, err := oras.PushBytes(ctx, pusher, ocispec.MediaTypeImageManifest, manData)
	if err != nil {
		return nil, err
	}
	manDesc.Annotations = id.Annotations()

	tagRef := id.UnqualifiedReference()
	if err := pusher.Tag(ctx, manDesc, tagRef); err != nil {
		return nil, err
	}

	outputFile := id.Filename()
	pf, err := renameio.NewPendingFile(outputFile)
	if err != nil {
		return nil, err
	}
	defer pf.Cleanup()

	if err := tar.Directory(pf, os.DirFS(tmpDir)); err != nil {
		return nil, err
	}
	if err := pf.CloseAtomicallyReplace(); err != nil {
		return nil, err
	}

	return &Report{
		ID: id,
		Image: ReportImage{
			Descriptor: manDesc,
			Reference:  tagRef,
		},
		OutputFile: outputFile,
	}, nil
}
