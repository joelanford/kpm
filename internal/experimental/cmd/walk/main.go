package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/oci"

	artifactv1 "github.com/joelanford/kpm/internal/experimental/pkg/artifact/v1"
	oci3 "github.com/joelanford/kpm/internal/experimental/pkg/encoding/oci"
	oci2 "github.com/joelanford/kpm/internal/experimental/pkg/oci"
)

type descMeta struct {
	mediaType    string
	artifactType string
}

func d2m(d ocispec.Descriptor) descMeta {
	return descMeta{d.MediaType, d.ArtifactType}
}

func main() {
	ociDir := os.Args[1]
	ref := os.Args[2]

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	t, err := oci.NewWithContext(ctx, ociDir)
	if err != nil {
		log.Fatal(err)
	}

	decoder := oci3.NewDecoder(t)
	walker := oci2.New(t)

	start := time.Now()
	defer func() { fmt.Println(time.Since(start)) }()

	found := map[descMeta]map[digest.Digest]int{}
	if err := walker.Reference(ctx, ref, func(ctx context.Context, path []ocispec.Descriptor, descriptor ocispec.Descriptor, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		k := d2m(descriptor)
		if digests, ok := found[k]; !ok {
			digests = map[digest.Digest]int{}
			found[k] = digests
		}
		found[k][descriptor.Digest]++

		indent := strings.Repeat("  ", len(path))
		if descriptor.ArtifactType == "" {
			fmt.Printf("%s|- %s %s\n", indent, descriptor.MediaType, descriptor.Digest)
			return nil
		}
		switch descriptor.ArtifactType {
		case artifactv1.ArtifactTypeCatalog:
			//var c artifactv1.Catalog
			//if err := decoder.Decode(ctx, descriptor, &c); err != nil {
			//	return err
			//}
			fmt.Printf("%scatalog", indent)
		case artifactv1.ArtifactTypePackage:
			if descriptor.Annotations[artifactv1.AnnotationPackageName] != "amq-broker-rhel8" {
				return oci2.ErrSkip
			}
			var p artifactv1.Package
			if err := decoder.Decode(ctx, descriptor, &p); err != nil {
				return err
			}
			fmt.Printf("%spackage %q\n", indent, p.ID.Name)
		case artifactv1.ArtifactTypeChannel:
			if descriptor.Annotations[artifactv1.AnnotationChannelName] != "7.12.x" {
				return oci2.ErrSkip
			}
			var ch artifactv1.Channel
			if err := decoder.Decode(ctx, descriptor, &ch); err != nil {
				return err
			}
			fmt.Printf("%schannel %q\n", indent, ch.ID.Name)
		case artifactv1.ArtifactTypeUpgradeEdge:
			var ue artifactv1.UpgradeEdge
			if err := decoder.Decode(ctx, descriptor, &ue); err != nil {
				return err
			}
			fmt.Printf("%supgrade edge (%s --> %s)\n", indent, ue.From.ID.String(), ue.To.ID.String())
			return oci2.ErrSkip
		case artifactv1.ArtifactTypeBundle:
			var b artifactv1.Bundle
			if err := decoder.Decode(ctx, descriptor, &b); err != nil {
				return err
			}
			fmt.Printf("%sbundle (v:%q, r:%d)\n", indent, b.ID.Version, b.ID.Release)
			return oci2.ErrSkip
		case artifactv1.ArtifactTypeDeprecation:
			switch path[len(path)-1].ArtifactType {
			case artifactv1.ArtifactTypeCatalog:
				var d artifactv1.Deprecation[*artifactv1.Catalog]
				if err := decoder.Decode(ctx, descriptor, &d); err != nil {
					return err
				}
				fmt.Printf("%sdeprecation: %s\n", indent, d.Message)
			case artifactv1.ArtifactTypePackage:
				var d artifactv1.Deprecation[*artifactv1.Package]
				if err := decoder.Decode(ctx, descriptor, &d); err != nil {
					return err
				}
				fmt.Printf("%sdeprecation: %s\n", indent, d.Message)
			case artifactv1.ArtifactTypeChannel:
				var d artifactv1.Deprecation[*artifactv1.Channel]
				if err := decoder.Decode(ctx, descriptor, &d); err != nil {
					return err
				}
				fmt.Printf("%sdeprecation: %s\n", indent, d.Message)
			case artifactv1.ArtifactTypeBundle:
				var d artifactv1.Deprecation[*artifactv1.Bundle]
				if err := decoder.Decode(ctx, descriptor, &d); err != nil {
					return err
				}
				fmt.Printf("%sdeprecation: %s\n", indent, d.Message)
			}

		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	//for _, m := range slices.SortedFunc(maps.Keys(found), func(a, b descMeta) int {
	//	if d := cmp.Compare(a.mediaType, b.mediaType); d != 0 {
	//		return d
	//	}
	//	return cmp.Compare(a.artifactType, b.artifactType)
	//}) {
	//	digests := found[m]
	//	fmt.Println(m, len(digests))
	//	iter := func(yield func(digestCount) bool) {
	//		for digest, count := range digests {
	//			if !yield(digestCount{digest, count}) {
	//				return
	//			}
	//		}
	//	}
	//
	//	for _, dg := range slices.SortedFunc(iter, func(a, b digestCount) int {
	//		return cmp.Compare(b.count, a.count)
	//	}) {
	//		fmt.Println("     ", dg)
	//	}
	//}
}

type digestCount struct {
	digest digest.Digest
	count  int
}
