package v1alpha1

import (
	"encoding/json"
	"fmt"
	"regexp"
)

type Release struct {
	raw string
}

func ParseRelease(raw string) (Release, error) {
	if err := validateRawRelease(raw); err != nil {
		return Release{}, err
	}
	return Release{raw: raw}, nil
}

func MustParseRelease(raw string) Release {
	r, err := ParseRelease(raw)
	if err != nil {
		panic(err)
	}
	return r
}

func (r *Release) String() string {
	return r.raw
}

// MarshalJSON implements the encoding/json.Marshaler interface.
func (r Release) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

// UnmarshalJSON implements the encoding/json.Unmarshaler interface.
func (r *Release) UnmarshalJSON(data []byte) (err error) {
	var releaseString string
	if err = json.Unmarshal(data, &releaseString); err != nil {
		return
	}

	*r, err = ParseRelease(releaseString)

	return
}

const releasePattern = `^[A-Za-z0-9]+([.+-][A-Za-z0-9]+)*$`

var releaseRegex = regexp.MustCompile(releasePattern)

func validateRawRelease(raw string) error {
	if !releaseRegex.MatchString(raw) {
		return fmt.Errorf("invalid release %q: does not match pattern %s", raw, releasePattern)
	}
	return nil
}
