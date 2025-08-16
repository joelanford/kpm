package v1

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"

	"sigs.k8s.io/yaml"

	specsv1 "github.com/joelanford/kpm/internal/api/specs/v1"
	gencsv "github.com/joelanford/kpm/internal/third_party/operator-sdk/generate/clusterserviceversion"
	"github.com/joelanford/kpm/internal/third_party/operator-sdk/generate/collector"
	"github.com/joelanford/kpm/internal/third_party/operator-sdk/generate/genutil"
	"github.com/joelanford/kpm/internal/third_party/operator-sdk/k8sutil"
)

type bundleGenerateLoader struct {
	fsys fs.FS
	conf specsv1.RegistryV1GenerateSource
}

func NewGenerateLoader(workingFS fs.FS, source specsv1.RegistryV1GenerateSource) BundleLoader {
	return &bundleGenerateLoader{fsys: workingFS, conf: source}
}

func (g *bundleGenerateLoader) Load() (*Bundle, error) {
	m := &collector.Manifests{}
	updateFromFSFile := func(m *collector.Manifests, manifestFilePath string) error {
		f, err := g.fsys.Open(manifestFilePath)
		if err != nil {
			return err
		}
		defer f.Close()
		return m.UpdateFromReader(f)
	}

	for _, manifestFileGlob := range g.conf.ManifestFiles {
		matches, err := fs.Glob(g.fsys, manifestFileGlob)
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			if err := updateFromFSFile(m, match); err != nil {
				return nil, err
			}
		}
	}

	if len(m.ClusterServiceVersions) != 1 {
		return nil, fmt.Errorf("expected exactly one base ClusterServiceVersion, found %d", len(m.ClusterServiceVersions))
	}

	relatedImages, err := genutil.FindRelatedImages(m)
	if err != nil {
		return nil, fmt.Errorf("failed to find related images: %w", err)
	}

	csvGen := gencsv.Generator{
		Collector:            m,
		OperatorName:         g.conf.PackageName,
		Annotations:          g.conf.CSV.Annotations,
		Version:              g.conf.CSV.Version,
		ExtraServiceAccounts: g.conf.CSV.ExtraServiceAccounts,
		RelatedImages:        relatedImages,
	}

	buf := &bytes.Buffer{}
	if err := csvGen.Generate(gencsv.WithWriter(buf)); err != nil {
		return nil, fmt.Errorf("failed to generate CSV: %w", err)
	}

	objs, err := genutil.GetManifestObjects(m, g.conf.CSV.ExtraServiceAccounts)
	if err != nil {
		return nil, err
	}

	var files manifestFiles
	csvFileName := fmt.Sprintf("%s.v%s.clusterserviceversion.yaml", g.conf.PackageName, g.conf.CSV.Version)
	csvFile, err := newManifestFileFromReader(buf, csvFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV data: %w", err)
	}
	files = append(files, *csvFile)

	for _, obj := range objs {
		objFilename := fmt.Sprintf("%s.%s.yaml", obj.GetName(), strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind))
		objData, err := k8sutil.GetObjectBytes(obj, yaml.Marshal)
		if err != nil {
			return nil, err
		}
		objFile, err := newManifestFileFromReader(bytes.NewReader(objData), objFilename)
		if err != nil {
			return nil, err
		}
		files = append(files, *objFile)
	}

	bundleManifests, err := files.toManifests()
	if err != nil {
		return nil, err
	}

	annotationsFile, err := NewYAMLValueFile[Annotations](annotationsFileName, Annotations{Annotations: map[string]string{
		annotationMediaType: mediaType,
		annotationManifests: manifestsDirectory,
		annotationMetadata:  metadataDirectory,
		annotationPackage:   g.conf.PackageName,
	}})
	if err != nil {
		return nil, err
	}

	bundleMetadata := &metadata{
		annotationsFile: *annotationsFile,
	}
	if err := bundleMetadata.validate(); err != nil {
		return nil, err
	}

	return &Bundle{
		manifests: bundleManifests,
		metadata:  bundleMetadata,
	}, nil
}
