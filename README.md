# `kpm`, the Kubernetes Package Manager

`kpm` is a package manager for Kubernetes. It is designed to be a simple CLI, library, and web service for managing
Kubneretes-native applications.

## Installation

```console
go install github.com/joelanford/kpm@latest
```

## What works-ish

1. Build a registry+v1 bundle and push it to an image registry

   ```
   kpm build bundle <bundleRoot> docker://<imageRepo>
   ```

2. Build an FBC catalog and push it to an image registry

   ```
   kpm build catalog <catalogRoot> docker://<imageRepo>
   ```

## What's next

1. Build an FBC catalog using a set of bundle files or directories as a source
    - Build and push all of the bundles
    - Generate an FBC, with channel entries based purely on the bundles' semver versions
    - Build and push the FBC
