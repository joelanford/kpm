package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"mime"
	"reflect"
	"slices"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"k8s.io/apimachinery/pkg/labels"
	"oras.land/oras-go/v2/content"
)

type BlobUnmarshaler interface {
	UnmarshalBlob(ocispec.Descriptor, []byte) error
}

func UnmarshalBlob(descriptor ocispec.Descriptor, data []byte, b any) error {
	if unmarshaler, ok := b.(BlobUnmarshaler); ok {
		return unmarshaler.UnmarshalBlob(descriptor, data)
	}
	if strings.HasSuffix(descriptor.MediaType, "+json") {
		return json.Unmarshal(data, b)
	}
	return fmt.Errorf("unsupported media type %q (perhaps %T should implement BlobUnmarshaler?)", descriptor.MediaType, b)
}

func UnmarshalDescriptor[T any](ctx context.Context, fetcher content.Fetcher, descriptor ocispec.Descriptor) (*T, error) {
	data, err := content.FetchAll(ctx, fetcher, descriptor)
	if err != nil {
		return nil, err
	}

	var obj T
	if err := UnmarshalBlob(descriptor, data, &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

type Decoder struct {
	fetcher content.Fetcher
}

func NewDecoder(f content.Fetcher) *Decoder {
	return &Decoder{fetcher: f}
}

func (d *Decoder) Decode(ctx context.Context, desc ocispec.Descriptor, v any) error {
	if desc.MediaType != ocispec.MediaTypeImageManifest {
		return errors.New("decoding currently only supports decoding from an OCI Manifest")
	}
	man, err := UnmarshalDescriptor[ocispec.Manifest](ctx, d.fetcher, desc)
	if err != nil {
		return err
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return &InvalidDecodeError{rv.Type()}
	}

	rv = rv.Elem()
	typ := rv.Type()

	configList := make([]int, 0)
	artifactMap := make(map[int]artifactFetcher)
	subjectMap := make(map[int]subjectFetcher)
	blobMap := make(map[int]blobFetcher)
	annotationMap := make(map[int]annotationFetcher)
	for i := 0; i < typ.NumField(); i++ {
		if !rv.Field(i).CanSet() {
			continue
		}
		field := typ.Field(i)

		tag := field.Tag.Get("oci")
		if tag == "" {
			continue
		}
		key, fetcherStr, found := strings.Cut(tag, ":")
		if !found && key != "config" {
			return fmt.Errorf("invalid oci tag %q", tag)
		}
		switch key {
		case "config":
			configList = append(configList, i)
		case "artifact":
			f, err := parseArtifactFetcher(fetcherStr)
			if err != nil {
				return err
			}
			artifactMap[i] = *f
		case "subject":
			f, err := parseSubjectFetcher(fetcherStr)
			if err != nil {
				return err
			}
			subjectMap[i] = *f
		case "blob":
			f, err := parseBlobFetcher(fetcherStr)
			if err != nil {
				return err
			}
			blobMap[i] = *f
		case "annotation":
			f, err := parseAnnotationFetcher(fetcherStr)
			if err != nil {
				return err
			}
			annotationMap[i] = *f
		}
	}

	for _, i := range configList {
		field := rv.Field(i)
		b, err := fetchConfig(ctx, d.fetcher, *man)
		if err != nil {
			return err
		}

		iface := field.Addr().Interface()
		if field.Kind() == reflect.Pointer {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			iface = field.Interface()
		}
		if err := UnmarshalBlob(b.descriptor, b.data, iface); err != nil {
			return err
		}
	}

	for _, i := range slices.Sorted(maps.Keys(artifactMap)) {
		field := rv.Field(i)
		f := artifactMap[i]
		subartifacts, err := f.Fetch(*man)
		if err != nil {
			return err
		}
		if len(subartifacts) == 0 {
			continue
		}

		if field.Kind() == reflect.Slice {
			for _, subdesc := range subartifacts {
				if field.IsNil() {
					newSlice := reflect.MakeSlice(field.Type(), 0, 1)
					field.Set(newSlice)
				}
				elemType := field.Type().Elem()
				newElem := reflect.New(elemType).Elem()
				newElemPtr := newElem.Addr().Interface()
				if err := d.Decode(ctx, subdesc, newElemPtr); err != nil {
					return err
				}
				updatedSlice := reflect.Append(field, newElem)
				field.Set(updatedSlice)
			}
		} else {
			if len(subartifacts) > 1 {
				typ := rv.Type().Field(i)
				return fmt.Errorf("found %d subartifacts matching tag %q for field %q", len(subartifacts), typ.Tag, typ.Name)
			}
			iface := field.Addr().Interface()
			if field.Kind() == reflect.Pointer {
				if field.IsNil() {
					field.Set(reflect.New(field.Type().Elem()))
				}
				iface = field.Interface()
			}
			if err := d.Decode(ctx, subartifacts[0], iface); err != nil {
				return err
			}
		}

	}

	for _, i := range slices.Sorted(maps.Keys(subjectMap)) {
		field := rv.Field(i)
		fieldType := typ.Field(i)
		if fieldType.Type.Kind() != reflect.Pointer || fieldType.Type.Elem().Kind() != reflect.Struct {
			return fmt.Errorf("decoding currently only supports a single subject struct pointer field, found %q on field %q", field.Kind(), typ.Field(i).Name)
		}
		f := subjectMap[i]
		subject, err := f.Fetch(ctx, d.fetcher, *man)
		if err != nil {
			return err
		}
		if subject == nil {
			continue
		}
		val := reflect.ValueOf(subject).Elem()
		field.Set(val)
	}

	for _, i := range slices.Sorted(maps.Keys(blobMap)) {
		f := blobMap[i]
		blobs, err := f.Fetch(ctx, d.fetcher, *man)
		if err != nil {
		}
		if len(blobs) == 0 {
			continue
		}

		field := rv.Field(i)
		if field.Kind() == reflect.Slice {
			for _, b := range blobs {
				if field.IsNil() {
					newSlice := reflect.MakeSlice(field.Type(), 0, 1)
					field.Set(newSlice)
				}
				elemType := field.Type().Elem()
				newElem := reflect.New(elemType).Elem()
				newElemPtr := newElem.Addr().Interface()
				if err := UnmarshalBlob(b.descriptor, b.data, newElemPtr); err != nil {
					return err
				}
				updatedSlice := reflect.Append(field, newElem)
				field.Set(updatedSlice)
			}
		} else {
			if len(blobs) > 1 {
				typ := rv.Type().Field(i)
				return fmt.Errorf("found %d blobs matching tag %q for field %q", len(blobs), typ.Tag, typ.Name)
			}
			iface := field.Addr().Interface()
			if field.Kind() == reflect.Pointer {
				if field.IsNil() {
					field.Set(reflect.New(field.Type().Elem()))
				}
				iface = field.Interface()
			}
			if err := UnmarshalBlob(blobs[0].descriptor, blobs[0].data, iface); err != nil {
				return err
			}
		}
	}

	for _, i := range slices.Sorted(maps.Keys(annotationMap)) {
		field := rv.Field(i)
		f := annotationMap[i]
		annotationValue := f.Fetch(*man)
		field.Set(reflect.ValueOf(annotationValue))
	}
	return nil
}

// An InvalidDecodeError describes an invalid argument passed to [Decode].
// (The argument to [Decode] must be a non-nil pointer.)
type InvalidDecodeError struct {
	Type reflect.Type
}

func (e *InvalidDecodeError) Error() string {
	if e.Type == nil {
		return "oci: Decode(nil)"
	}

	if e.Type.Kind() != reflect.Pointer || e.Type.Elem().Kind() != reflect.Struct {
		return "oci: Decode(non-struct pointer " + e.Type.String() + ")"
	}
	return "json: Decode(nil " + e.Type.String() + ")"
}

func parseDescriptorMatcher(matcherString string) (*descriptorMatcher, error) {
	// mediaType=foo;artifactType=bar;selector=baz
	dm := &descriptorMatcher{}
	parts := strings.Split(matcherString, ";")
	seenKeys := map[string]struct{}{}
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("malformed OCI descriptor matcher %q in tag: invalid part %q", matcherString, part)
		}
		if _, ok := seenKeys[key]; ok {
			return nil, fmt.Errorf("malformed OCI descriptor matcher %q in tag: duplicate key %q", matcherString, key)
		}
		switch key {
		case "mediaType":
			mt, params, err := mime.ParseMediaType(value)
			if err != nil {
				return nil, fmt.Errorf("malformed OCI descriptor matcher %q: invalid mediatype %q: %v", matcherString, value, err)
			} else if len(params) > 0 {
				return nil, fmt.Errorf("malformed OCI descriptor matcher %q: invalid mediatype %q: expected no parameters, found %v", matcherString, value, params)
			} else if value != mt {
				return nil, fmt.Errorf("malformed OCI descriptor matcher %q: invalid mediatype %q: expected %q", matcherString, value, mt)
			}
			dm.mediaType = value
		case "artifactType":
			at, params, err := mime.ParseMediaType(value)
			if err != nil {
				return nil, fmt.Errorf("malformed OCI descriptor matcher %q: invalid artifacttype %q: %v", matcherString, value, err)
			} else if len(params) > 0 {
				return nil, fmt.Errorf("malformed OCI descriptor matcher %q: invalid artifacttype %q: expected no parameters, found %v", matcherString, value, params)
			} else if value != at {
				return nil, fmt.Errorf("malformed OCI descriptor matcher %q: invalid artifacttype %q: expected %q", matcherString, value, at)
			}
			dm.artifactType = value
		case "selector":
			var err error
			dm.annotationSelector, err = labels.Parse(value)
			if err != nil {
				return nil, fmt.Errorf("malformed OCI descriptor matcher %q in tag: invalid selector %q: %v", matcherString, value, err)
			}
		default:
			return nil, fmt.Errorf("malformed OCI descriptor matcher %q in tag: unknown key %q", matcherString, key)
		}
		seenKeys[key] = struct{}{}
	}
	return dm, nil
}

