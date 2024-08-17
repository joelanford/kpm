DEMOS := $(shell find . -mindepth 2 -name Makefile -exec dirname {} \; | sort)
CLEANS := $(addsuffix .clean,$(DEMOS))

.PHONY: all $(DEMOS)
all: $(DEMOS)
$(DEMOS):
	$(MAKE) -C $@


.PHONY: clean $(CLEANS)
clean: $(CLEANS)
$(CLEANS): %.clean:
	$(MAKE) -C $* clean
