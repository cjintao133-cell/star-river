# Star River Microkernel · Features

> Every capability below is implemented in code; each maps to a package under `kernel/`.
> Positioning: **an embeddable AI-assistant kernel** (Go + C, Android/Windows). It does not run model weights — it owns
> the layer *around the model, inside the app*: context assembly, memory, emotion, proactivity, tools/MCP, multimodal
> adapters, encrypted storage, dual-end bridges.
> Size: Go production ≈ 19.5k lines / 140 files; C/H ≈ 0.9k lines; 31 kernel packages + 3 commands.

---

## 1. Model access (`kernel/adapter`)

| Sub-package | Capabilities (exported) |
|---|---|
| `adapter/openai` | `New(baseURL, key)` → `Client`; `ChatRequest/ChatResult/Usage/ToolDef/Citation/ModelInfo`; streaming (`core.StreamEvent`). Any OpenAI-compatible endpoint (Ollama `http://127.0.0.1:11434/v1`, LM Studio, vLLM, gateways). |
| `adapter/anthropic` | `New` → Anthropic Messages protocol (`x-api-key` + `anthropic-version`). |
| `adapter/gemini` | `New` → Gemini protocol. |
| `adapter/discovery` | Model catalog discovery/classification: `Discover`, `Classify`, `BuildCards`, `HeuristicCaps`, `LazyValidate`, `ModelMismatch`; `Capability`/`CapabilityCard`/`Catalog`. |
| `adapter/probe` | Capability probing: `Caps`, `Signal`, `AsProbeFn`. |
| `adapter/router` | Multi-endpoint routing + failover: `New`, `WithFailoverPredicate`, `WithStats`, `WithPublisher`, `WithAllowHigherTier`; `PlanEntry`/`PriceTier`/`StatsProvider` — pick by success/latency/price. |

> Three protocols hand-written, no third-party SDK; unified outbound via `kernel/netx` (redirect-disable prevents credential leakage).

## 2. Context engine (`kernel/orchestrator`)

- **7-layer context** (`Layer`): system / persona / world-book / long-term memory recall / rolling summary / recent messages / current input.
- **Budget allocation** (`Config`: `TotalChars`, `Priority[NumLayers]`, `MinRecentMessages`).
- **Assembly** (`Assembler.Assemble(Layers, MemorySink) → Report`): over budget, compresses from lowest priority; entry-level fit, summary truncation by chars.
- **Forced memory consolidation** (`MemorySink`): truncated recent messages are handed to the memory engine — never silently lost.
- **Minimal assembly** (`BuildMinimal`) and **rolling-summary relay**.

## 3. Storage (`kernel/store`)

Pure-Go, no cgo. `Open` (SQLite WAL); `LatestVersion` versioned migrations; `FieldCipher` (AES-GCM field encryption for
message bodies); `Repo` (sessions/messages), `CharacterRepo`, `StatsRepo`; `MessagePage` paging; `Redact`; `ModelStats`
(success/latency/tokens).

## 4. Secrets (`kernel/secret`)

`Vault` abstraction: `DPAPIVault` (Windows DPAPI), `MemVault` (in-memory/test), `DelegateVault`. `EncryptBackup`/`DecryptBackup`
(user-password-derived). `Scrub`/`ScrubString` (log scrubbing — keys never in logs).

## 5. Character cards (`kernel/cards`)

`Parse` auto-detects four carriers: `ParseJSON` (V1/V2/V3), `ParsePNG` (`tEXt`/`iTXt` embedded base64, `ccv3` > `chara`),
`ParseCharx` (zip `card.json`); `IsPNG`/`IsCharx`. World book `BookEntry` + `ActivatedEntry` (`Constant` always-on + keyword
recursion, depth cap + loop detection).

## 6. Tools / MCP (`kernel/tool` + `kernel/tooldisc`)

- `Client` (Streamable HTTP) / `StdioClient` / `StdioManager`: connect remote or stdio MCP servers.
- `Manager`: **multi-server hot-plug** (`Add`/`Remove`/`SetEnabled`/`RefreshTools`/`CallTool`) without restart.
- Built-in tools: `NewMemoryQueryTool`, `NewMoodReportTool`, `NewVisualRequestTool` (in-process, for the main brain).
- `tooldisc`: **two-stage on-demand discovery** (`Index`/`SearchTool`/`EstimateTokens`/`FootprintTokens`) — thousands of tools without blowing the context.

## 7. Memory (`kernel/hippocampus`)

`Engine`: `Encode` (initial stability from importance/emotion), `Review` (reinforce on hit), `Recall`/`RecallFromStore`
(score by retention). `Retention`: forgetting curve `R = exp(-t/S)`; `Memory` with stability/strength/review time.
`MemorySim`/`RunMonth`: year/month scale simulation.

