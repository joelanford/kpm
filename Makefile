export VERSION := $(shell git describe --tags --always --dirty)

export GO_BUILD_ASMFLAGS := all=-trimpath=$(PWD)
export GO_BUILD_GCFLAGS := all=-trimpath=$(PWD)
export GO_BUILD_LDFLAGS := -s -w
export GO_BUILD_TAGS :=

.PHONY: install
install:
	CGO_ENABLED=0 go install .

.PHONY: build
build:
	go build -o bin/kpm .

.PHONY: test
test:
	go test ./...

.PHONY: demos
demos:
	$(MAKE) -C demos

ifeq ($(origin IMAGE_REPO), undefined)
IMAGE_REPO := ghcr.io/joelanford/kpm
endif
export IMAGE_REPO

ifeq ($(origin IMAGE_TAG), undefined)
IMAGE_TAG := devel
endif
export IMAGE_TAG

ifeq ($(origin ENABLE_RELEASE_PIPELINE), undefined)
ENABLE_RELEASE_PIPELINE := false
endif
export ENABLE_RELEASE_PIPELINE

ifeq ($(origin GORELEASER_ARGS), undefined)
GORELEASER_ARGS := --snapshot --clean
endif
export GORELEASER_ARGS

.PHONY: release
release:
	go tool goreleaser release $(GORELEASER) $(GORELEASER_ARGS)
