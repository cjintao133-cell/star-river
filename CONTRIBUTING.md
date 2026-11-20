# Contributing to Star River Microkernel

Thanks for your interest. This project is an **embeddable AI-assistant kernel** (Go + C). Please read this and
`docs/architecture.md` before contributing.

## Before you start

- Open an issue to discuss non-trivial changes first.
- By contributing you agree your contributions are licensed under **Apache-2.0** (inbound = outbound), per the DCO
  (sign off your commits with `git commit -s`).

## Per-commit gate (must pass)

```sh
gofmt -l .            # must print nothing
go vet ./...
staticcheck ./...     # go install honnef.co/go/tools/cmd/staticcheck@latest
go test ./kernel/...  # concurrent packages: go test -race
sh scripts/check-open-split.sh   # layering guard (core must not import capability plugins)
```

- Core-package coverage target ≥ 70%.
- C code: must pass ASan + Valgrind with zero reports (`native/`).

## Red lines (non-negotiable)

- **Dependency direction**: core packages must **not** import capability-plugin packages
  (`hippocampus` / `endocrine` / `proactive`). Enforced by `scripts/check-open-split.sh`.
- **Keys never in logs or URLs.** All outbound HTTP goes through `kernel/netx` (no redirect-following).
- Security-sensitive code (crypto, cgo handles, signature chain) is tagged `REVIEW-NEEDED` and needs external review
  before it can be relied upon.

## Style

- Standard Go formatting; small, focused packages; keep the core thin and plugin-friendly.
- Add tests next to the code. Favor deterministic tests (inject `core/clock` fake clocks).
- English for code/comments/docs; keep the `*.zh.md` docs in sync when you change a doc.

## Commit messages

Conventional-ish: `feat(scope): ...`, `fix(scope): ...`, `docs: ...`, `chore: ...`. Sign off (`-s`).
