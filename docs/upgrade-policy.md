# Upgrade & Maintenance Policy

## Supported versions

- The latest `0.x` release line (currently `0.1.x`, alpha) receives best-effort security fixes.
- Older alpha versions are not separately maintained; users are expected to upgrade to the latest
  `0.1.x`. Because the project is pre-1.0, minor versions may include breaking API changes.

## Upgrade path

- **Go consumers:** pin a tagged version and upgrade with the standard Go toolchain
  (`go get github.com/cjintao133-cell/star-river@<version>`). API changes between alpha versions are
  documented in release notes; `go.mod` / tests show the supported surface.
- **Native (C) artifacts:** rebuild from tagged source with the provided `native/Makefile`; no
  binary compatibility is promised across versions while pre-1.0.
- **Local data:** the SQLite schema is migrated forward automatically on open. Back up the database
  file before a major upgrade; a downgrade may not be able to read a newer schema.

## Breaking changes

While the project is `0.x`, breaking changes are allowed but MUST be called out in the release
notes with migration steps. Once `1.0` is reached, semantic versioning applies to the public API.
