package v2

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"golang.org/x/crypto/openpgp/clearsign"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

type Chart struct {
	archiveFile string

	archiveData []byte
	chrt        *chart.Chart

	provenanceData []byte
}

func LoadPackage(archiveFile string) (*Chart, error) {
	p := &Chart{archiveFile: archiveFile}
	for _, fn := range []func() error{
		p.load,
		p.validate,
	} {
		if err := fn(); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (ch *Chart) load() error {
	if err := do(
		ch.loadArchiveFile,
		ch.loadProvenanceFile,
	); err != nil {
		return fmt.Errorf("failed to load package %q: %w", ch.archiveFile, err)
	}
	return nil
}

func (ch *Chart) loadArchiveFile() error {
	archiveData, err := os.ReadFile(ch.archiveFile)
	if err != nil {
		return fmt.Errorf("failed to open chart archive: %w", err)
	}

	chrt, err := loader.LoadArchive(bytes.NewReader(archiveData))
	if err != nil {
		return fmt.Errorf("failed to load chart archive: %w", err)
	}
	ch.chrt = chrt
	ch.archiveData = archiveData
	return nil
}

func (ch *Chart) loadProvenanceFile() error {
	provenanceData, err := os.ReadFile(fmt.Sprintf("%s.prov", ch.archiveFile))
	if err != nil {
		return fmt.Errorf("failed to read provenance file: %w", err)
	}
	ch.provenanceData = provenanceData
	return nil
}

func (ch *Chart) validate() error {
	if err := do(
		ch.validateChart,
		ch.validateProvenance,
	); err != nil {
		return fmt.Errorf("failed to validate chart: %v", err)
	}
	return nil
}

func (ch *Chart) validateChart() error {
	if err := ch.chrt.Validate(); err != nil {
		return fmt.Errorf("failed to validate chart: %w", err)
	}
	return nil
}

func (ch *Chart) validateProvenance() error {
	block, _ := clearsign.Decode(ch.provenanceData)
	if block == nil {
		return fmt.Errorf("failed to validate provenance file: signature block not found")
	}
	return nil
}

func do(funcs ...func() error) error {
	var errs []error
	for _, fn := range funcs {
		errs = append(errs, fn())
	}
	return errors.Join(errs...)
}
