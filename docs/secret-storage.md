# Secret Storage Design

> Implements: keys never in logs/errors/commits; keys only read/written through Keystore/DPAPI (never plaintext in SQLite);
> security-sensitive crypto diffs are tagged REVIEW-NEEDED.

## 1. Contract

- All keys/sensitive fields are read/written only through the `kernel/secret.Vault` interface.
- **The persistence layer (store/SQLite) stores only `secret.SecretRef` (an id string), never the secret itself.** Field
  types use `SecretRef`, so plaintext can't be written into session/config tables at the type level.
- Usage: fetch secret from the Vault → hand to the adapter (memory-only; never persisted, never in URLs/logs) → discard.
- Before any persist/log, any bytes that could contain a secret are scrubbed with `secret.Scrub` / `ScrubString`.

## 2. Unified interface

```go
type Vault interface {
    Set(id string, secret []byte) error
    Get(id string) ([]byte, error)   // ErrNotFound if absent
    Delete(id string) error          // idempotent
}
```

## 3. Platform backends (same interface, pluggable)

| Backend | File | Notes | Status |
|---|---|---|---|
| MemVault | vault.go | In-memory, non-persistent; tests/local dev only | Implemented, tested |
| DelegateVault | vault_delegate.go | Injects a platform Delegate as a Vault; unified validation | Implemented, tested |
| Windows DPAPI | vault_windows.go (`//go:build windows`) | DPAPI (current-user) encryption + ciphertext on disk | Cross-compiles; runtime DEFERRED; REVIEW-NEEDED |
| Android Keystore | (via Delegate) | Keystore key in secure hardware; encrypt/decrypt done by Kotlin `android.security.keystore`, injected into `DelegateVault` through the `kernel/mobilebind` facade | Interface ready; Kotlin impl in UI/bridge layer; runtime DEFERRED |

### Windows DPAPI notes (REVIEW-NEEDED)
- Pure `syscall` to `crypt32.dll` (`CryptProtectData`/`CryptUnprotectData`, freed with `LocalFree`) — **zero new deps**.
- Only visible under `//go:build windows`; Linux build machines verify compile/vet/staticcheck via `GOOS=windows`, **no runtime verification → DEFERRED**.
- Crypto-implementation diff → **not final until external review; do not self-merge.**
- Ciphertext dir recommended `%APPDATA%/star-river/secrets`, files `0600`, ids whitelisted by `sanitizeID` to prevent path traversal.

### Android Keystore notes
- The Go kernel never touches JNI/Keystore directly: Keystore keys must be generated/used in the Android process. Kotlin uses
  `KeyStore.getInstance("AndroidKeyStore")` + AES/GCM, stores ciphertext in app-private storage, and injects an object
  implementing `secret.Delegate` into Go's `DelegateVault` via the gomobile facade. **Go depends only on the `Delegate`
  interface; platform code stays in the platform layer.**

## 4. "No plaintext in SQLite" double assertion (persistence_test.go)

1. **Unit test**: build the `persistedConfig` to be persisted (contains only `APIKeyRef`), serialize to a temp "db file", assert the secret lives only in the Vault and is retrievable via `ref.id`.
2. **File string scan**: scan the raw bytes of that "db file" and assert it contains **no secret plaintext**; include a **reverse control** (deliberately write the secret into the struct, the scan must catch it) to prove the scan isn't vacuous; also assert the log file has no plaintext after Scrubbing.

> SQLite (store) arrives later; this uses a temp-file simulation of the "store-references-only" contract and assertions.
> When the real store lands, swap in the real schema/db path and reuse the same assertions.

## 5. Deferred

- Windows DPAPI runtime round-trip verification → deferred (consistent with other Windows runtime items).
- Android Keystore Kotlin implementation and gomobile injection wiring → M5.
- Real SQLite file string-scan assertion → after the store lands (contract and test skeleton ready).
- DPAPI crypto implementation: REVIEW-NEEDED.
