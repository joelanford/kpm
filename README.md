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
   cd <bundleRoot>
   kpm build bundle --destination=docker://<imageRepo>
   ```

2. Build an FBC catalog and push it to an image registry

   ```
   cd <catalogRoot>
   kpm build catalog --destination=docker://<imageRepo>
   ```
