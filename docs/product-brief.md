# Star River Microkernel · Product Overview

## One-liner

**An embeddable AI-assistant kernel**: it packages the foundation that gives an AI *continuity and agency* —
**context assembly, memory, emotion, proactivity, tools/MCP, multimodal adapters, encrypted local storage** —
into a kernel you can drop into your own app.

> It **does not run model weights**; it owns the layer *around the model, inside your app*. Inference is delegated
> to the endpoint you choose (cloud or local model), BYOK, no own backend.

## What it is

A microkernel written in **Go + C** (31 kernel packages + C hot paths) that does the foundation for a long-running,
stateful, pluggable AI assistant:

- **Context engine** (`orchestrator`): 7-layer context assembly + budget allocation + forced memory consolidation before truncation + rolling summary.
- **Model access** (`adapter`): OpenAI-compatible / Anthropic / Gemini + model discovery/probe/router — model-agnostic.
- **Storage** (`store`): SQLite (pure Go, no cgo) + field-level AES-GCM + versioned migrations.
- **Secrets** (`secret`): Keystore/DPAPI vault + encrypted backup + log scrubbing.
- **Tools/MCP** (`tool`/`tooldisc`): MCP client/manager (HTTP+stdio, hot-pluggable) + built-in tools + on-demand discovery for thousands of tools.
- **Pluggable capabilities**: Ebbinghaus memory (`hippocampus`), 5-D emotion (`endocrine`), proactivity scheduler (`proactive`) — each a separate, removable package.
- **Multimodal**: vision (multi-dialect image gen + mandatory watermark + degradation, `visual`), voice STT/TTS (`voice`), multi-backend search (`search`).
- **Support**: backup import/export (`backup`), local diagnostics (`diag`), compliance switch (`compliance`), i18n (`i18n`), safe outbound HTTP (`netx`).
- **Dual-end bridges**: `mobilebind` (gomobile → Android AAR, 71 exported functions), `desktopbind` (c-shared DLL + JNI).
- **C hot paths**: device probe, int8/NEON vector search (`native/`).

## What it is NOT (honest boundaries)

| Don't mistake it for | Real difference |
|---|---|
| An inference runtime (llama.cpp / LiteRT / RunAnywhere) | It does **not** run models; it **orchestrates** them |
| A memory add-on (Mem0 / Zep) | Not just "memory bolted on" — a **full assistant brain** |
| A stateful agent platform (Letta) | An **embeddable Go kernel**, not a Python server-side agent |
| A generic AI SDK (Vercel/GoAI) | Not just "call the model" — **state/memory/multimodal/encrypted storage/dual bridges** |
| An on-device companion SDK (NVIDIA ACE) | No GPU/game-engine lock-in; pure Go+C, cross-platform |

## Honest data boundary (read first)

| Data | Where | Leaves the device? |
|---|---|---|
| Sessions/messages, long-term memory, vector index | Local device (SQLite, message body AES-GCM encrypted) | No |
| Emotion/proactivity state, keys | Local (keys in Keystore/DPAPI) | No |
| Crash dump | Local file, **no network capability** | No |
| **Conversation context (prompt)** | Sent to **the model endpoint you configure** | **Yes** (to that endpoint) |
| **Image-gen prompt** | Sent to the image endpoint you configure | **Yes** |

**In one line**: no own backend, no telemetry; data only goes to **the endpoint you configure**. Point it at a cloud
endpoint → data goes there; point it at a local Ollama → fully offline.

## Fit / not a fit

- ✅ Embed a stateful AI assistant (persistent memory/emotion/proactivity) in your own app, controlling data and keys.
- ✅ Run local / on-prem, even fully offline (with a local model).
- ✅ Use a clean Go kernel as a base to build your UI / app / service on.
- ❌ Want a turnkey chat app (no product UI is included).
- ❌ Require "production-grade audited security" (see `KNOWN-ISSUES.md`; 8 areas await external security review).

## Size (verifiable)

Go production ≈ 19.5k lines / 140 files; C/H ≈ 0.9k lines; 31 kernel packages + 3 commands.
Per-module capabilities: see [`features.md`](./features.md).
