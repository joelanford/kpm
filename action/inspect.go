package action

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"oras.land/oras-go/v2"
)

type Inspect struct {
	Target oras.ReadOnlyTarget
	Tag    string
	Output io.Writer
}

func (i *Inspect) Run(ctx context.Context) error {
	desc, err := i.Target.Resolve(ctx, i.Tag)
	if err != nil {
		return fmt.Errorf("resolve tag %q: %w", i.Tag, err)
	}
	rc, err := i.Target.Fetch(ctx, desc)
	if err != nil {
		return fmt.Errorf("fetch data for tag %q with digest %q: %w", i.Tag, desc.Digest, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read data for tag %q with digest %q: %w", i.Tag, desc.Digest, err)
	}

	enc := json.NewEncoder(i.Output)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(json.RawMessage(data)); err != nil {
		return fmt.Errorf("print data for tag %q with digest %q: %w", i.Tag, desc.Digest, err)
	}
	return nil
}