## 8. Emotion (`kernel/endocrine`)

`Engine`: `Tick` (timed decay), `Interact` (delta on interaction), `State`/`Snapshot`/`Restore`. `Registry`/`Dimension`:
**data-driven dimension table** (5 defaults, extendable/removable, e.g. `DominanceDimension`). `Coupler`: emotion→style
coupling rules.

## 9. Proactivity (`kernel/proactive`)

`Scheduler`: `Tick(reviews, emotion)` evaluates a round; `SetEnabled`/`SetSourceEnabled`; `Snapshot`/`Restore`. Three
sources: `MemoryReviewSource` (memory due), `ScheduleSource` (schedule), `EmotionSource` (emotion threshold). Three gates:
same-round dedup + rate limit (default 3/h, 8/day) + quiet hours (default 23:00–07:00).

## 10. Vision (`kernel/visual`)

`Service` + `GenerateRequest`/`GenerateResult`; `ImageBackend`: `ComfyUIBackend`, `GenericImageBackend`. `ImageDialect`:
multi-dialect (OpenAI/SiliconFlow/Generic/ChatGemini; parses data/images/output/choices/markdown). `Watermarker` +
`HasAIWatermark`: **mandatory AI watermark** (cannot be disabled). `ModelCapability`: probe + degradation. `Gallery`/`SelfiePipeline`.

## 11. Voice / Search (`kernel/voice`, `kernel/search`)

- `voice`: `SttProvider`/`TtsProvider` abstraction + `OpenAIClient` (Whisper/audio, BYOK).
- `search`: `Provider` (`BraveProvider`/`MCPProvider`) + `Router` (failover) + `MemoryCache` + `TokenBucket` (rate limit); pluggable backends.

## 12. Backup / Diag / Compliance / i18n

- `backup`: `WriteArchive`/`ReadArchive`, zip + per-file SHA-256, export→clear→import round-trip consistent.
- `diag`: `Capture`/`Guard`/`GuardThen`; `Dump` **local-only, no network capability**; `PrivacyBoundaryMarkdown`.
- `compliance`: `ContentPolicy`/`Flavor` (different default policy by build flavor).
- `i18n`: `Bundle`/`New`/`NewFrom` — UI strings decoupled from model language.

## 13. Safe transport (`kernel/netx`)

`Client(timeout)` unified outbound + `RedirectPolicy`: **no following redirects** (red line — prevents key leakage across domains).

## 14. C hot paths (`native/`)

`Index`/`Hit` (int8/NEON vector search); `CPUFeatures`/`AvailMemBytes` (device probe). `kernel/native` provides the Go wrapper.

## 15. Dual-end bridges (`mobilebind` / `desktopbind`)

- `mobilebind` (gomobile → Android AAR, **71 exported functions**): `ChatStream`/`ChatStreamTools`/`ChatStreamImage`,
  `CreateCharacter`/`ListCharacters`, `AppendMessage`, `BusPublish`/`BusRecent`, `BackupExport`/`BackupImport`,
  `ComplianceFlavor`, `DeviceProfile`, ...
- `desktopbind` (c-shared DLL + JNI, `Sr*`): `SrInitKernel`/`SrChatOnce`/`SrCreateCharacter`/`SrListCharacters`/`SrListMessages`/`SrVersion`/`SrDeviceProfile`, ...

## 16. Event bus & core contracts (`bus` / `core`)

- `bus`: `Bus`/`Subscription` doorbell broadcast (events carry topic/version only; subscribers pull snapshots; bounded buffer, drop-oldest, publisher never blocks).
- `core`: six types `Message`/`ContentBlock`/`ToolCall`/`Event`/`StreamEvent`/`Role` + `Clock` (real/fake for deterministic tests).

## 17. Commands (`cmd/`)

- `srchat`: minimal CLI chat loop (minimal 7-layer + streaming; key only from file/env).
- `livecheck`: real-endpoint smoke (selftest watermark / chat / image / all).
- `stresssim`: year-scale stress simulation.

---

## Dependency direction (layering)

```
App (your UI)
  └─ mobilebind / desktopbind (AAR / DLL+JNI)
      └─ core: core · bus · adapter · orchestrator · store · secret · tool · tooldisc
               · cards · visual · voice · search · backup · diag · compliance · i18n · netx
          └─ capability plugins: hippocampus(memory) · endocrine(emotion) · proactive(scheduler)
              └─ C hot paths (native/vector, device_probe)
```
Core packages **do not** import capability plugins (guarded by `scripts/check-open-split.sh`); the core compiles standalone.
