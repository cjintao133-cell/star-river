# Known Issues & Limitations (read before release)

> This project is positioned as a **kernel / framework**. Below are the **un-closed items**, stated honestly.
> **Evaluate accordingly; do not claim "production-grade secure / turnkey".** UI is out of tree (reference impl is WIP),
> see README / the `with-ui` branch.

## 1. Security: awaiting third-party review (red line 7 — must fix before release)
The following involve crypto / cgo / JNI / signing and have **not** undergone external human security review. They are
self-hardened, which is not the same as audited:
1. `kernel/store/crypto.go` — AES-GCM field encryption (AAD-bound to row id; further versioned hardening possible).
2. `kernel/secret/vault_windows.go` — Windows DPAPI key custody (atomic create; ACL to review).
3. `kernel/secret/keybackup.go` — encrypted key backup.
4. `kernel/native/vector` cgo — C-side vector index (Go-side handle/dimension hardened; C impl to review).
5. `kernel/desktopbind/jnibridge.c` and `jnibridge_kernel.c` — hand-written JNI pointer bounds.
6. `svc-dist/manifest.go` + `update.go` — manifest signing (ed25519) and updates; monotonic version to harden.
7. `kernel/native/bridge` cgo section.
> **Do not claim "military-grade / production-grade encryption" before external review completes.**

## 2. Red lines not fully closed
- **Red line 5 "true delete overwrites memory/vectors"**: delete-overwrite not yet threaded through all vector/derived data — **partial**.
- **Red line 5 "orchestrator memory outbound classification"**: the mechanism deciding which memories may leave with a request is not fully in place (current assembly path not used online).
- **Endpoint freeze / HTTPS enforcement**: "no following redirects" is implemented (prevents credential leak across domains), but full endpoint-freeze / HTTPS-only validation is still pending.

## 3. On-device unverified (needs real devices / external services)
- Background delivery rate, long-run stability (Android 72h / Windows 24h), three-tier device matrix.
- Human review of real ComfyUI / image API / selfie quality (watermark enforced, quality not manually accepted).
- Release-signed packages and channel assets (current builds are debug, not release-signed).

## 4. Reference UI (moved out of tree, WIP)
Android / desktop UI is a **reference implementation**, unpolished, with interaction bugs (e.g. tapping a character in
"Contacts" creates multiple sessions). UI is not a release-evaluation target; retrieve via the `with-ui` branch.

## 5. Functional / data-level known issues
- **Community "MCP marketplace" built-in seeds**: `tavily` is wrongly labelled "no key" — the official remote requires auth; without a key it `401`s. Forum posts are built-in demo seeds, not a real community. That page is demo-ish.
- **Gemini text-stream edge case**: some Gemini text models return 200 with an empty body via the OpenAI-compatible `/chat/completions` (image-model path unaffected, verified on real devices).
- `cmd/srchat`, `cmd/livecheck`, `cmd/stresssim` are CLI/debug tools, not product entry points.

## 6. Statement
- Local kernel + BYOK: keys only via Keystore/DPAPI, encrypted locally; vectors/memory stay on device (capability-wise).
- Crash dumps written locally only, with no network upload capability.
- Released under **Apache-2.0**, "as is", without warranty. Read this list before evaluating/using.
