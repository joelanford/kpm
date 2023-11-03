# `kpm`, the Kubernetes Package Manager

`kpm` is a package manager for Kubernetes. It is designed to be a simple CLI, library, and web service for managing
Kubneretes-native applications.

## Installation

```console
go install github.com/joelanford/kpm@latest
```

## Concepts

### Bundle
A `Bundle` is a versioned artifact. It has:
- basic identifying metadata:
  - `name`
  - `version`
  - `release`
- relational metadata
  - `provides`
  - `requires`
  - `conflicts`
- bundle content, define by a `mediatype`.

### Package
A `Package` is the metadata for a group of `Bundle`s, (grouped by their `name`). Package metadata
contains the `name`, `description`, `maintainers`, `categories`, and other metadata about the `Package`s.

### Catalog
A `Catalog` is simply a collection of `Package`s.

## Storage Mechanism

`kpm` uses OCI archives and registries to store and distribute packages. This allows `kpm` to leverage existing,
established, and ubiquitous technology and infrastructure for storing and distributing packages.

**NOTE**: `kpm` depends on the artifact guidance included in the OCI image spec, which is currently in release candidate
status. Not all OCI registries support the artifact spec, and `kpm` will not work with those registries.

## Roadmap

`kpm` is currently a library that defines APIs for catalogs, packages, and bundles, and helper functions that simplify
copying and pushing these custom artifacts to compliant registries and local OCI stores.

Future work will include a CLI and web service that will allow users to interact with these artifacts.
