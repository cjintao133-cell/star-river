# Community Ecosystem · Git-repo-as-backend Interface Contract (v1)

> Goal: **no server** — use a Git repo (GitHub/Gitee/GitLab) to host the "MCP marketplace catalog + forum content".
> The client does only **HTTPS GET of static JSON** (read-only, no user data, zero-upload compliant).
> Key design: **the contract is decoupled from where it's hosted** — `base` points to raw.githubusercontent today, to a
> self-hosted API tomorrow; the client doesn't change a line.

## 0. Can / can't (up front)

| Capability | Git repo? | Notes |
|---|---|---|
| MCP marketplace **catalog** (read) | ✅ | Repo holds `market/index.json`, client pulls it |
| MCP plugin **install** | ✅ | Catalog entry contains `endpoint`; client `mcpAddServer` connects directly (the plugin is a remote MCP server, not in the repo) |
| Forum **browse** (read) | ✅ | Repo holds `forum/posts.json` (merged posts) |
| Forum **post/reply** (write) | ⚠️ partial | Git isn't suited to high-frequency writes. Two options: ① merge posts into the repo = read-only mirror; ② **use GitHub Issues / Discussions as the write entry** (client opens an issue or calls the API; needs a user GitHub account). |
| Moderation / anti-poisoning | ✅ | Via repo PR review (only merged = officially listed) + optional signature file |

**Conclusion**: catalog/browse → Git repo is perfect; posting → use GitHub Issues, or later a real backend. Contract unchanged.

## 1. Hosting (any, client only cares about `base`)
- **GitHub raw**: `https://raw.githubusercontent.com/<owner>/<repo>/main/`
- **jsDelivr CDN** (faster, cached): `https://cdn.jsdelivr.net/gh/<owner>/<repo>@main/`
- **GitHub Pages**: `https://<owner>.github.io/<repo>/`
- Self-hosted API (later): `https://api.your-domain/`
The client fills one into "Settings → Community source"; paths are appended.

## 2. `market/index.json` (MCP marketplace catalog)

```json
{
  "version": 1,
  "updated": "2026-11-20T00:00:00Z",
  "plugins": [
    {
      "id": "brave-search",
      "name": "Brave Search",
      "description": "Brave Search MCP, requires an API key",
      "transport": "sse",
      "endpoint": "https://mcp.brave.com/sse",
      "categories": ["search"],
      "auth": { "type": "header", "header": "Authorization", "hint": "Bearer <your Brave key>" },
      "version": "1.0.0"
    }
  ]
}
```

Fields:
- `transport`: `http` | `sse` (**Android supports only these two**; `stdio` is desktop-only, greyed out on Android).
- `auth.type`: `none` = install directly; `header` = user supplies a value (put into the `header` header), **never in URL, never logged**.
- `id` unique; `name` is the local server label; reinstall dedupes by `id`.

## 3. `forum/posts.json` (read-only forum mirror / after moderation merge)

```json
{
  "version": 1,
  "updated": "2026-11-20T00:00:00Z",
  "posts": [
    { "id": "p1", "title": "How to degrade animated backgrounds on a 2GB phone", "author": "newbie", "created": "2026-11-18T10:00:00Z", "replies": 7, "tags": ["help"], "body": "..." }
  ]
}
```
Write entry (optional): `forum.write` points to repo Discussions/Issues; the client "post" button opens that URL (submitted via the user's GitHub account).

## 4. Client behavior
1. Enter "MCP marketplace" → GET `{base}market/index.json`; on success render catalog cards (install buttons); on failure/timeout → fall back to built-in seeds and show "source unreachable".
2. Enter "Community forum" → GET `{base}forum/posts.json`; fall back to seeds on failure.
3. Install = read entry → (auth=none) `mcpAddServer(name, endpoint, "")`; auth=header → prompt for value → `mcpAddServer`.
4. Everything is read-only GET, no user data leaves; `base` is configurable for switching to self-hosted.

## 5. Security / anti-poisoning
- **Only entries you merge into the main branch are "officially listed"** — PR review is the review.
- Optional `market/index.json.sig` (ed25519, reusing the svc-dist signing idea) — later; v1 relies on repo permissions.
- Client greys out `transport=stdio` on Android; `endpoint` must be `https://` (reject plaintext HTTP).

## 6. When to move to a real backend
When you need **real-time posting/likes/user accounts**, **paid/authenticated private plugins**, or **frequent catalog
updates**. Then point `base` at a real API returning the **same JSON structure** — a smooth switch.
