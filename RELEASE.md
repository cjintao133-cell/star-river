# Star River Microkernel · Release Notes

> **An embeddable AI-assistant kernel / framework** (Go kernel + C hot paths). BYOK, no own backend.
> This repo focuses on the **kernel & protocols**; UI is a reference implementation, moved out of tree (WIP).

## What this is
An embeddable AI-assistant **kernel**: a general core (event bus / multi-protocol adapters / SQLite storage / tools & MCP /
7-layer context orchestration) + pluggable "capability plugin" packs (Ebbinghaus memory / 5-D emotion / proactivity scheduler).
The core stays clean and compiles standalone; data & keys stay local, credentials only via platform Keystore/DPAPI.

## Repository contents
```
kernel/     Go kernel (26 packages): adapter(openai/anthropic/gemini/router) orchestrator store
            tool(MCP) cards bus hippocampus endocrine proactive visual voice search secret
            backup diag compliance i18n netx native(vector) mobilebind desktopbind …
native/     C/H (device_probe, vector, etc.), with a Makefile for three-platform static libs
cmd/        srchat / livecheck / stresssim (CLI & debug tools)
scripts/    check-open-split.sh (layering guard) etc.
docs/       product / usage / faq / features / troubleshooting / architecture / licensing / privacy
```
Size: Go production ≈ 19.5k lines / 139 files; C/H ≈ 0.9k lines.

## Quick start (kernel only)
```bash
# Requires: Go; C toolchain (Makefile builds three-platform static libs)
go build ./...
go test ./kernel/...
staticcheck ./...
make -f native/Makefile all vector     # -> build/native/*/lib*.a
bash scripts/check-open-split.sh        # layering guard (core must not depend on capability plugins), should PASS
```

## About the UI
Android (Kotlin Compose) / Windows desktop UI are a **reference implementation**, moved out of tree (still WIP).
To get them: `git checkout with-ui` (or tag `pre-ui-strip`); AAR/DLL built via gomobile / c-shared.

## Known issues
**Please read [`KNOWN-ISSUES.md`](KNOWN-ISSUES.md) before release**: crypto/JNI boundaries awaiting third-party review,
un-closed red lines, on-device-unverified list, etc.

## License
Apache License 2.0 (see `LICENSE`). Provided "as is", without warranty.
