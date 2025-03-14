package v1

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sync"

	"github.com/blang/semver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"

	v2 "github.com/joelanford/kpm/internal/experimental/pkg/artifact/v1"
	oci2 "github.com/joelanford/kpm/internal/experimental/pkg/oci"
	slicesutil "github.com/joelanford/kpm/internal/util/slices"
)

func FBCToOCI(ctx context.Context, fbcFS fs.FS, outDir string) (ocispec.Descriptor, error) {
	metasDir, err := os.MkdirTemp("", "kpm-from-fbc-")
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	defer os.RemoveAll(metasDir)

	if err := writeMetasToFiles(ctx, fbcFS, metasDir); err != nil {
		return ocispec.Descriptor{}, err
	}

	var store oras.Target
	store, err = oci.NewWithContext(ctx, outDir)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	var (
		allPackageDescs []ocispec.Descriptor
		mu              sync.Mutex
	)
	if err := walkPackages(ctx, metasDir, func(ctx context.Context, pkgDir string) error {
		bundles, err := collectBundles(ctx, os.DirFS(filepath.Join(pkgDir, declcfg.SchemaBundle)))
		if err != nil {
			return err
		}

		channels, err := collectChannels(ctx, os.DirFS(filepath.Join(pkgDir, declcfg.SchemaChannel)), bundles)
		if err != nil {
			return err
		}
		slices.SortFunc(channels, compareChannels)

		pkg, err := getPackage(ctx, os.DirFS(filepath.Join(pkgDir, declcfg.SchemaPackage)), channels)
		if err != nil {
			return err
		}

		pkgDesc, err := oci2.PushArtifact(ctx, store, pkg)
		if oci2.IgnoreExists(err) != nil {
			return err
		}

		if err := storeDeprecations(ctx, store, pkgDir, *pkg, channels, bundles); err != nil {
			return err
		}

		fmt.Println("stored package", pkg.ID.String())
		mu.Lock()
		defer mu.Unlock()
		allPackageDescs = append(allPackageDescs, pkgDesc)
		return nil
	}); err != nil {
		return ocispec.Descriptor{}, err
	}

	slices.SortFunc(allPackageDescs, comparePackages)
	catalog := &v2.ShallowCatalog{
		Packages: allPackageDescs,
	}
	if _, err := oci2.PushBlob(ctx, store, oci2.EmptyJSONBlob()); oci2.IgnoreExists(err) != nil {
		return ocispec.Descriptor{}, err
	}
	desc, err := oci2.PushArtifact(ctx, store, catalog)
	if oci2.IgnoreExists(err) != nil {
		return ocispec.Descriptor{}, err
	}
	return desc, store.Tag(ctx, desc, "catalog")
}

