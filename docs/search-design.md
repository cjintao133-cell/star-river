# Web Search Module · Design

> Product constraints: local-first, zero-upload, BYOK, Android + Windows, works in and out of China, cost-conscious.

## 1. Acceptance goal (UI)
User types in a search box → results page shows **title + link + snippet + thumbnail** (search-engine-like); the same
results also feed AI tool calls (the main brain can search).

## 2. Candidate backends (survey facts)
| Backend | Cost | Thumbnail | Notes |
|---|---|---|---|
| **SearXNG (self-hosted)** | Free | Varies by engine | AGPL-3.0, aggregates 70+ engines, JSON returns title/url/snippet; self-hostable |
| Brave Search API | ~$5/1k | Yes (image field) | Stable overseas |
| Exa | ~$7/1k | Limited | Semantic/long-form leaning |
| SerpAPI | from $25/mo | Yes (Google inline images) | High unit price; only when Google images needed |
| Tavily | Free tier | Limited | AI-search leaning |
| Bing Search API | — | — | **Retired 2025-08, not integrated** |
| China: Bocha / Zhipu search | Varies | Yes | More stable in China (Brave/Tavily unstable there) |

## 3. Conclusions

### 1) Adapter abstraction (`kernel/search`)
- **Unified interface**: `Search(ctx, Query{Text, Lang, Region, Count, Recency}) → []Result`; plus `HealthCheck()`, `Quota()`.
- **Unified result**: `Result{ Title, URL, Snippet, Thumbnail?, Favicon?, PublishedAt?, Source }`.
- Adapters **declare capabilities** (has thumbnail / supports Chinese / supports recency) for routing; upper layers are backend-agnostic.

### 2) BYOK backend mix + failover
- **Free first**: user self-hosted SearXNG (preferred) → Tavily free tier → Brave paid.
- **Overseas path**: Brave primary, Tavily backup; Exa only for semantic/long-form; SerpAPI only for Google thumbnails.
- **China path**: add domestic adapters (Bocha/Zhipu), or enable Baidu/Sogou engines in SearXNG.
- **Routing**: probe connectivity at startup → order by "has key, reachable, has quota" → degrade on failure.

### 3) Thumbnail-missing handling (privacy-friendly)
- Degradation chain: backend image → locally fetch the target page `og:image` (first few, short timeout, async, non-blocking) → site favicon → domain first-letter color block.
- **No third-party favicon service** (avoids leaking browsing behavior); provide a "don't load remote images" toggle.

### 4) Cache & rate limiting (the cost-saving core)
- Results cached in **local DB**: key = normalized query + params; recency ~15min, general ~24h; thumbnails on-disk LRU.
- Per-backend **token bucket** + local monthly quota accounting + daily spend cap; 429/timeout → circuit-break + exponential backoff + switch to backup.
- **Biggest saving**: let the model decide "do I even need to search" + dedupe similar queries.

### 5) AGPL risk
- Calling the user's own SearXNG instance over **HTTP does not create a derivative work**, so the app isn't AGPL-tainted; bundling/embedding/distributing SearXNG code would need assessment (recommendation: client connects only, don't distribute SearXNG itself).

### 6) One-line architecture
> `kernel/search` defines a unified `SearchProvider` interface + pluggable adapters (SearXNG/Brave/Bocha/Tavily…), with
> capability declarations and automatic primary/backup routing; local cache + token-bucket rate limiting control cost;
> thumbnails degrade in multiple levels, privacy-friendly; BYOK, default zero-upload.

## 4. Integrating popular search MCPs (hard requirement)

Search should **not** only go through native API adapters; it must also integrate popular **search MCP servers**, participating
in the unified retrieval/routing alongside native adapters.

- **Verified common search MCPs** (all expose "search" as an MCP tool): Tavily, Exa, Brave, Firecrawl, Perplexity Web, DuckDuckGo,
  **Baidu search MCP** (domestic-first), Parallel AI.
- **Reuse the kernel**: a search MCP is just an MCP server exposing a `search` tool — route through `kernel/tool`'s MCP client
  (Streamable HTTP / stdio[Windows]) + `kernel/tooldisc` index, **no per-MCP code needed**.
- **Unified abstraction**: `SearchProvider` has two kinds — ① Native adapter (SearXNG/Brave/Bocha… direct); ② MCP adapter (call
  the search tool on a configured MCP server, map results to unified `Result`). Upper layers/search page/routing don't distinguish them.
- **Config & switching**: settings let you "add a search MCP server" (endpoint/key/tool name) → into the `tooldisc` index →
  selectable in the search page and routing; multi-attach, ordering, enable/disable.
- **Routing**: choose/best-effort across Native and MCP backends by "configured, reachable, has quota", backing each other up.

> Conclusion: search backends = **native API adapters + popular search MCP adapters**, both pluggable, routed together.

## 5. Next steps
1. `kernel/search`: `SearchProvider` interface + `Result` + routing (primary/backup + capability declarations) + local cache + token bucket.
2. Implement the **SearXNG adapter** first (free, self-hostable, easy to test) + **Brave adapter**; reserve domestic (Bocha) interface.
3. Tests: httptest mock backend JSON (title/url/snippet/thumbnail parsing, missing-image degradation, 429 circuit-break, primary/backup switch, cache hit); never hit the real network.

> Voice (STT/TTS) has its own design (`kernel/voice`): adapter abstraction + audio preprocessing (native) + optional third-party; next round.
