.PHONY: all
all: bundle


BB_ARGS :=
ifdef OPERATOR_IMAGE
	BB_ARGS := $(BB_ARGS) --set-image=operator=$(OPERATOR_IMAGE)
endif

.PHONY: bundle
bundle:
	bb 1.10.1.bundle.kpmspec.yaml --set-value=dist=$(DIST) $(BB_ARGS)
