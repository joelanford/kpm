################
# Build and test
################
export GO_BUILD_ASMFLAGS := all=-trimpath=$(PWD)
export GO_BUILD_GCFLAGS := all=-trimpath=$(PWD)
export GO_BUILD_LDFLAGS := -s -w
export GO_BUILD_TAGS := json1
export CGO_ENABLED := 1
GO_BUILD_FLAGS := -tags '$(GO_BUILD_TAGS)' -ldflags '$(GO_BUILD_LDFLAGS)' -gcflags '$(GO_BUILD_GCFLAGS)' -asmflags '$(GO_BUILD_ASMFLAGS)'

.PHONY: install
install:
	go install $(GO_BUILD_FLAGS) .

.PHONY: build
build:
	go build $(GO_BUILD_FLAGS) -o bin/kpm .

.PHONY: test
test:
	CGO_ENABLED=1 go test -race $(GO_BUILD_FLAGS) ./...

##########
# Release
##########
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
