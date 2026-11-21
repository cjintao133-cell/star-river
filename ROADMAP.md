# Roadmap (next 12 months)

This is a candid plan of what the project intends to do — and intentionally **not** do — over the
next year. Dates are targets, not guarantees; priorities may shift with security findings.

## Intend to do

1. **Harden the kernel runtime** — stabilize context assembly, memory and emotion plugins, and the
   tool/MCP layers; keep the core dependency-light and pure-Go where possible.
2. **Supply-chain trust** — reach SLSA Build Level 3 provenance for release binaries, publish
   Sigstore signatures and an SBOM (SPDX/CycloneDX) with every release.
3. **Raise the security bar** — commission independent review of crypto / cgo / signing code;
   keep zero ASan/Valgrind reports on the C hot paths.
4. **Raise test coverage where it lags** — keep statement coverage at or above 80% project-wide;
   add regression tests for every fixed bug.
5. **Grow maintainers** — move bus factor toward 2 by onboarding a second regular maintainer.
6. **Reference integrations** — keep the Android/desktop reference UI out-of-tree but documented,
   so the kernel is evaluated on architecture and protocols.

## Intend NOT to do

- **Not** ship or run model weights. The kernel owns the layer *around* the model; it is BYOK.
- **Not** add a hosted backend or telemetry. It stays local-first and offline-capable.
- **Not** chase every MCP/agent fad; capabilities stay pluggable and optional.
- **Not** claim "audited / production-grade" security before independent review completes.

## Non-goals for this year

- Multi-tenant server / SaaS operation.
- Mobile app store distribution as a primary channel.
