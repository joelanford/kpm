package action

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/containers/image/v5/docker/reference"

	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/action/migrations"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/pkg/containertools"

	"github.com/joelanford/kpm/internal/experimental/kpm"
)

type Render struct {
	Migrations     *migrations.Migrations
	AllowedRefMask action.RefType
}

func (r *Render) Render(ctx context.Context, ref string) (*declcfg.DeclarativeConfig, error) {
	if filepath.Ext(ref) == ".kpm" {
		return r.renderKpm(ctx, ref)
	}
	stdRender := action.Render{
		Refs:           []string{ref},
		Migrations:     r.Migrations,
		AllowedRefMask: r.AllowedRefMask,
	}
	return stdRender.Run(ctx)
}

func (r *Render) renderKpm(ctx context.Context, kpmPath string) (*declcfg.DeclarativeConfig, error) {
	kpmFile, err := kpm.Open(ctx, kpmPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open kpm file %q: %w", kpmPath, err)
	}
	kpmContentRoot, err := kpmFile.Mount("")
	if err != nil {
		return nil, fmt.Errorf("failed to mount kpm file %q: %w", kpmPath, err)
	}
	defer os.RemoveAll(kpmContentRoot)

	configsDir, isFBCCatalog := kpmFile.Annotations[containertools.ConfigsLocationLabel]
	if isFBCCatalog {
		kpmContentRoot = filepath.Join(kpmContentRoot, configsDir)
	}

	canonicalRef, err := reference.WithDigest(reference.TrimNamed(kpmFile.Reference), kpmFile.Descriptor.Digest)
	if err != nil {
		return nil, fmt.Errorf("failed to create canonical reference: %w", err)
	}
	refTmpl := template.Must(template.New("image").Parse(canonicalRef.String()))
	stdRender := action.Render{
		Refs:             []string{kpmContentRoot},
		AllowedRefMask:   r.AllowedRefMask & (action.RefBundleDir | action.RefDCDir),
		ImageRefTemplate: refTmpl,
		Migrations:       r.Migrations,
	}
	return stdRender.Run(ctx)
}
