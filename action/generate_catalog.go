package action

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing/fstest"
	"time"

	"github.com/Masterminds/semver/v3"
	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/fsutil"
	"github.com/joelanford/kpm/internal/registryv1"
	"github.com/joelanford/kpm/oci"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

type GenerateCatalog struct {
	Bundles []v1.KPM
	Tag     string
}

func (a *GenerateCatalog) Run(_ context.Context) (fs.FS, error) {
	if len(a.Bundles) == 0 {
		return nil, fmt.Errorf("no bundles provided")
	}
	return a.generate()
}

type bundleMetadata struct {
	reference   string
	packageName string
	version     semver.Version
	release     string
}

func (i bundleMetadata) Compare(j bundleMetadata) int {
	comparison := i.version.Compare(&j.version)
	if comparison != 0 {
		return comparison
	}
	return strings.Compare(i.release, j.release)
}

func (i bundleMetadata) String() string {
	release := i.release
	if i.release == "" {
		release = "0"
	}
	return fmt.Sprintf("%s.v%s-%s", i.packageName, i.version.String(), release)
}

func (a *GenerateCatalog) generate() (fs.FS, error) {
	packages := map[string]map[bundleMetadata]oci.Artifact{}
	for _, bundle := range a.Bundles {
		annotations, err := bundle.Artifact.Annotations()
		if err != nil {
			return nil, fmt.Errorf("failed to get annotations for bundle with tag %q: %w", bundle.Artifact.Tag(), err)
		}
		if mediaType, foundMediaType := annotations["operators.operatorframework.io.bundle.mediatype.v1"]; !foundMediaType || mediaType != "registry+v1" {
			return nil, fmt.Errorf("bundle with tag %q is not a catalog bundle", bundle.Artifact.Tag())
		}
		packageName, foundPackageName := annotations["operators.operatorframework.io.bundle.package.v1"]
		if !foundPackageName {
			return nil, fmt.Errorf("failed to find package name for bundle with tag %q", bundle.Artifact.Tag())
		}
		release, releaseFound := annotations["operators.operatorframework.io.bundle.release.v1"]
		if !releaseFound {
			release = "0"
		}
		version, err := getBundleVersion(bundle.Artifact, annotations["operators.operatorframework.io.bundle.manifests.v1"])
		if err != nil {
			return nil, fmt.Errorf("failed to get version for bundle with tag %q: %w", bundle.Artifact.Tag(), err)
		}
		bm := bundleMetadata{
			reference:   fmt.Sprintf("%s@%s", bundle.OriginReference.String(), bundle.Descriptor.Digest.String()),
			packageName: packageName,
			version:     *version,
			release:     release,
		}
		fmt.Printf("found bundle %q\n", bm.String())

		bundleMetadatas, ok := packages[packageName]
		if !ok {
			bundleMetadatas = map[bundleMetadata]oci.Artifact{}
		}
		bundleMetadatas[bm] = bundle.Artifact
		packages[packageName] = bundleMetadatas
	}

	catalogFsys := fstest.MapFS{}
	for _, pkgName := range sets.List(sets.KeySet(packages)) {
		fmt.Printf("building package %q\n", pkgName)
		pkg := packages[pkgName]
		channelBundles := map[string][]bundleMetadata{}
		highestVersion := semver.Version{}
		for bm := range pkg {
			channelName := channelNameForVersion(bm.version)
			if channel, ok := channelBundles[channelName]; ok {
				channelBundles[channelName] = append(channel, bm)
			} else {
				channelBundles[channelName] = []bundleMetadata{bm}
			}
			if bm.version.GreaterThan(&highestVersion) {
				highestVersion = bm.version
			}
		}
		var channels []declcfg.Channel
		for chName, bundleMetadatas := range channelBundles {
			slices.SortFunc(bundleMetadatas, func(i, j bundleMetadata) int {
				return i.Compare(j)
			})
			channelBundles[chName] = bundleMetadatas
			ch := declcfg.Channel{
				Schema:  declcfg.SchemaChannel,
				Package: pkgName,
				Name:    chName,
			}
			stack := []bundleMetadata{}
			for _, bm := range bundleMetadatas {
				entry := declcfg.ChannelEntry{
					Name: bm.String(),
				}
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i].version.Minor() == bm.version.Minor() {
						entry.Skips = append(entry.Skips, stack[i].String())
					} else {
						entry.Replaces = stack[i].String()
						break
					}
				}
				ch.Entries = append(ch.Entries, entry)
				stack = append(stack, bm)
			}
			channels = append(channels, ch)
		}
		fbc := declcfg.DeclarativeConfig{
			Packages: []declcfg.Package{{Schema: declcfg.SchemaPackage, Name: pkgName, DefaultChannel: channelNameForVersion(highestVersion)}},
			Channels: channels,
		}

		bms := sets.KeySet(pkg).UnsortedList()
		slices.SortFunc(bms, func(i, j bundleMetadata) int { return i.Compare(j) })
		for _, bm := range bms {
			art := pkg[bm]
			bundleFS, err := oci.Extract(context.Background(), art)
			if err != nil {
				return nil, fmt.Errorf("failed to get filesystem from bundle: %w", err)
			}

			if err := func() error {
				tmpDir, err := os.MkdirTemp("", "bundle-*")
				if err != nil {
					return err
				}
				defer os.RemoveAll(tmpDir)
				if err := fsutil.Write(tmpDir, bundleFS); err != nil {
					return err
				}
				render := action.Render{
					Refs:           []string{tmpDir},
					Registry:       nil,
					AllowedRefMask: action.RefBundleDir,
					Migrate:        true,
				}
				logrus.SetLevel(logrus.PanicLevel)
				tmpFbc, err := render.Run(context.Background())
				if err != nil {
					return fmt.Errorf("failed to render bundle image: %w", err)
				}

				tmpFbc.Bundles[0].Name = bm.String()
				tmpFbc.Bundles[0].Image = bm.reference
				fbc.Bundles = append(fbc.Bundles, tmpFbc.Bundles[0])

				return nil
			}(); err != nil {
				return nil, err
			}
		}
		buf := &bytes.Buffer{}
		if err := declcfg.WriteJSON(fbc, buf); err != nil {
			return nil, fmt.Errorf("failed to write FBC for package %q: %w", pkgName, err)
		}
		catalogFsys[pkgName] = &fstest.MapFile{
			Mode:    os.ModeDir | 0755,
			ModTime: time.Time{},
		}
		catalogFsys[fmt.Sprintf("%s/catalog.json", pkgName)] = &fstest.MapFile{
			Data:    buf.Bytes(),
			Mode:    0644,
			ModTime: time.Time{},
		}
	}
	return catalogFsys, nil
}

func getBundleVersion(bundle oci.Artifact, manifestsPath string) (*semver.Version, error) {
	fsys, err := oci.Extract(context.Background(), bundle)
	if err != nil {
		return nil, fmt.Errorf("failed to extract artifact filesystem: %w", err)
	}

	manifestsFS, err := fs.Sub(fsys, cmp.Or(filepath.Clean(manifestsPath), "manifests"))
	if err != nil {
		return nil, fmt.Errorf("failed to find manifests in bundle: %w", err)
	}
	return registryv1.GetVersion(manifestsFS)
}

func channelNameForVersion(version semver.Version) string {
	return fmt.Sprintf("default-v%d", version.Major())
}
