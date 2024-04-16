package transport

import (
	"errors"
	"fmt"
	"github.com/joelanford/kpm/internal/remote"
	"strings"
)

func init() {
	all.Register(&DockerTransport{})
}

type DockerTransport struct{}

func (t *DockerTransport) ParseReference(ref string) (Target, error) {
	if !strings.HasPrefix(ref, "//") {
		return nil, errors.New("invalid reference: expected docker://<repo>[:tag]")
	}
	repo, tag, _ := strings.Cut(strings.TrimPrefix(ref, "//"), ":")

	target, err := remote.NewRepository(repo)
	if err != nil {
		return nil, err
	}
	return &DockerTarget{
		Repo: repo,
		ORASTarget: &ORASTarget{
			Remote: target,
			Tag:    tag,
		},
	}, nil
}

func (t *DockerTransport) Protocol() string {
	return "docker"
}

type DockerTarget struct {
	Repo string
	*ORASTarget
}

func (t *DockerTarget) String() string {
	return fmt.Sprintf("docker://%s", t.Repo)
}
