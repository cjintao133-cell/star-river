# Star River Microkernel · Usage

This guide describes only what actually works in the **current main tree (kernel/framework)**. UI is out of tree (see FAQ).

## 0. Prerequisites

- **Go**: `go.mod` requires **go 1.26.11** (use a ≥1.26 toolchain; older versions fail to build).
- **C toolchain**: `cc`/`gcc` (builds `native/` hot paths; the pure-Go part runs without it).
- Optional: `staticcheck` (`go install honnef.co/go/tools/cmd/staticcheck@latest`).
- Android builds additionally need NDK r26+, JDK17, Gradle 8.9 (only when producing an AAR).

## 1. Get & build

```sh
git clone <repo> star-river && cd star-river

go build ./...          # compile everything
go vet ./...            # static checks
go test ./kernel/...    # kernel unit tests (add -race for concurrent packages)

# Layering guard (dependency direction)
sh scripts/check-open-split.sh

# C hot paths (optional; produces per-platform static libs under build/native/)
make -f native/Makefile all vector
```

## 2. Minimal chat (srchat)

`cmd/srchat` is a minimal CLI chat loop (minimal 7-layer context + OpenAI-compatible streaming). **The key is read only
from a file or env var — never from argv/logs.**

```sh
# A) Cloud endpoint (BYOK, any OpenAI-compatible gateway)
echo -n "sk-yourkey" > /root/myk
go run ./cmd/srchat \
  -base https://your-host/v1 \
  -model your-model-id \
  -keyfile /root/myk

# B) Key via env
SR_API_KEY=sk-yourkey go run ./cmd/srchat -base https://your-host/v1 -model your-model-id
```

Flags:

| Flag | Meaning |
|---|---|
| `-base` | **required**; OpenAI-compatible base, e.g. `https://host/v1` |
| `-model` | **required**; model id |
| `-keyfile` | API key file path (or env `SR_API_KEY`) |
| `-persona` | optional system persona prompt |
| `-once "hi"` | send one message then exit; empty → REPL (`:quit` to exit) |
| `-maxtokens` | max tokens per generation, default 1024 |
| `-maxmsgs` / `-maxchars` | context message/char budget, default 20 / 8000 |
| `-timeout` | per-request timeout, default 90s |

## 3. Fully offline (local model) — the real "pure local"

Ollama / LM Studio expose an **OpenAI-compatible API**; point `-base` at them to stay fully offline:

```sh
# Ollama example (default 127.0.0.1:11434, OpenAI-compatible prefix /v1)
ollama pull qwen2.5:7b

go run ./cmd/srchat \
  -base http://127.0.0.1:11434/v1 \
  -model qwen2.5:7b \
  -keyfile /dev/null        # local models usually don't check the key; any non-empty file works
```

> Now the prompt only travels over loopback/LAN — **never leaves the device**. This is the "local kernel + local inference" form.

## 4. Real-endpoint smoke tests (livecheck)

```sh
# Local, free: validates the watermark pipeline
go run ./cmd/livecheck -mode selftest

# Real endpoint: one chat (proves endpoint/key/protocol work)
go run ./cmd/livecheck -mode chat  -run -base https://host/v1 -model your-model -keyfile /root/myk

# Real endpoint: one image gen → mandatory watermark → assert HasAIWatermark
go run ./cmd/livecheck -mode image -run -base https://host/v1 -model your-model -imgmodel your-image-model -keyfile /root/myk
```

## 5. Import character cards

`kernel/cards` supports four carriers: V1/V2/V3 JSON, chara PNG (embedded base64, `ccv3` > `chara`), charx (zip `card.json`
or any json), and world books (`Constant` + keyword activation, depth cap + loop detection). See `kernel/cards` `Parse` & structs.

## 6. Embed into your own app (advanced)

- Android: `gomobile bind` the kernel (`kernel/mobilebind` API) → AAR.
- Windows/desktop: `go build -buildmode=c-shared` → DLL + JNI bridge (`kernel/desktopbind`).

## 7. Keys & privacy (red lines)

- Key only via `-keyfile` or `SR_API_KEY`; **not in argv/logs** (output scrubbing is a second safety net).
- Adapters **never follow redirects** (prevents credential leakage across domains).
- Data destination = the `-base` you set; see `docs/privacy-boundary.md` and `docs/product-brief.md`.

## 8. Common failures

| Symptom | Cause / fix |
|---|---|
| Build fails on Go version | Toolchain < 1.26; upgrade Go |
| "no key provided" at start | Neither `-keyfile` nor `SR_API_KEY` set |
| "must provide -base and -model" | A required flag is missing |
| Local model errors | `-base` must include `/v1`; ensure ollama is running |
| Some Gemini text models return 200 with empty body | Known edge case, see `KNOWN-ISSUES.md` |
