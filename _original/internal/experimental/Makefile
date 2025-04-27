.PHONY: all
all: install

.PHONY: install
install:
	(cd ../../ && go mod tidy)
	go install ./cmd/bb

.PHONY: demos
demos:
	$(MAKE) -C demos
