# Architecture & Package Boundaries

Kernel boundaries for the Star River Microkernel. For the honest data boundary see `privacy-boundary.md`.

## Hard facts
Local-first, no upload to our servers, BYOK. Go kernel + C hot paths + Kotlin Compose UI. Android & Windows; cross-compiled on Linux.

## Package boundaries
| Package | Responsibility | License |
|---|---|---|
| kernel/core/types | Data contracts (six types + Validate, stdlib only) | Apache-2.0 |
| kernel/bus | Event broadcast (doorbell) + direct-call hybrid | Apache-2.0 |
| kernel/adapter | openai/anthropic/gemini three protocols + discovery/classification/failover | Apache-2.0 |
| kernel/orchestrator | 7-layer context + budget allocation + rolling summary | Apache-2.0 |
| kernel/store | SQLite WAL single-writer + AES-GCM field encryption | Apache-2.0 |
| kernel/hippocampus | Forgetting-curve memory engine (capability plugin) | Apache-2.0 |
| kernel/endocrine | 5-D emotion state machine (capability plugin) | Apache-2.0 |
| kernel/proactive | Proactive scheduler (capability plugin) | Apache-2.0 |
| kernel/cards | V2/V3/chara PNG character-card parsing | Apache-2.0 |
| kernel/tool | Remote MCP + Windows stdio + built-in tools | Apache-2.0 |
| kernel/visual | Vision recipes / generation adapters / mandatory AI watermark | Apache-2.0 |
| kernel/codebrain | v1 JSON config only, v2 real code | Apache-2.0 (placeholder) |
| native/ | C: device_probe, vector search, audio preprocessing | Apache-2.0 |
| ui-android / ui-desktop | Kotlin Compose / Compose Multiplatform | Apache-2.0 (reference impl, WIP) |
| svc-dist | manifest / CDN / ed25519 signing / update chain | Apache-2.0 |

**Layering constraint**: core packages must not import capability-plugin packages (hippocampus/endocrine/proactive); the reverse is allowed. Everything is Apache-2.0.

## Communication
- Commands/queries: direct interface calls (synchronous, stack-traceable, timeout-controlled).
- State changes: `bus` broadcast. Events are just doorbells: payload carries a version, subscribers pull the latest snapshot and tolerate dropped bells.
- Backpressure: bounded buffer, drop-oldest + counter on overflow; slow subscribers never block publishers.
- v1 notes: `kernel/bus` shipped; subscribers receive all topics and filter on the consumer side; default buffer 64; `ErrClosed` semantics; drop count via `Subscription.Dropped()`.

## Dependency policy (red line 1: nothing unlisted may be added)
- Standard library first.
- `modernc.org/sqlite`: pure-Go SQLite driver, zero-cgo cross-compilation.
- `golang.org/x/mobile` (gomobile): build-chain tool.
- Build-time only: `staticcheck`, `garble` (not shipped as product deps).
- `hnswlib`: on hold; revisit only if on-device benchmarks miss targets, and only with separate approval.

## Platform differences (code must not assume uniformity)
- Android: MCP remote protocol only; built-in tools in-process; no dynamic binary download.
- Windows: stdio MCP as a separate process; tray/DPAPI behind interface abstractions with Linux mock tests.
- Capability matrix: `platform-capability.md`.

## Engineering discipline
- UI strings go through a string table, no hard-coded text (`kernel/i18n`, go:embed JSON; zh default / en complete; `{placeholders}` supported; guarded by `scripts/check-i18n-keys.sh`).
- Per-commit gate: `go vet` + `staticcheck` + relevant unit tests (concurrent packages with `-race`); core coverage ≥70%; C with ASan+Valgrind zero reports.
- Tests only hit mock servers and localhost.

## Bridges
- `kernel/mobilebind`: gomobile AAR bridge facade (thin, stable signatures; kernel logic never lives in the bridge).
- `kernel/desktopbind`: c-shared desktop export + hand-written JNI bridge (`jnibridge.c`, REVIEW-NEEDED) + minimal cross headers; thin facade reusing `mobilebind`.
