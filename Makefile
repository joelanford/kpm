.PHONY: install
install:
	go install .

.PHONY: build
build:
	go build -o bin/kpm .

.PHONY: test
test:
	go test ./...
