.PHONY: install
install:
	CGO_ENABLED=1 go install .

.PHONY: build
build:
	go build -o bin/kpm .

.PHONY: test
test:
	go test ./...
