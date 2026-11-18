#!/usr/bin/env bash
# 本地 CI（T-M0-08：go vet + native 构建 + C 门禁三跑；staticcheck/race/AAR/DLL 由 T-M0-10 接入）
# 与 .github/workflows/ci.yml 保持同一套命令（真实 Actions 触发 DEFERRED 至仓库上线）。
# 顺序纪律：bridge 包 LDFLAGS 依赖 build/native/*.a，必须先 make native。
set -uo pipefail
cd "$(dirname "$0")/.."
export PATH=/usr/local/go/bin:/root/go/bin:$PATH
export TMPDIR=/root/build-tmp
fail=0

echo "== native static libs =="
if make -j4 -f native/Makefile all; then echo "[OK] native"; else echo "[FAIL] native"; fail=1; fi

echo "== C gate (normal/ASan/Valgrind serial) =="
if bash native/test-c.sh; then echo "[OK] c-gate"; else echo "[FAIL] c-gate"; fail=1; fi

echo "== go vet =="
if go vet ./...; then echo "[OK] go vet"; else echo "[FAIL] go vet"; fail=1; fi

echo "== RESULT =="
[ "$fail" -eq 0 ] && echo "CI-LOCAL GREEN" || echo "CI-LOCAL RED"
exit "$fail"