func fetchConfig(ctx context.Context, fetcher content.Fetcher, manifest ocispec.Manifest) (*blob, error) {
	data, err := content.FetchAll(ctx, fetcher, manifest.Config)
	if err != nil {
		return nil, err
	}
	return &blob{descriptor: manifest.Config, data: data}, nil
}

type descriptorMatcher struct {
	mediaType          string
	artifactType       string
	annotationSelector labels.Selector
}

func (d *descriptorMatcher) Matches(desc *ocispec.Descriptor) bool {
	if desc == nil {
		return false
	}
	if d.mediaType != "" && d.mediaType != desc.MediaType {
		return false
	}
	if d.artifactType != "" && d.artifactType != desc.ArtifactType {
		return false
	}
	if d.annotationSelector != nil {
		labelSet := labels.Set(desc.Annotations)
		if !d.annotationSelector.Matches(labelSet) {
			return false
		}
	}
	return true
}

func parseArtifactFetcher(matcherString string) (*artifactFetcher, error) {
	dm, err := parseDescriptorMatcher(matcherString)
	if err != nil {
		return nil, err
	}
	return &artifactFetcher{matcher: *dm}, nil
}

type artifactFetcher struct {
	matcher descriptorMatcher
}

func (m *artifactFetcher) Fetch(manifest ocispec.Manifest) ([]ocispec.Descriptor, error) {
	var matches []ocispec.Descriptor
	for _, layer := range manifest.Layers {
		if m.matcher.Matches(&layer) {
			matches = append(matches, layer)
		}
	}
	return matches, nil
}

