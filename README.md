# Star River Microkernel

[![CI](https://github.com/cjintao133-cell/star-river/actions/workflows/ci.yml/badge.svg)](https://github.com/cjintao133-cell/star-river/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cjintao133-cell/star-river.svg)](https://pkg.go.dev/github.com/cjintao133-cell/star-river)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26.11+-00ADD8.svg)](go.mod)
[![OpenSSF Best Practices — Silver](https://www.bestpractices.dev/projects/16960/badge)](https://www.bestpractices.dev/projects/16960)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/cjintao133-cell/star-river/badge)](https://scorecard.dev/viewer/?uri=github.com/cjintao133-cell/star-river)

**A runtime for AI apps — not another AI app.**

Star River is an embeddable kernel in Go — a pure-Go core, plus one small optional C module (a SIMD int8 vector index). It owns the layer *around* the model — context assembly,
memory, emotion, tools/MCP, multimodal adapters, encrypted local storage — so your app doesn't have to.
You bring the model (BYOK); it brings everything around it. No Python, no container, no backend of ours, no telemetry.

**Why Go, and why small:**

- **Small & self-contained** — ~19.5k LOC Go; a single static binary at **~6.7 MB** (`-ldflags "-s -w"`, pure-Go build without the optional C vector index). **One** direct dependency (`modernc.org/sqlite`, pure Go) — no interpreter, no venv, no 2 GB image.
- **Fast cold start** — milliseconds, no interpreter/runtime warm-up. Fits CLIs, edge boxes, serverless.
- **Bounded memory** — long-running memory state stays bounded; a **365-day** fake-clock stress test asserts it (no unbounded "state explosion").
- **Channel/goroutine-native** — the event bus is channel-based and MCP stdio processes run as goroutines; no GIL.
- **Well-tested** — **92% average statement coverage across kernel packages** (statement-weighted over the whole tree incl. CLI entry binaries: 81.7%); 680+ test/fuzz/benchmark functions, `-race` on concurrent packages, coverage gated in CI.
- **Cross-compiles** — one command to Linux / Windows / macOS / arm64; Android via `gomobile bind` → AAR (needs NDK).

*Scope of C, honestly: ~0.9k lines of C (excluding build-generated headers) for the optional SIMD int8 vector index, device probing, and the JNI / c-shared bridge. The core logic is pure Go.*

```go
import (
    "github.com/cjintao133-cell/star-river/kernel/hippocampus"
    "github.com/cjintao133-cell/star-river/kernel/endocrine"
)
mem := hippocampus.New(hippocampus.Options{})   // forgetting-curve memory
emo := endocrine.New(nil, nil)                  // pluggable 5-D emotion
mem.Encode("m1", "user prefers Go", 0.9, 0.7)  // remember (text, importance, emotion)
```

> **It does not run model weights — it owns the layer *around* the model, *inside* your app.**
> Memory / emotion / proactivity are pluggable capabilities, not its definition.

> **Status**: this repo focuses on the **core / framework**. The Android & desktop **UI lives out of the
> main tree** (reference implementation, WIP). Evaluate the project by its **kernel architecture / protocols / data security**.

> Docs: [Product](./docs/product-brief.md) · [Features](./docs/features.md) · [Usage](./docs/usage.md) · [FAQ](./docs/faq.md) · [Troubleshooting](./docs/troubleshooting.md) — 中文文档见 [README.zh.md](./README.zh.md) / `docs/*.zh.md`

## See it work (offline, ~5 seconds, no key)

```sh
go run ./examples/hello-kernel
```
It runs the core loop locally — event bus → memory (forgetting-curve recall) → emotion (5-D state):
```
[bus]      received doorbell  topic="kernel.started" source="hello-kernel"
[memory]   recall   "the user is building a local-first AI kernel"  score=1.53 retention=1.00
[emotion]  arousal 0.30 -> 0.70   valence 0.00 -> 0.30
```

## Architecture


```
        ┌────────────────────────── App layer (UI) ─────────────────────────┐
        │   Android: Kotlin Compose      Windows: Compose Multiplatform      │
        └───────────────▲───────────────────────────────▲──────────────────┘
                        │ gomobile AAR / c-shared DLL + JNI
        ┌───────────────┴───────────────────────────────┴──────────────────┐
        │                    Go kernel (star-river-kernel)                  │
        │  core/types  core/clock  bus   adapter(openai/anthropic/gemini)  │
        │  orchestrator(7-layer context)  store(SQLite WAL+AES-GCM)        │
        │  tool(remote/stdio MCP + builtins)  cards(character cards)       │
        │  ────────────── capability plugins (pluggable) ──────────────    │
        │  hippocampus(memory)  endocrine(emotion)  proactive(scheduler)   │
        └───────────────▲──────────────────────────────────────────────────┘
                        │ cgo
        ┌───────────────┴───────────────┐
        │  C hot paths: device_probe / vector search (int8·NEON)  │
        └────────────────────────────────┘
```

- **Communication**: commands/queries go through direct interface calls; state changes go through the `bus`
  doorbell broadcast (subscribers pull the latest snapshot, tolerate dropped bells, bounded-buffer backpressure).
- **Layering constraint (dependency direction)**: core packages **must not** import capability-plugin packages;
  plugins may import the core. Enforced by `scripts/check-open-split.sh` (the core compiles standalone).

## Repository layout

```
star-river/
  kernel/       Go kernel
    core/{types,clock}  bus  adapter/  orchestrator/  store/  secret/
    cards/  tool/  tooldisc/  visual/  voice/  search/  backup/  diag/  compliance/  i18n/  netx/
    native/{bridge,vector}  mobilebind/  desktopbind/
    hippocampus/ endocrine/ proactive/        capability plugins
  native/       C modules (device_probe, vector search)
  cmd/srchat/       minimal CLI chat loop
  cmd/livecheck/    real-endpoint smoke tests
  cmd/stresssim/    year-scale stress simulation
  docs/             product/usage/faq/features/troubleshooting/architecture/licensing/privacy (see docs/README.md)
  scripts/          build & guard scripts
```

## Quick start

Prerequisites: **Go 1.26.11+**. The pure-Go packages (`bus`, memory, emotion, `orchestrator`, `cards`, `store`, …) build with `CGO_ENABLED=0` — **no C toolchain required**. A C toolchain is only needed for the full `./...` build and the optional native modules (SIMD vector index, desktop JNI bridge); Android `gomobile bind` also needs NDK r26+/JDK17/Gradle 8.9.

```sh
# Full gate (build / vet / race tests)
go build ./...
go vet ./...
staticcheck ./...
go test -race ./...

# Layering guard (core must not depend on capability plugins)
sh scripts/check-open-split.sh

# Minimal CLI chat (key only via -keyfile or SR_API_KEY; never in argv/logs)
go run ./cmd/srchat -base https://host/v1 -model <model> -keyfile /path/to/key
```

Build artifacts: Android `gomobile bind` → AAR; Windows/desktop `-buildmode=c-shared` → DLL + JNI bridge.

## Character cards

Four carriers are supported (`kernel/cards`):

- **V2 / V3 JSON**: `spec` or `spec_v2` + `data` nested structure.
- **chara PNG**: base64 card JSON embedded in PNG `tEXt`/`iTXt` chunks (`ccv3` preferred over `chara`).
- **charx**: `card.json` inside a zip (falls back to any json).
- World-book entries support keyword activation (`Constant` always-on + keyword activation, depth cap + loop detection).

## Dependency policy

Nothing outside the `docs/architecture.md` dependency list may be introduced. Current deps only:
standard library first; `modernc.org/sqlite` (pure-Go SQLite, no cgo); `golang.org/x/mobile` (build chain).
No third-party SDKs; agent protocols (OpenAI/Anthropic/Gemini/MCP) are hand-written.

## Contributing

- Per-commit gate: clean `gofmt` + `go vet` + `staticcheck` + relevant unit tests (concurrent packages with `-race`);
  kernel-package statement-weighted coverage ≥85% (CI-enforced; per-package average ~92%); C code passes ASan+Valgrind with zero reports.
- The red lines in `docs/architecture.md` are non-negotiable, especially dependency direction (core does not import
  capability plugins) and keys never appearing in logs/URLs.
- Security-sensitive code (crypto, cgo handles, signature chain) is tagged `REVIEW-NEEDED` and requires external review before release.

## License

- **Whole repo under Apache-2.0** (see `LICENSE` / `docs/licensing.md`), including all kernel packages and the
  memory/emotion/proactivity capability plugins.

## Status

Kernel in development (open items and on-device-unverified items in `KNOWN-ISSUES.md`; doc index in `docs/README.md`).
UI polish and on-device acceptance are handled together in a later phase.
