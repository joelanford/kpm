package v2

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/blang/semver/v4"
	"golang.org/x/crypto/openpgp/clearsign"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"

	bundlev1alpha1 "github.com/joelanford/kpm/internal/api/bundle/v1alpha1"
)

type Package struct {
	archiveFile string
	id          bundlev1alpha1.ID

	archiveData []byte
	chrt        *chart.Chart

	provenanceData []byte
}

func LoadPackage(archiveFile string) (*Package, error) {
	p := &Package{archiveFile: archiveFile}
	for _, fn := range []func() error{
		p.load,
		p.validate,
		p.complete,
	} {
		if err := fn(); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (p *Package) load() error {
	if err := do(
		p.loadArchiveFile,
		p.loadProvenanceFile,
	); err != nil {
		return fmt.Errorf("failed to load package %q: %w", p.archiveFile, err)
	}
	return nil
}

func (p *Package) loadArchiveFile() error {
	archiveData, err := os.ReadFile(p.archiveFile)
	if err != nil {
		return fmt.Errorf("failed to open chart archive: %w", err)
	}

	ch, err := loader.LoadArchive(bytes.NewReader(archiveData))
	if err != nil {
		return fmt.Errorf("failed to load chart archive: %w", err)
	}
	p.chrt = ch
	p.archiveData = archiveData
	return nil
}

func (p *Package) loadProvenanceFile() error {
	provenanceData, err := os.ReadFile(fmt.Sprintf("%s.prov", p.archiveFile))
	if err != nil {
		return fmt.Errorf("failed to read provenance file: %w", err)
	}
	p.provenanceData = provenanceData
	return nil
}

func (p *Package) validate() error {
	if err := do(
		p.validateChart,
		p.validateProvenance,
	); err != nil {
		return fmt.Errorf("failed to validate chart: %v", err)
	}
	return nil
}

func (p *Package) validateChart() error {
	if err := p.chrt.Validate(); err != nil {
		return fmt.Errorf("failed to validate chart: %w", err)
	}
	return nil
}

func (p *Package) validateProvenance() error {
	block, _ := clearsign.Decode(p.provenanceData)
	if block == nil {
		return fmt.Errorf("failed to validate provenance file: signature block not found")
	}
	return nil
}

func (p *Package) complete() error {
	if err := do(
		p.populateID,
	); err != nil {
		return err
	}
	return nil
}

func (p *Package) populateID() error {
	verStr := p.chrt.Metadata.Version
	ver, err := semver.Parse(verStr)
	if err != nil {
		return fmt.Errorf("failed to parse chart version %q as semver: %w", verStr, err)
	}
	p.id = bundlev1alpha1.NewID(
		p.chrt.Metadata.Name,
		ver,
		bundlev1alpha1.MustParseRelease("0"),
	)
	return nil
}

func do(funcs ...func() error) error {
	var errs []error
	for _, fn := range funcs {
		errs = append(errs, fn())
	}
	return errors.Join(errs...)
}
