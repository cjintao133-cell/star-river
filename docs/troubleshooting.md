# Star River Microkernel · Known Issues / Risks & Solutions

> For people who will actually run it: the pitfalls, the unverified risks, and the fixes.
> Complements `KNOWN-ISSUES.md` (release-blocking security/red-line list); this doc is more about **usage troubleshooting**.

---

## A. Build & environment

| Issue | Cause | Fix |
|---|---|---|
| `go build` toolchain version error | `go.mod` requires **Go 1.26.0** | Upgrade to Go ≥1.26; don't downgrade `go.mod` |
| `go test -race ./...` slow/timeout | race detector overhead | Only `-race` the concurrent packages (bus/orchestrator/proactive), or raise timeout |
| C errors / `make -f native/Makefile` fails | no C toolchain | install `gcc`/`clang`; the pure-Go part runs without native |
| `staticcheck` not found | not installed | `go install honnef.co/go/tools/cmd/staticcheck@latest` |
| `check-open-split.sh` fails | a core package imports a capability plugin | fix per the reported line: core only depends on core (layering, not licensing) |

## B. Connecting models (most common)

| Issue | Cause | Fix |
|---|---|---|
| `must provide -base and -model` | missing required flags | provide both; `-base` like `https://host/v1` |
| `no key provided` | neither `-keyfile` nor `SR_API_KEY` | provide one |
| Local Ollama errors / 404 | missing OpenAI-compat prefix or service down | `-base http://127.0.0.1:11434/v1` (**include `/v1`**); `ollama serve` + `ollama pull <model>` |
| Some local frameworks require a key | they validate non-empty | give `-keyfile` any non-empty file |
| Some Gemini text models return 200 with empty body | known edge (via OpenAI-compat `/chat/completions` streaming) | use the native `adapter/gemini` path or another model; image path unaffected |
| Gateway 302/redirect then fails | the kernel **does not follow redirects** (security red line) | use the **final endpoint**, not redirecting short/marketing URLs |
| Model listing fails/timeout | probe timeout or endpoint lacks `/v1/models` | pass `-model` manually, or use an endpoint that supports `/v1/models` |
| Returned model differs from request | gateway substitution | `adapter/discovery` `ModelMismatch` flags it; switch to a trusted endpoint |

## C. Character card import

| Issue | Cause | Fix |
|---|---|---|
| PNG card not parsed | non-standard `tEXt/iTXt`, or compressed chunk | use `Parse` auto-detect; if needed, export V2 JSON and re-import |
| charx "card.json not found" | zip layout unexpected | it falls back to any json; else unzip and use `ParseJSON` manually |
| World book not triggering | keywords/`Constant`/depth config | check `BookEntry.Keys`, `Constant`, recursion depth cap (default 3) |

## D. Memory / emotion / proactivity (behavior)

| Symptom | Explanation | Handling |
|---|---|---|
| Memory "not recalled" | recall threshold (default 0.15); strength decays | raise importance/emotion for that entry, or `Review` to reinforce — by design |
| Emotion drifts back to baseline | `Tick` decays exponentially to baseline | normal; use `Interact` to keep it up |
| Proactivity "too noisy" / "silent" | three gates: dedup / rate limit (3/h, 8/day) / quiet (23–7) | tune `Config`: `Enabled`, `RateLimit`, `QuietHours`, `DefaultCooldown` |
| Proactivity never fires | `Config.Enabled` zero value = false (off by default, revocable) | `SetEnabled(true)` and enable the relevant `SourceKind` |

## E. Security implementation (not yet externally reviewed — important)

Hardened but **not externally audited** (see `KNOWN-ISSUES.md` §1):

| Module | Status | Action |
|---|---|---|
| `store/crypto.go` AES-GCM | AAD-bound to row id, hardened | external crypto review before launch |
| `secret/vault_windows.go` DPAPI | atomic create, ACL to review | external review of ACL & failure paths |
| `secret/keybackup.go` | user-password-derived backup | review KDF params & salt mgmt |
| `native/vector` cgo | Go-side handle/dimension validated | C side to review; boundary-test untrusted input |
| `desktopbind/jnibridge*.c` | hand-written JNI pointer bounds | external review + fuzz/ASan |
| `svc-dist` ed25519 manifest | signing present | harden monotonic version (anti-rollback) |
| `native/bridge` cgo | — | review |

> **Don't claim "production/military-grade encryption" before external review.**

## F. Unverified / unknown risks (honest list)

- **On-device long-run unverified**: Android 72h, Windows 24h background delivery stability need real devices.
- **No release-signed build yet**: debug builds only, not release signing.
- **Image quality not human-reviewed**: watermark enforced, but real selfie/image-gen aesthetics not manually accepted.
- **Red lines not fully closed** (partial): delete-overwrite not threaded through all vector/derived data; memory "outbound
  classification" incomplete; endpoint freeze/HTTPS-only full validation pending.
- **UI not in this repo**: reference impl moved out (WIP), see `with-ui`; not a release-evaluation target.

## G. General troubleshooting flow

1. **Pass the gate first**: `go build ./... && go vet ./... && go test ./kernel/...` — is it env or code?
2. **Minimal repro for access issues**: `go run ./cmd/livecheck -mode chat -run -base ... -model ... -keyfile ...` — isolate key/endpoint/protocol.
3. **Scrubbing**: errors are redacted (`cleanOut`/`Redact`/`Scrub`); never paste raw keys in issues.
4. **Diagnostics**: crashes only write a local dump (`diag`); you decide whether to attach it to an issue.
