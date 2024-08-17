# `kpm`, the Kubernetes Package Manager

`kpm` is a package manager for Kubernetes. It is designed to be a simple CLI for managing
Kubneretes-native applications.

To begin, it focuses on OLM's registry+v1 bundle and FBC catalog formats, but the intention
is to expand it to support more Kubernetes-native application artifacts (e.g. helm charts)

## Installation

```console
go install github.com/joelanford/kpm@latest
```

## What works-ish

1. Build a registry+v1 bundle and push it to an image registry

   ```
   $ cat << EOF > bundle.kpmspec.yaml
   apiVersion: specs.kpm.io/v1
   kind: Bundle
   registryNamespace: quay.io/joelanford
   bundleRoot: ./my-olm-package/0.0.1
   EOF

   $ kpm build bundle bundle.kpmspec.yaml
   Bundle written to my-olm-package-v0.0.1.bundle.kpm with tag "quay.io/joelanford/my-olm-package-bundle:v0.0.1" (digest: sha256:fe54318b20e00a37337058b569490e2e2df29fbaaf1af4761a11929e6d364ace)

   $ kpm push my-olm-package-v0.0.1.bundle.kpm
   pushed "my-olm-package-v0.0.1.bundle.kpm" to "quay.io/joelanford/my-olm-package-bundle:v0.0.1" (digest: sha256:fe54318b20e00a37337058b569490e2e2df29fbaaf1af4761a11929e6d364ace)
   ```

2. Build a catalog from an existing FBC and push it to an image registry

   ```
   $ cat << EOF > catalog.kpmspec.yaml
   apiVersion: specs.kpm.io/v1
   kind: Catalog
   tag: "quay.io/joelanford/kpm-example-catalog:fbc"
   source:
     sourceType: fbc
     fbc:
       catalogRoot: ./catalog/
       cacheFormat: json
   EOF

   $ kpm build catalog catalog.kpmspec.yaml
   Catalog written to kpm-example-catalog-fbc.catalog.kpm with tag "quay.io/joelanford/kpm-example-catalog:fbc" (digest: sha256:0e258e37001a40fca5d627e87142b791ef1c5aa0cc780e5bd39e3251e52901d2)

   $ kpm push kpm-example-catalog-fbc.catalog.kpm
   pushed "kpm-example-catalog-fbc.catalog.kpm" to "quay.io/joelanford/kpm-example-catalog:fbc" (digest: sha256:0e258e37001a40fca5d627e87142b791ef1c5aa0cc780e5bd39e3251e52901d2)
   ```

3. Bundle a catalog from an FBC template and push it to an image registry

   ```
   $ cat << EOF > semver.yaml
   schema: olm.semver
   generateMajorChannels: true
   stable:
     bundles:
     - image: ./my-olm-package-v0.0.1.bundle.kpm
     - image: ./my-olm-package-v0.1.0.bundle.kpm
     - image: ./my-olm-package-v1.0.0.bundle.kpm
     - image: ./my-olm-package-v1.1.0.bundle.kpm
     - image: ./my-olm-package-v2.0.0.bundle.kpm
   EOF

   $ cat << EOF > catalog.kpmspec.yaml
   apiVersion: specs.kpm.io/v1
   kind: Catalog
   tag: "quay.io/joelanford/kpm-demo-catalog:semver-migrated"
   source:
     sourceType: fbcTemplate
     fbcTemplate:
       templateFile: semver.yaml
       migrationLevel: bundle-object-to-csv-metadata
       fbcCacheFormat: pogreb.v1
   EOF

   $ kpm build catalog catalog.kpmspec.yaml
   Catalog written to kpm-demo-catalog-semver-migrated.catalog.kpm with tag "quay.io/joelanford/kpm-demo-catalog:semver-migrated" (digest: sha256:35fdf36ca04dc412fbed4c23573814c79319cce6723a97136c4a3ad5aeff4941)

   $ kpm push kpm-example-catalog-fbc.catalog.kpm
   pushed "kpm-demo-catalog-semver-migrated.catalog.kpm" to "quay.io/joelanford/kpm-demo-catalog:semver-migrated" (digest: sha256:35fdf36ca04dc412fbed4c23573814c79319cce6723a97136c4a3ad5aeff4941)
   ```

4. Render a KPM file

   ```
   $ kpm render my-olm-package-v0.0.1.bundle.kpm -o yaml
   schema: olm.bundle
   package: my-olm-package
   name: my-olm-package.v0.0.1
   image: quay.io/joelanford/my-olm-package-bundle:v0.0.1
   ...
   ```

