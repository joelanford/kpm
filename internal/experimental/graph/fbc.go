package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"

	mmsemver "github.com/Masterminds/semver/v3"
	bsemver "github.com/blang/semver/v4"
	"github.com/opencontainers/go-digest"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
)

func (idx *Index) AddFBC(fbc declcfg.DeclarativeConfig, tags map[string][]string) error {
	xBundles, err := convertBundles(fbc.Bundles)
	if err != nil {
		return err
	}

	for _, pkgBundles := range xBundles {
		for _, b := range pkgBundles {
			idx.AddNode(*b.graph, tags)
		}
	}

	for _, ch := range fbc.Channels {
		chTags := mergeTags(tags, map[string][]string{"channel": {ch.Name}})

		pkgBundles := xBundles[ch.Package]
		bundlesByName := map[string]*xBundle{}
		bundlesByVersion := map[mmsemver.Version]*xBundle{}
		for i, b := range pkgBundles {
			bundlesByName[b.fbc.Name] = &pkgBundles[i]
			bundlesByVersion[b.graph.Version] = &pkgBundles[i]
		}
		for _, entry := range ch.Entries {
			entryBundle, ok := bundlesByName[entry.Name]
			if !ok {
				return fmt.Errorf("bundle %q not found", entry.Name)
			}
			idx.AddTag(entryBundle.digest, "channel", ch.Name)

			if entry.Replaces != "" {
				replacedBundle, ok := bundlesByName[entry.Replaces]
				if !ok {
					_, _ = fmt.Fprintf(os.Stderr, "WARNING: skipping edge creation for unknown bundle %q for entry %q in channel %q with tags %v\n", entry.Replaces, entry.Name, ch.Name, tags)
					continue
				}

				if _, err := idx.AddEdge(NewEdge(replacedBundle.digest, entryBundle.digest), chTags); err != nil {
					return fmt.Errorf("failed edge creation from %q to %q channel %q with tags %v: %w\n", entry.Replaces, entry.Name, ch.Name, tags, err)
				}
			}
			for _, skipName := range entry.Skips {
				skippedBundle, ok := bundlesByName[skipName]
				if !ok {
					_, _ = fmt.Fprintf(os.Stderr, "WARNING: skipping edge creation for unknown bundle %q for entry %q in channel %q with tags %v\n", skipName, entry.Name, ch.Name, tags)
					continue
				}
				if _, err := idx.AddEdge(NewEdge(skippedBundle.digest, entryBundle.digest), chTags); err != nil {
					return fmt.Errorf("failed edge creation from %q to %q channel %q with tags %v: %w\n", entry.Replaces, entry.Name, ch.Name, tags, err)
				}
			}
			if entry.SkipRange != "" {
				skipRange, err := bsemver.ParseRange(entry.SkipRange)
				if err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "WARNING: skipping edge creation for invalid skipRange %q for entry %q in channel %q with tags: %v\n", entry.SkipRange, entry.Name, ch.Name, tags)
				} else {
					for _, skipRangeBundle := range pkgBundles {
						bv := bsemver.MustParse(skipRangeBundle.graph.Version.String())
						if !skipRange(bv) {
							continue
						}
						if _, err := idx.AddEdge(NewEdge(skipRangeBundle.digest, entryBundle.digest), chTags); err != nil {
							return fmt.Errorf("failed edge creation from %q to %q channel %q with tags %v: %w\n", entry.Replaces, entry.Name, ch.Name, tags, err)
						}
					}
				}
			}
		}
	}
	return nil
}

type xBundle struct {
	fbc    *declcfg.Bundle
	graph  *graphv1.Node
	digest digest.Digest
}

func convertBundles(in []declcfg.Bundle) (map[string][]xBundle, error) {
	dropVersionMetadata := func(bundle *xBundle) {
		bundle.graph.Version = *(mmsemver.New(
			bundle.graph.Version.Major(),
			bundle.graph.Version.Minor(),
			bundle.graph.Version.Patch(),
			bundle.graph.Version.Prerelease(),
			"",
		))
	}
	out := make(map[string][]xBundle)
	for i := range in {
		b := &in[i]
		version, err := parseBundleVersion(b.Properties)
		if err != nil {
			return nil, err
		}
		gb := graphv1.Node{
			MediaType: graphv1.MediaTypeNode,
			NVR: graphv1.NVR{
				Name:    b.Package,
				Version: *version,
			},
			Reference: fmt.Sprintf("oci://%s", b.Image),
		}
		xb := xBundle{fbc: b, graph: &gb}
		out[b.Package] = append(out[b.Package], xb)
	}

	for pkgName, pkgBundles := range out {
		slices.SortFunc(pkgBundles, func(a, b xBundle) int {
			if d := a.graph.Version.Compare(&b.graph.Version); d != 0 {
				return d
			}
			am := mmsemver.New(0, 0, 0, a.graph.Version.Metadata(), "")
			bm := mmsemver.New(0, 0, 0, b.graph.Version.Metadata(), "")
			return am.Compare(bm)
		})
		if len(pkgBundles) > 0 {
			dropVersionMetadata(&pkgBundles[0])
			for i := range pkgBundles[1:] {
				prev := &pkgBundles[i]
				cur := &pkgBundles[i+1]
				if cur.graph.Version.Equal(&prev.graph.Version) {
					cur.graph.Release = prev.graph.Release + 1
				}
				dropVersionMetadata(cur)
			}
		}
		for i := range pkgBundles {
			pkgBundles[i].digest = digestOf(pkgBundles[i].graph)
		}
		out[pkgName] = pkgBundles
	}
	return out, nil
}

func parseBundleVersion(props []property.Property) (*mmsemver.Version, error) {
	for _, prop := range props {
		if prop.Type != property.TypePackage {
			continue
		}
		var pkg property.Package
		if err := json.Unmarshal(prop.Value, &pkg); err != nil {
			return nil, err
		}
		v, err := mmsemver.NewVersion(pkg.Version)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
	return nil, errors.New("no version information found")
}
