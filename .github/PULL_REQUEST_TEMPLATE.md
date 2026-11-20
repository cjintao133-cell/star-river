## What changed
<!-- Describe the change and the motivation. -->

## Which layer
- [ ] Core
- [ ] Capability plugin
- [ ] Bridge
- [ ] Docs / tooling

## Checklist
- [ ] `gofmt -l .` prints nothing
- [ ] `go vet ./...` passes
- [ ] `staticcheck ./...` passes
- [ ] `go test ./kernel/...` passes (concurrent packages: `-race`)
- [ ] `sh scripts/check-open-split.sh` passes (core doesn't import capability plugins)
- [ ] No secrets in code/logs/URLs
- [ ] Updated docs (and `*.zh.md` if a doc changed)
- [ ] Commits are signed off (`git commit -s`)

## Related issue
Closes #
