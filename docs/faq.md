# Star River Microkernel · FAQ

## Q1: Is it really "pure local"?
**Layer by layer**: kernel, memory, emotion, proactivity, storage, keys — **all on-device**. **Inference** sends the
conversation context to the model endpoint you configure. Cloud endpoint → it goes to the cloud; local Ollama/LM Studio
→ fully offline. The project has **no own backend, no telemetry**. So the accurate phrase is "**local kernel + BYOK
inference**", not "no data ever leaves the device".

## Q2: Can it run fully offline?
**Yes.** Attach a local model: point `-base` at `http://127.0.0.1:11434/v1` (Ollama) or a local LM Studio address. See `docs/usage.md` §3.

## Q3: Which models / providers are supported?
Three adapters: **OpenAI-compatible**, **Anthropic**, **Gemini**. So anything OpenAI-compatible works (Ollama, LM Studio,
gateways, vLLM, ...), plus the three official ones. **BYOK**: you supply the key.

## Q4: Where does my data go? Is any of it uploaded to you?
**Not to us** — there is no own server collecting data. Data only goes to **the endpoint you configure**. Crash dumps are
local-only with no network capability. See `docs/privacy-boundary.md`.

## Q5: How are keys stored / how safe is it?
- CLI: read from file or env, **not in argv/logs**; adapters don't follow redirects.
- Embedded mobile/desktop: Android Keystore, Windows DPAPI (`kernel/secret`).
- ⚠️ But the crypto/JNI implementations are **not yet externally security-reviewed** (`KNOWN-ISSUES.md` lists 8 areas).
  Don't claim "production/military-grade encryption" yet.

## Q6: Is there a UI? Can I just install and chat?
**The main tree is kernel/framework only — no product UI.** No UI is shipped in this release. Build your app on the kernel yourself; a reference UI is not part of this version.

## Q7: Does it support character cards?
Yes — four carriers: V1/V2/V3 JSON, chara PNG, charx (zip), world books (keyword activation/recursion/always-on). See `kernel/cards`.

## Q8: What category is it? How is it different from AIRI / OpenClaw / SillyTavern / Letta?
- Category: **an embeddable AI-assistant kernel** — not an app, a base you drop into your app.
- Not a web container (AIRI), not a server-side agent platform (OpenClaw), not a chat frontend (SillyTavern), not a cloud
  memory layer (Mem0/Zep), and not an inference runtime that runs models (LiteRT/llama.cpp).
- Coordinates: **around the model, inside the app** — context assembly + memory + emotion + proactivity + tools/MCP +
  multimodal + encrypted storage + dual bridges; BYOK inference, zero backend.
- Not chasing star counts; it aims to be a trustworthy, reusable foundation in that niche.

## Q9: Can it be used in production?
**This is v0.1.0-alpha.** There is hardening (redirect-disable / stress tests / real-endpoint smoke / govulncheck clean),
but `KNOWN-ISSUES.md` lists: 8 areas awaiting external security review, some red lines not fully closed, on-device
long-run unverified. **Evaluate accordingly; don't claim "turnkey / production-grade secure."**

## Q10: Is there a plugin system?
The **main line is the kernel itself, without a plugin system**. (A plugin contract/example plugins live on the separate
`plugin-wip` branch, exploratory, not merged.) On main you can extend via `bus` + `tool` + the `Adapter` interfaces.

## Q11: What license?
**Whole repo Apache-2.0**, including all kernel packages and the memory/emotion/proactivity capability plugins (all open).
The "core must not import capability plugins" rule is only an engineering **layering** constraint, unrelated to licensing.

## Q12: How to report / contribute?
After release, GitHub Issues / Discussions are the single feedback entry. For kernel issues,
attach `go build ./...`, `go vet ./...` output and repro steps.

## Q13: Why no UI, and why so much code?
The kernel thickens the foundation "long-term companionship" needs (31 packages, ~19.5k lines Go + C): multi-protocol
access, encrypted storage, context orchestration, MCP, character cards, memory/emotion/proactivity, vision, C hot paths.
The UI is just one layer on top and is independently replaceable.
