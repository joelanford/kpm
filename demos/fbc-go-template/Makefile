.PHONY: all
all:
	mkdir -p kpms/basic && kpm build catalog basic.kpmspec.yaml -o kpms/basic
	mkdir -p kpms/semver && kpm build catalog semver.kpmspec.yaml -o kpms/semver

.PHONY:
clean:
	rm -rf kpms
