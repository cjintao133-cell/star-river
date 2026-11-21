# Security Requirements (what users can and cannot expect)

This documents the security expectations for Star River (required for the OpenSSF Best Practices
Silver badge: "what the user can and cannot expect in terms of security").

## What the project aims to provide

- **Data at rest:** field-level encryption with AES-256-GCM; the local SQLite store (WAL) keeps
  sensitive fields encrypted. Keys are held via the OS keystore (Android Keystore / Windows DPAPI),
  never written in plaintext config or logs.
- **Data in transit:** outbound network calls use Go's `net/http` with TLS 1.2+ and certificate
  verification on by default; all outbound HTTP is routed through `kernel/netx` with no
  redirect-following. The kernel ships a telemetry-free design: **no data is sent to the authors.**
- **Key handling:** credentials and private keys are stored separately from other data and can be
  rotated without recompiling.
- **No custom crypto:** there is no bespoke network transport cryptography; only vetted primitives
  (AES-GCM, standard TLS).

## What the project does NOT yet provide

- **No independent security audit.** Code in `kernel/store/crypto.go`, `kernel/secret/`, the cgo
  boundaries (`kernel/native/*`), the JNI bridge, and `svc-dist/` signing are **hardened but await
  external review** (see SECURITY.md, KNOWN-ISSUES.md §1). Do not treat them as audited.
- **No formal threat model for hostile multi-tenant hosting.** The project assumes a single local
  user per device.
- **No guarantee against a fully compromised host OS.** OS-level keystore/DPAPI protections are
  the trust anchor.

## Out of scope

- Protecting against an attacker with root/admin on the device.
- Side-channel / physical attacks on the hardware.

Security expectations may only be strengthened over time; any reduction is treated as a defect.
