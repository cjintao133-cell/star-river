# Platform Capability & Content Compliance

## Content-compliance flavors (dual SKU)

The same codebase produces two SKUs via a build parameter (no fork), selected by
`-ldflags "-X star-river/kernel/compliance.currentFlavor=<flavor>"`; default `international`.

| Dimension | international (default) | domestic |
|---|---|---|
| Sensitive filtering | standard | strict |
| Default-blocked categories | nsfw | nsfw / violence / gambling |
| Default UI language | en | zh |
| Report entry | shown | shown (mandatory) |
| Image generation | on | on |

- The kernel side (`kernel/compliance.ContentPolicy`) exposes the default policy and the `ReportEntryVisible` bit. The
  report-entry UI and real dual-SKU build artifacts belong to the UI/build layer (deferred). `ReportEntryVisible` is
  `true` for both flavors (compliance baseline).

## Platform notes
- **Android**: MCP remote protocol only; built-in tools in-process; no dynamic binary download.
- **Windows**: stdio MCP as a separate process; DPAPI/tray behind interface abstractions, mock-tested on Linux.
- See `architecture.md` for the package boundary table.
