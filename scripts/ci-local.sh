#!/usr/bin/env bash
# 本地 CI（内核/框架侧）。UI 已移出主树后，本脚本只验证 Go 内核 + native + C 门禁 + 拆分。
# 顺序纪律：bridge/vector LDFLAGS 依赖 build/native/*.a，必须先 make native + make vector。
# 如需构建 Android/桌面 UI，请切到 with-ui 分支（tag pre-ui-strip）。
set -uo pipefail
cd "$(dirname "$0")/.."
export PATH=/usr/local/go/bin:/root/go/bin:$PATH
export TMPDIR=/root/build-tmp
fail=0

echo "== native static libs =="
if make -j4 -f native/Makefile all; then echo "[OK] native"; else echo "[FAIL] native"; fail=1; fi

echo "== native vector libs (T-M3-01) =="
if make -j4 -f native/Makefile vector; then echo "[OK] vector"; else echo "[FAIL] vector"; fail=1; fi

echo "== C gate (normal/ASan/Valgrind serial) =="
if bash native/test-c.sh; then echo "[OK] c-gate"; else echo "[FAIL] c-gate"; fail=1; fi

echo "== vector C test+ASan+Valgrind =="
if gcc -O2 -Wall -Wextra -Werror -std=c11 native/vector/vector.c native/vector/test/test_vector.c -o build/native/vector-test -lm && build/native/vector-test \
   && gcc -O1 -g -fsanitize=address -fno-omit-frame-pointer -std=c11 native/vector/vector.c native/vector/test/test_vector.c -o build/native/vector-test-asan -lm && build/native/vector-test-asan >/dev/null \
   && valgrind --leak-check=full --error-exitcode=99 --quiet build/native/vector-test >/dev/null; then
  echo "[OK] vector-c-gate"; else echo "[FAIL] vector-c-gate"; fail=1; fi

echo "== go vet =="
if go vet ./...; then echo "[OK] go vet"; else echo "[FAIL] go vet"; fail=1; fi

echo "== staticcheck =="
if staticcheck ./...; then echo "[OK] staticcheck"; else echo "[FAIL] staticcheck"; fail=1; fi

echo "== go test -race =="
if go test -p 2 -race -count=1 ./kernel/...; then echo "[OK] go test -race"; else echo "[FAIL] go test -race"; fail=1; fi

echo "== open/closed split (redline 2) =="
if bash scripts/check-open-split.sh; then echo "[OK] open-split"; else echo "[FAIL] open-split"; fail=1; fi

echo "== RESULT =="
[ "$fail" -eq 0 ] && echo "CI-LOCAL GREEN" || echo "CI-LOCAL RED"
exit "$fail"
