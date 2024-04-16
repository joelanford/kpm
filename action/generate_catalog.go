package action

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing/fstest"
	"text/template"
	"time"

	"github.com/Masterminds/semver/v3"
	buildv1 "github.com/joelanford/kpm/build/v1"
	"github.com/joelanford/kpm/internal/fsutil"
	"github.com/joelanford/kpm/internal/registryv1"
	"github.com/joelanford/kpm/internal/remote"
	"github.com/joelanford/kpm/oci"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

type GenerateCatalog struct {
	BundleRepository string
	Bundles          []oci.Artifact
	PushBundles      bool
	Tag              string

	Log func(string, ...interface{})
}

func (a *GenerateCatalog) Run(_ context.Context) (fs.FS, error) {
	if len(a.Bundles) == 0 {
		return nil, fmt.Errorf("no bundles provided")
	}
	opts := []buildv1.BuildOption{}
	if a.Log != nil {
		opts = append(opts, buildv1.WithLog(a.Log))
	}
	return a.generate()
}

type bundleMetadata struct {
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
	if a.BundleRepository == "" {
		return nil, fmt.Errorf("bundle repository is required")
	}

	packages := map[string]map[bundleMetadata]oci.Artifact{}
	a.Log("loading bundles")
	for _, bundle := range a.Bundles {
		annotations, err := bundle.Annotations()
		if err != nil {
			return nil, fmt.Errorf("failed to get annotations for bundle with tag %q: %w", bundle.Tag(), err)
		}
		if mediaType, foundMediaType := annotations["operators.operatorframework.io.bundle.mediatype.v1"]; !foundMediaType || mediaType != "registry+v1" {
			return nil, fmt.Errorf("bundle with tag %q is not a catalog bundle", bundle.Tag())
		}
		packageName, foundPackageName := annotations["operators.operatorframework.io.bundle.package.v1"]
		if !foundPackageName {
			return nil, fmt.Errorf("failed to find package name for bundle with tag %q", bundle.Tag())
		}
		release, releaseFound := annotations["operators.operatorframework.io.bundle.release.v1"]
		if !releaseFound {
			release = "0"
		}
		version, err := getBundleVersion(bundle, annotations["operators.operatorframework.io.bundle.manifests.v1"])
		if err != nil {
			return nil, fmt.Errorf("failed to get version for bundle with tag %q: %w", bundle.Tag(), err)
		}
		bm := bundleMetadata{
			packageName: packageName,
			version:     *version,
			release:     release,
		}

		bundleMetadatas, ok := packages[packageName]
		if !ok {
			bundleMetadatas = map[bundleMetadata]oci.Artifact{}
		}
		bundleMetadatas[bm] = bundle
		packages[packageName] = bundleMetadatas
	}

	catalogFsys := fstest.MapFS{}
	for _, pkgName := range sets.List(sets.KeySet(packages)) {
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
			_, desc, err := oci.Write(context.Background(), io.Discard, art)
			if err != nil {
				return nil, fmt.Errorf("failed to get bundle descriptor for bundle %q: %w", bm.String(), err)
			}
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
					Refs:             []string{tmpDir},
					Registry:         nil,
					AllowedRefMask:   action.RefBundleDir,
					Migrate:          true,
					ImageRefTemplate: template.Must(template.New("image").Parse(fmt.Sprintf("%s@%s", a.BundleRepository, desc.Digest.String()))),
				}
				logrus.SetLevel(logrus.PanicLevel)
				a.Log("rendering bundle %q into catalog", bm.String())
				tmpFbc, err := render.Run(context.Background())
				if err != nil {
					return fmt.Errorf("failed to render bundle image: %w", err)
				}

				tmpFbc.Bundles[0].Name = bm.String()
				fbc.Bundles = append(fbc.Bundles, tmpFbc.Bundles[0])

				if a.PushBundles {
					a.Log("pushing image for bundle %q", bm.String())
					imageRef := tmpFbc.Bundles[0].Image
					target, err := remote.NewRepository(imageRef)
					if err != nil {
						return fmt.Errorf("failed to get repository for reference %q: %w", imageRef, err)
					}
					if _, err := oci.Push(context.Background(), art, target, oci.PushOptions{}); err != nil {
						return fmt.Errorf("failed to push image for bundle %q: %w", bm.String(), err)
					}
				}
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
	return fmt.Sprintf("stable-v%d", version.Major())
}
