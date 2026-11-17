#!/usr/bin/env bash
# 本地 CI（T-M0-03 阶段：仅 go vet；后续任务逐步补 staticcheck/race/C 门禁）
# 与 .github/workflows/ci.yml 保持同一套命令。
set -uo pipefail
cd "$(dirname "$0")/.."
export PATH=/usr/local/go/bin:/root/go/bin:$PATH
export TMPDIR=/root/build-tmp
fail=0
echo "== go vet =="
if go vet ./...; then echo "[OK] go vet"; else echo "[FAIL] go vet"; fail=1; fi
echo "== RESULT =="
[ "$fail" -eq 0 ] && echo "CI-LOCAL GREEN" || echo "CI-LOCAL RED"
exit "$fail"
