# Privacy Boundary (no-upload commitment)

Star River's promise: **no upload by default, keys stay in your hands.** This document lists *what is stored locally*,
*when data leaves the device*, and *how crash dumps are handled* — a behavioral boundary spec.

## 1. What is stored locally (all on your own device)

| Data | Location | Encrypted |
|---|---|---|
| Sessions & messages | Local SQLite (`sessions` / `messages`) | Message body `content_cipher` uses AES-GCM field encryption (key in local Keystore/DPAPI) |
| Long-term memory, vector index | Local (engine memory / later persisted) | Per storage-base policy |
| Emotion state, proactivity state | Local snapshots | — |
| Local model stats (success/latency/token) | Local SQLite (`model_stats`) | — |
| API keys (BYOK) | Local secret vault (Android Keystore / Windows DPAPI) | Protected by the OS keystore; backup encrypted with a user-password-derived key |
| App config | Local (`app_meta`) | — |

The above is **not uploaded to any server of ours** (the project has no own backend collecting user data).

## 2. When data leaves the device

Data leaves only when **you initiate it** and the **destination is an endpoint you configured**:

1. **Your model endpoint** (BYOK): on chat, the context is sent to **the API address you entered** (official or a gateway you trust). The key travels only in the request header, never in URLs/logs.
2. **Your MCP / image service**: tool calls and image generation send requests to **the server you configured**.
3. **Update/theme/character-card download (CDN)**: only when you trigger a download, files are fetched from the configured CDN; no user content is reported.
4. **Crash dump (off by default)**: see section 3; stored locally by default.

Beyond this, the gateway/adapter layer does not send your data to any third party on its own. Criterion: **the endpoint
address is entered by you; if you don't enter it or don't trigger it, data doesn't leave the device.**

## 3. Crash dumps (local only by default, never auto-uploaded)

- On crash, a dump file is written **only** locally (dir `0700`, file `0600`) with crash info + stack + Go version/OS/arch;
  **no network reporting** (the diag package has no network capability, so it cannot upload).
- Whether to send the dump to a developer is **entirely your manual choice**: find the local dump under Settings → About/Diagnostics and decide whether to paste it into an issue.
- Offline crashes: the dump still lands locally (no network dependency).

## 4. What you can take with you

- **Data export**: package sessions/messages/stats/config into an archive for backup or migration.
- **Key backup**: encrypted with a password you set; restore on a new device with the same password. The password is never
  stored; if forgotten, it cannot be recovered.

## 5. Outside the boundary (explicitly not done)

- No telemetry/analytics to the cloud.
- No upload of any content without your knowledge.
- No keys written into logs, URLs, or crash dumps.

> This matches the copy embedded in `kernel/diag`; viewable in-app at Settings → About → Privacy Boundary.
