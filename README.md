# `kpm`, the Kubernetes Package Manager

`kpm` is a package manager for Kubernetes. It is designed to be a simple CLI for managing
Kubernetes-native application packaging.

To begin, it focuses on OLM's registry+v1 bundle format, but the intention is to expand it
to support more (FBC, Helm, and other Kubernetes-native application artifacts)

## Installation

```console
go install github.com/joelanford/kpm@latest
```

## OLM `registry+v1` bundles

1. Build the bundle into a `kpm` file:

   ```console
   $ cat << EOF > bundle.kpmspec.yaml
   apiVersion: specs.kpm.io/v1alpha1
   kind: RegistryV1
   
   source:
     sourceType: BundleDirectory
     bundleDirectory:
       path: ./bundle
   EOF
   
   $ kpm build ./bundle.kpmspec.yaml
   Bundle my-operator.v0.1.0 written to my-operator.v0.1.0.kpm (digest: sha256:ebd8006b0bee0e1b0b26b313b21213229c436498a6ad023d3bbb561abfccb815)
   ```

2. Push the `kpm` file to an image registry:

   ```console
   $ skopeo copy oci-archive:./my-operator.v0.1.0.kpm docker://quay.io/my-org/my-operator:v0.1.0
   Getting image source signatures
   Copying blob 62ef5b1cfc03 done   |
   Copying config 34b12195df done   |
   Writing manifest to image destination
   ```