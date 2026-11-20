# Ecosystem Ingestion · Design & Red Lines

> Status: v1, 2026-11-20. Answers a frequent request: "Can I ingest another open-source framework's plugin ecosystem and
> turn its plugins into MCP tools I can use?"
> Bottom line up front: **you can only ingest MCP-native capabilities + data formats; a framework's private plugins cannot be
> "auto-converted" — you can only write a compatibility layer or rewrite, and most only run on desktop/server.**

## 0. One-line principle

**Ingest the ecosystem via the MCP boundary.** Plugins are always external processes — optional, disableable, sandbox-isolated.
Never bring a second runtime (Python/Node) onto the device to "eat plugins"; never swallow license-infectious code.

## 1. Two things not to confuse: plugins ≠ MCP tools

- **A framework's "plugin"**: code bound to that framework's internal API (Python/JS), hooking the host's message pipeline, DB, even DOM. **Not a standalone runnable thing.**
- **An MCP tool**: a capability exposed over inter-process JSON-RPC, callable by any MCP client. **A different layer.**

The two are not the same layer. There is **no** universal converter that turns "framework-private plugins" into MCP.

Fact check (2026-11): AstrBot v3.5.0+ natively supports MCP, but its **1000+ plugins are Python extensions**, not MCP.
SillyTavern can act as an MCP client via community extensions, but those extensions are JS.

## 2. Four ingestion levels (graded by "can / how safe")

### ① MCP-native → usable today, zero code (main path)
Any capability published as an MCP server (Tavily/Exa/Firecrawl/filesystem/browser/database/**Baidu search MCP**…) plugs
straight into `kernel/tool` (the MCP host). Two-stage on-demand discovery for thousands of tools + hot-plug is already there.
- Transport: Streamable HTTP / SSE / stdio (stdio desktop/Windows only).
- **Zero extra runtime on device** — naturally satisfies the red line.

### ② Data / format import → doable now, safe
Character-card V2, world books, presets, theme packs — **data, not code**. SillyTavern cards/world books import directly.
- Read only from **user-selected local files**; remote avatar references use a placeholder, **no request made** (local-only constraint).

### ③ Compatible MCP server → doable, but a per-framework shim, high cost
For host-internal plugins like AstrBot (Python) / SillyTavern (JS extensions): write a **separate-process** "compatibility server"
that loads that framework's plugins and exposes them as MCP tools.
Costs (be aware):
- Must drag that framework's runtime (Python/Node) → **violates the "device-side pure Go+C" red line**; desktop/server-side only.
- Plugins often hook host internals (message flow/DB/DOM); **may not run once extracted.**
- Running arbitrary third-party code → security/sandbox pressure maxed.
**Do it only for "the worthwhile few" plugins, case by case; never a universal ingest; never into the Android kernel.**

### ④ Source port → not automatic, manual rewrite
To truly match a capability, an honest rewrite is most reliable — just costs people.

## 3. The moat is the reverse direction: expose your capabilities as MCP servers
Export character/memory/emotion/proactivity/vision **as MCP servers**, so SillyTavern / AstrBot / Claude Desktop and other
MCP clients can call **our** sample characters and memory engine. **Being a standard provider is worth far more than being an
ecosystem vacuum cleaner.**

## 4. Red lines (crossing = violation)
1. **No second runtime on device**: don't stuff Python/Node into the Android kernel.
2. **Arbitrary third-party code must be external process + sandbox-isolated + optional/disableable.**
3. **License infection**: GPL/AGPL — client connection only, never swallow the code (same as the SearXNG principle).
4. **Local-first**: imports never auto-fetch remote resources; avatar references use placeholders.
5. **Keys never in URLs/logs**: MCP server keys are entered in settings and stored encrypted.
6. **Don't break layering**: the core (incl. `kernel/tool` MCP host) imports no specific framework/plugin adapter code; compatibility layers live in the app-assembly layer or a separate process.

## 5. Where it lives (UI)
Community page → MCP marketplace → **Ecosystem ingestion** section, three tiers visualized: MCP-native / compatible server /
format import. Make it clear "what I support and don't", killing the "universal converter" fantasy.
