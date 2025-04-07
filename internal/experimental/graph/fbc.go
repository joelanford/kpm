package graph

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	mmsemver "github.com/Masterminds/semver/v3"
	bsemver "github.com/blang/semver/v4"
	"github.com/opencontainers/go-digest"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
)

func FromFBC(fbc declcfg.DeclarativeConfig) (*graphv1.Graph, error) {
	graph := &graphv1.Graph{
		MediaType: graphv1.MediaTypeGraph,
		Nodes:     make(map[digest.Digest]graphv1.Node),
		Edges:     make(map[digest.Digest]graphv1.Edge),
		Tags:      make(map[digest.Digest]graphv1.Tag),
	}
	xBundles, err := convertBundles(fbc.Bundles)
	if err != nil {
		return nil, err
	}

	for _, pkgBundles := range xBundles {
		for _, b := range pkgBundles {
			graph.Nodes[b.digest] = *b.graph
		}
	}

	for _, ch := range fbc.Channels {
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
				return nil, fmt.Errorf("bundle %q not found", entry.Name)
			}
			entryTag := graphv1.Tag{
				MediaType: graphv1.MediaTypeTag,
				Scope:     graphv1.ScopeNode,
				Key:       "channel",
				Value:     ch.Name,
				Reference: entryBundle.digest,
			}
			entryTagDigest := digestOf(entryTag)
			graph.Tags[entryTagDigest] = entryTag
			if entry.Replaces != "" {
				replacedBundle, ok := bundlesByName[entry.Replaces]
				if !ok {
					return nil, fmt.Errorf("bundle %q not found", entry.Replaces)
				}
				edge := graphv1.Edge{
					MediaType: graphv1.MediaTypeEdge,
					From:      replacedBundle.digest,
					To:        entryBundle.digest,
				}
				edgeDigest := digestOf(edge)
				graph.Edges[edgeDigest] = edge
				edgeTag := graphv1.Tag{
					MediaType: graphv1.MediaTypeTag,
					Scope:     graphv1.ScopeEdge,
					Key:       "channel",
					Value:     ch.Name,
					Reference: edgeDigest,
				}
				edgeTagDigest := digestOf(edgeTag)
				graph.Tags[edgeTagDigest] = edgeTag
			}
			for _, skipName := range entry.Skips {
				skippedBundle, ok := bundlesByName[skipName]
				if !ok {
					return nil, fmt.Errorf("bundle %q not found", skipName)
				}
				edge := graphv1.Edge{
					MediaType: graphv1.MediaTypeEdge,
					From:      skippedBundle.digest,
					To:        entryBundle.digest,
				}
				edgeDigest := digestOf(edge)
				graph.Edges[edgeDigest] = edge
				edgeTag := graphv1.Tag{
					MediaType: graphv1.MediaTypeTag,
					Scope:     graphv1.ScopeEdge,
					Key:       "channel",
					Value:     ch.Name,
					Reference: edgeDigest,
				}
				edgeTagDigest := digestOf(edgeTag)
				graph.Tags[edgeTagDigest] = edgeTag
			}
			skipRange, err := bsemver.ParseRange(entry.SkipRange)
			if err != nil {
				return nil, fmt.Errorf("invalid skipRange for entry %q in channel %q: %w", entry.Name, ch.Name, err)
			}
			for _, skipRangeBundle := range pkgBundles {
				bv := bsemver.MustParse(skipRangeBundle.graph.Version.String())
				if !skipRange(bv) {
					continue
				}
				edge := graphv1.Edge{
					MediaType: graphv1.MediaTypeEdge,
					From:      skipRangeBundle.digest,
					To:        entryBundle.digest,
				}
				edgeDigest := digestOf(edge)
				graph.Edges[edgeDigest] = edge
				edgeTag := graphv1.Tag{
					MediaType: graphv1.MediaTypeTag,
					Scope:     graphv1.ScopeEdge,
					Key:       "channel",
					Value:     ch.Name,
					Reference: edgeDigest,
				}
				edgeTagDigest := digestOf(edgeTag)
				graph.Tags[edgeTagDigest] = edgeTag
			}
		}
	}
	return graph, nil
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
			MediaType: graphv1.MediaTypeBundle,
			NVR: graphv1.NVR{
				Name:    b.Package,
				Version: *version,
			},
			Reference: fmt.Sprintf("oci://%s", b.Image),
		}
		xb := xBundle{fbc: b, graph: &gb, digest: digestOf(gb)}
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

func digestOf(v any) digest.Digest {
	hasher := sha256.New()
	enc := json.NewEncoder(hasher)
	enc.SetIndent("", "")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return digest.NewDigestFromBytes(digest.SHA256, hasher.Sum(nil))
}
