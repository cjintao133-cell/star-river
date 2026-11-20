# Security Policy

## Reporting a vulnerability

Please **do not** open a public issue for security vulnerabilities.

Report privately through either channel:

- **GitHub private security advisory (preferred):** https://github.com/cjintao133-cell/star-river/security/advisories/new
- **Email:** security@star-river.dev

We aim to acknowledge within 7 days and to follow coordinated disclosure. Please include: affected
version/commit, a description, and a minimal reproduction if possible.

Preferred-Languages: en, zh

## Scope and current status (please read)

This project is **0.1.x (alpha)** and **not yet externally security-audited**. The following parts are
hardened but await independent review (see `KNOWN-ISSUES.md` §1):

- `kernel/store/crypto.go` — AES-GCM field encryption
- `kernel/secret/{vault_windows.go,keybackup.go}` — DPAPI custody & encrypted backup
- `kernel/native/vector` and `kernel/native/bridge` — cgo boundaries
- `kernel/desktopbind/jnibridge*.c` — hand-written JNI
- `svc-dist/` — ed25519 manifest signing / update chain

**Do not claim "production-grade / military-grade encryption" for this project until external review completes.**

## Supported versions

| Version | Supported |
|---|---|
| 0.1.x (alpha) | ✅ (best effort) |

## Disclosure

We follow coordinated disclosure. Once a fix is available, we will credit reporters (unless you prefer to stay anonymous).