func writeMetasToFiles(ctx context.Context, fbcFS fs.FS, baseDir string) error {
	var (
		files   = make(map[string]*os.File)
		filesMu sync.Mutex
	)
	if err := declcfg.WalkMetasFS(ctx, fbcFS, func(path string, meta *declcfg.Meta, err error) error {
		if err != nil || meta == nil {
			return err
		}
		switch meta.Schema {
		case declcfg.SchemaPackage, declcfg.SchemaChannel, declcfg.SchemaBundle, declcfg.SchemaDeprecation:
		default:
			return fmt.Errorf("unknown schema %q cannot be converted to OCI", meta.Schema)
		}

		metaPath := pathFromMeta(baseDir, *meta)
		if err := os.MkdirAll(filepath.Dir(metaPath), 0700); err != nil {
			return err
		}

		filesMu.Lock()
		f, ok := files[metaPath]
		if !ok {
			var err error
			f, err = os.Create(metaPath)
			if err != nil {
				return err
			}
			files[metaPath] = f
		}
		filesMu.Unlock()
		_, err = f.Write(meta.Blob)
		return err
	}); err != nil {
		return err
	}
	for _, file := range files {
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func compareBundles(a, b v2.Bundle) int {
	if c := a.ID.Version.Compare(b.ID.Version); c != 0 {
		return c
	}
	return cmp.Compare(a.ID.Release, b.ID.Release)
}

func compareUpgradeEdges(a, b v2.UpgradeEdge) int {
	if d := compareBundles(b.To, a.To); d != 0 {
		return d
	}
	return compareBundles(b.From, a.From)
}

func compareChannels(a, b v2.Channel) int {
	return cmp.Compare(a.ID.Name, b.ID.Name)
}

func comparePackages(a, b ocispec.Descriptor) int {
	return cmp.Compare(a.Annotations[v2.AnnotationPackageName], b.Annotations[v2.AnnotationPackageName])
}

func collectItems[O any](ctx context.Context, itemsFS fs.FS) ([]O, error) {
	var (
		items []O
		mu    sync.Mutex
	)
	if err := declcfg.WalkMetasFS(ctx, itemsFS, func(path string, meta *declcfg.Meta, err error) error {
		if err != nil {
			return err
		}
		o, err := parseMeta[O](*meta)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		items = append(items, o)
		return nil
	}, declcfg.WithConcurrency(1)); err != nil {
		return nil, err
	}
	return items, nil
}

func pathFromMeta(baseDir string, meta declcfg.Meta) string {
	// <pkgName>/<schema>.json
	pkgName := meta.Package
	if meta.Schema == declcfg.SchemaPackage {
		pkgName = meta.Name
	}
	if pkgName == "" {
		pkgName = "__empty__"
	}
	return filepath.Join(baseDir, pkgName, meta.Schema, "catalog.json")
}

func walkPackages(ctx context.Context, baseDir string, cb func(context.Context, string) error) error {
	pkgDirs, err := filepath.Glob(filepath.Join(baseDir, "*"))
	if err != nil {
		return err
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(runtime.NumCPU())
	for _, pkgDir := range pkgDirs {
		eg.Go(func() error {
			return cb(egCtx, pkgDir)
		})
	}
	return eg.Wait()
}

func parseMeta[O any](m declcfg.Meta) (O, error) {
	var o O
	err := json.Unmarshal(m.Blob, &o)
	return o, err
}

func getPackage(ctx context.Context, packageFS fs.FS, channels []v2.Channel) (*v2.Package, error) {
	packages, err := collectItems[declcfg.Package](ctx, packageFS)
	if err != nil {
		return nil, err
	}
	if len(packages) != 1 {
		return nil, fmt.Errorf("expected exactly one package, got %d", len(packages))
	}
	in := packages[0]

	out := &v2.Package{
		ID:             v2.PackageIdentity{Name: in.Name},
		DefaultChannel: in.DefaultChannel,
		Channels:       channels,
	}
	if in.Description != "" {
		out.DisplayMetadata = &v2.PackageDisplayMetadata{Description: in.Description}
	}
	if in.Icon != nil {
		out.Icon = v2.NewIcon(in.Icon.MediaType, in.Icon.Data)
	}
	return out, nil
}

func collectChannels(ctx context.Context, channelFS fs.FS, bundles []v2.Bundle) ([]v2.Channel, error) {
	aliases := make(map[string]*v2.Bundle)
	for i, bd := range bundles {
		for _, alias := range bd.ID.Aliases {
			aliases[alias] = &bundles[i]
		}
	}
	channels, err := collectItems[declcfg.Channel](ctx, channelFS)
	if err != nil {
		return nil, err
	}
	channelArtifacts := make([]v2.Channel, 0, len(channels))
	for _, ch := range channels {
		channelArtifacts = append(channelArtifacts, v2.Channel{
			ID:           v2.ChannelIdentity{Package: ch.Package, Name: ch.Name},
			UpgradeEdges: upgradeEdgesForChannel(ch.Entries, aliases),
			Bundles:      bundlesForChannel(ch.Entries, aliases),
		})
	}
	return channelArtifacts, nil
}

func upgradeEdgesForChannel(entries []declcfg.ChannelEntry, aliases map[string]*v2.Bundle) []v2.UpgradeEdge {
	var upgradeEdges []v2.UpgradeEdge

	for _, entry := range entries {
		to, ok := aliases[entry.Name]
		if !ok {
			continue
		}
		froms := map[*v2.Bundle]struct{}{}
		if entry.Replaces != "" {
			if repl, ok := aliases[entry.Replaces]; ok {
				froms[repl] = struct{}{}
			}
		}
		for _, skip := range entry.Skips {
			if s, ok := aliases[skip]; ok {
				froms[s] = struct{}{}
			}
		}
		r, err := semver.ParseRange(entry.SkipRange)
		if err == nil {
			for _, b := range aliases {
				if r(b.ID.Version) {
					froms[b] = struct{}{}
				}
			}
		}
		for from := range froms {
			upgradeEdges = append(upgradeEdges, v2.UpgradeEdge{From: *from, To: *to})
		}
	}
	slices.SortFunc(upgradeEdges, compareUpgradeEdges)
	return upgradeEdges
}

func bundlesForChannel(entries []declcfg.ChannelEntry, aliases map[string]*v2.Bundle) []v2.Bundle {
	out := make([]v2.Bundle, 0, len(entries))
	for _, entry := range entries {
		to, ok := aliases[entry.Name]
		if !ok {
			continue
		}
		out = append(out, *to)
	}
	return out
}

func collectBundles(ctx context.Context, bundleFS fs.FS) ([]v2.Bundle, error) {
	bundles, err := collectItems[declcfg.Bundle](ctx, bundleFS)
	if err != nil {
		return nil, err
	}
	var bundleArtifacts []v2.Bundle

	for _, b := range bundles {
		ba, err := convertBundle(b)
		if err != nil {
			return nil, err
		}
		bundleArtifacts = append(bundleArtifacts, *ba)
	}
	buildVersion := func(in semver.Version) semver.Version {
		return semver.Version{Pre: slicesutil.MapSlice(in.Build, func(b string) semver.PRVersion {
			return semver.PRVersion{VersionStr: b}
		})}
	}
	slices.SortFunc(bundleArtifacts, func(a, b v2.Bundle) int {
		if c := a.ID.Version.Compare(b.ID.Version); c != 0 {
			return c
		}
		return buildVersion(a.ID.Version).Compare(buildVersion(b.ID.Version))
	})
	bundlesIndicesByVersion := map[string][]int{}
	for i := range bundleArtifacts {
		bundleArtifacts[i].ID.Version.Build = nil
		v := bundleArtifacts[i].ID.Version.String()
		bundlesIndicesByVersion[v] = append(bundlesIndicesByVersion[v], i)
	}
	for _, bundleIndices := range bundlesIndicesByVersion {
		for release, bundleIndex := range bundleIndices {
			bundleArtifacts[bundleIndex].ID.Release = release
		}
	}
	return bundleArtifacts, nil
}

func convertBundle(in declcfg.Bundle) (*v2.Bundle, error) {
	props, err := property.Parse(in.Properties)
	if err != nil {
		return nil, err
	}
	if len(props.Packages) != 1 {
		return nil, fmt.Errorf("expected exactly one property of type %q, found %d", property.TypePackage, len(props.Packages))
	}
	if len(props.CSVMetadatas) != 1 {
		return nil, fmt.Errorf("expected exactly one property of type %q, found %d", property.TypeCSVMetadata, len(props.CSVMetadatas))
	}

	bundleIdentity, err := getBundleIdentity(in, props.Packages[0])
	if err != nil {
		return nil, err
	}

	resolutionMetadata, err := getBundleResolutionMetadata(props)
	if err != nil {
		return nil, err
	}

	return &v2.Bundle{
		ID:                 *bundleIdentity,
		ResolutionMetadata: resolutionMetadata,
		QueryMetadata:      getBundleQueryMetadata(props.CSVMetadatas[0]),
		DisplayMetadata:    getBundleDisplayMetadata(props.CSVMetadatas[0]),
		MirrorMetadata:     getMirrorMetadata(in.RelatedImages),
		ExtendedMetadata:   getBundleExtendedMetadata(props.CSVMetadatas[0], props.Others),
	}, nil
}

func getBundleIdentity(in declcfg.Bundle, pkgProp property.Package) (*v2.BundleIdentity, error) {
	pkg := pkgProp.PackageName
	version, err := semver.Parse(pkgProp.Version)
	if err != nil {
		return nil, err
	}
	release := 0
	name := fmt.Sprintf("%s.v%s-%d", pkg, version, release)
	var aliases []string
	if name != in.Name {
		aliases = append(aliases, in.Name)
	}
	return &v2.BundleIdentity{
		Package: pkg,
		Version: version,
		Release: release,
		Aliases: aliases,
		URI:     fmt.Sprintf("docker://%s", in.Image),
	}, nil
}

func getBundleKubeVersionRange(minKubeVersionStr string) (string, error) {
	if minKubeVersionStr == "" {
		return "", nil
	}
	minKubeVersionSemver, err := semver.ParseTolerant(minKubeVersionStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse minKubeVersion %q from %q property: %v", minKubeVersionStr, property.TypeCSVMetadata, err)
	}
	return fmt.Sprintf(">=%s", minKubeVersionSemver), nil
}

func getBundleResolutionMetadata(props *property.Properties) (*v2.BundleResolutionMetadata, error) {
	if props == nil {
		return nil, nil
	}
	kubeVersionRange, err := getBundleKubeVersionRange(props.CSVMetadatas[0].MinKubeVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to determine bundle kube version range: %v", err)
	}
	out := &v2.BundleResolutionMetadata{
		ProvidedGVKs: slicesutil.MapSlice(props.GVKs, func(gvk property.GVK) schema.GroupVersionKind {
			return schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind}
		}),
		RequiredGVKs: slicesutil.MapSlice(props.GVKsRequired, func(gvk property.GVKRequired) schema.GroupVersionKind {
			return schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind}
		}),
		RequiredPackages: slicesutil.MapSlice(props.PackagesRequired, func(pkg property.PackageRequired) v2.RequiredPackage {
			return v2.RequiredPackage{Name: pkg.PackageName, VersionRange: pkg.VersionRange}
		}),
		KubernetesVersionRange: kubeVersionRange,
	}
	if out.KubernetesVersionRange == "" && len(out.ProvidedGVKs) == 0 && len(out.RequiredGVKs) == 0 && len(out.RequiredPackages) == 0 {
		return nil, nil
	}
	return out, nil
}

func getBundleDisplayMetadata(csvMetadata property.CSVMetadata) *v2.BundleDisplayMetadata {
	out := &v2.BundleDisplayMetadata{
		Description: csvMetadata.Description,
		Links: slicesutil.MapSlice(csvMetadata.Links, func(l v1alpha1.AppLink) v2.NamedURL {
			return v2.NamedURL{
				Name: l.Name,
				URL:  l.URL,
			}
		}),
		Maintainers: slicesutil.MapSlice(csvMetadata.Maintainers, func(m v1alpha1.Maintainer) v2.NamedEmail {
			return v2.NamedEmail{
				Name:  m.Name,
				Email: m.Email,
			}
		}),
	}
	if out.Description == "" && len(out.Links) == 0 && len(out.Maintainers) == 0 {
		return nil
	}
	return out
}

func getBundleQueryMetadata(csvMetadata property.CSVMetadata) *v2.BundleQueryMetadata {
	out := &v2.BundleQueryMetadata{
		Keywords: csvMetadata.Keywords,
		Maturity: csvMetadata.Maturity,
		Provider: v2.NamedURL{
			Name: csvMetadata.Provider.Name,
			URL:  csvMetadata.Provider.URL,
		},
	}
	if len(out.Keywords) == 0 && out.Maturity == "" && out.Provider.Name == "" && out.Provider.URL == "" {
		return nil
	}
	return out
}

func getMirrorMetadata(relatedImages []declcfg.RelatedImage) *v2.BundleMirrorMetadata {
	if len(relatedImages) == 0 {
		return nil
	}
	return &v2.BundleMirrorMetadata{
		RelatedImages: slicesutil.MapSlice(relatedImages, func(ri declcfg.RelatedImage) string {
			return ri.Image
		}),
	}
}

const (
	AnnotationCSVAnnotations               = "clusterserviceversion-annotations"
	AnnotationCSVAPIServiceDefinitions     = "clusterserviceversion-apiservicedefinitions"
	AnnotationCSVCustomResourceDefinitions = "clusterserviceversion-customresourcedefinitions"
	AnnotationCSVInstallModes              = "clusterserviceversion-installModes"
	AnnotationCSVNativeAPIs                = "clusterserviceversion-nativeAPIs"
)

func getBundleExtendedMetadata(csvMetadata property.CSVMetadata, others []property.Property) *v2.BundleExtendedMetadata {
	bem := &v2.BundleExtendedMetadata{Annotations: map[string]string{}}
	marshalAnnotations := map[string]func(property.CSVMetadata) ([]byte, error){}

	if len(csvMetadata.Annotations) > 0 {
		marshalAnnotations[AnnotationCSVAnnotations] = func(m property.CSVMetadata) ([]byte, error) { return json.Marshal(m.Annotations) }
	}
	if !reflect.DeepEqual(csvMetadata.APIServiceDefinitions, v1alpha1.APIServiceDefinitions{}) {
		marshalAnnotations[AnnotationCSVAPIServiceDefinitions] = func(m property.CSVMetadata) ([]byte, error) { return json.Marshal(m.APIServiceDefinitions) }
	}
	if !reflect.DeepEqual(csvMetadata.CustomResourceDefinitions, v1alpha1.CustomResourceDefinitions{}) {
		marshalAnnotations[AnnotationCSVCustomResourceDefinitions] = func(m property.CSVMetadata) ([]byte, error) { return json.Marshal(m.CustomResourceDefinitions) }
	}
	if len(csvMetadata.InstallModes) > 0 {
		marshalAnnotations[AnnotationCSVInstallModes] = func(m property.CSVMetadata) ([]byte, error) { return json.Marshal(m.InstallModes) }
	}
	if len(csvMetadata.NativeAPIs) > 0 {
		marshalAnnotations[AnnotationCSVNativeAPIs] = func(m property.CSVMetadata) ([]byte, error) { return json.Marshal(m.NativeAPIs) }
	}
	for k, marshal := range marshalAnnotations {
		v, err := marshal(csvMetadata)
		if err != nil {
			panic(err)
		}
		bem.Annotations[k] = string(v)
	}

	byType := map[string][]json.RawMessage{}
	for _, other := range others {
		byType[other.Type] = append(byType[other.Type], other.Value)
	}
	for typ, values := range byType {
		v, err := json.Marshal(values)
		if err != nil {
			panic(err)
		}
		bem.Annotations[typ] = string(v)
	}
	if len(bem.Annotations) == 0 {
		return nil
	}
	return bem
}

func storeDeprecations(ctx context.Context, t oras.Target, pkgDir string, pkg v2.Package, channels []v2.Channel, bundles []v2.Bundle) error {
	deprecationsStat, err := os.Stat(filepath.Join(pkgDir, declcfg.SchemaDeprecation))
	if err == nil && deprecationsStat.IsDir() {
		deprecations, err := collectItems[declcfg.Deprecation](ctx, os.DirFS(filepath.Join(pkgDir, declcfg.SchemaDeprecation)))
		if err != nil {
			return err
		}
		if len(deprecations) != 1 {
			return fmt.Errorf("expected exactly one deprecated package descriptor, got %d", len(deprecations))
		}
		for _, deprecation := range deprecations {
			for _, e := range deprecation.Entries {
				var a oci2.Artifact
				switch e.Reference.Schema {
				case declcfg.SchemaPackage:
					a = &v2.Deprecation[*v2.Package]{
						Message:   e.Message,
						Reference: &pkg,
					}
				case declcfg.SchemaChannel:
					for _, ch := range channels {
						if ch.ID.Name == e.Reference.Name {
							a = &v2.Deprecation[*v2.Channel]{
								Message:   e.Message,
								Reference: &ch,
							}
							break
						}
					}
				case declcfg.SchemaBundle:
				bundleLoop:
					for _, b := range bundles {
						for _, alias := range b.ID.Aliases {
							if alias == e.Reference.Name {
								a = &v2.Deprecation[*v2.Bundle]{
									Message:   e.Message,
									Reference: &b,
								}
								break bundleLoop
							}
						}
					}
				}

				if _, err := oci2.PushArtifact(ctx, t, a); oci2.IgnoreExists(err) != nil {
					return err
				}
			}
		}
	}
	return nil
}