func parseSubjectFetcher(matcherString string) (*subjectFetcher, error) {
	dm, err := parseDescriptorMatcher(matcherString)
	if err != nil {
		return nil, err
	}
	return &subjectFetcher{matcher: *dm}, nil
}

type subjectFetcher struct {
	matcher descriptorMatcher
}

func (m *subjectFetcher) Fetch(ctx context.Context, fetcher content.Fetcher, manifest ocispec.Manifest) (*ocispec.Manifest, error) {
	if manifest.Subject == nil || m.matcher.Matches(manifest.Subject) {
		return nil, nil
	}
	return UnmarshalDescriptor[ocispec.Manifest](ctx, fetcher, *manifest.Subject)
}

func parseBlobFetcher(matcherString string) (*blobFetcher, error) {
	dm, err := parseDescriptorMatcher(matcherString)
	if err != nil {
		return nil, err
	}
	return &blobFetcher{matcher: *dm}, err
}

type blobFetcher struct {
	matcher descriptorMatcher
}

type blob struct {
	descriptor ocispec.Descriptor
	data       []byte
}

func (m *blobFetcher) Fetch(ctx context.Context, fetcher content.Fetcher, manifest ocispec.Manifest) ([]blob, error) {
	var matches []ocispec.Descriptor
	for _, layer := range manifest.Layers {
		if m.matcher.Matches(&layer) {
			matches = append(matches, layer)
		}
	}
	var blobs []blob
	for _, desc := range matches {
		data, err := content.FetchAll(ctx, fetcher, desc)
		if err != nil {
			return nil, err
		}
		blobs = append(blobs, blob{desc, data})
	}
	return blobs, nil
}

func parseAnnotationFetcher(matcherString string) (*annotationFetcher, error) {
	// key=foo
	af := &annotationFetcher{}
	parts := strings.Split(matcherString, ";")
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("malformed OCI descriptor matcher %q in tag: invalid part %q", matcherString, part)
		}
		switch key {
		case "key":
			af.key = value
		default:
			return nil, fmt.Errorf("malformed OCI descriptor matcher %q in tag: unknown key %q", matcherString, key)
		}
	}
	if af.key == "" {
		return nil, fmt.Errorf("malformed OCI descriptor matcher %q in tag: missing key", matcherString)
	}
	return af, nil
}

type annotationFetcher struct {
	key string
}

func (m *annotationFetcher) Fetch(manifest ocispec.Manifest) string {
	return manifest.Annotations[m.key]
}
