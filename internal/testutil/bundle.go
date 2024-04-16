package testutil

import (
	"io/fs"
	"testing/fstest"
)

func GenerateBundle(packageName, version, release string) fs.FS {
	csv := `---
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: ` + packageName + `.v` + version + `--` + release + `
spec:
  version: ` + version + `
`
	annotations := `annotations:
  operators.operatorframework.io.bundle.package.v1: ` + packageName + `
  operators.operatorframework.io.bundle.mediatype.v1: registry+v1
`
	if release != "" {
		annotations += `  operators.operatorframework.io.bundle.release.v1: ` + release + `
`
	}

	return fstest.MapFS{
		"manifests":                 &fstest.MapFile{Mode: fs.ModeDir | 0755},
		"manifests/csv.yaml":        &fstest.MapFile{Mode: 0644, Data: []byte(csv)},
		"metadata":                  &fstest.MapFile{Mode: fs.ModeDir | 0755},
		"metadata/annotations.yaml": &fstest.MapFile{Mode: 0644, Data: []byte(annotations)},
	}
}
