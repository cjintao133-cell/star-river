#!/usr/bin/env bash
# 本地 CI（T-M0-10 全矩阵：native/C 门禁/vet/staticcheck/race/AAR+APK/DLL/artifacts）
# 与 .github/workflows/ci.yml 保持同一套命令（真实 Actions 触发 DEFERRED 至仓库上线）。
# 顺序纪律：bridge/vector LDFLAGS 依赖 build/native/*.a，必须先 make native + make vector；APK 依赖 AAR。
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

echo "== AAR (gomobile bind android/arm64) =="
if gomobile bind -target=android/arm64 -androidapi 21 -o build/mobilebind.aar ./kernel/mobilebind; then echo "[OK] aar"; else echo "[FAIL] aar"; fail=1; fi

echo "== APK (assembleDebug) =="
if cp build/mobilebind.aar ui-android/app/libs/ && (cd ui-android && ./gradlew assembleDebug --no-daemon); then echo "[OK] apk"; else echo "[FAIL] apk"; fail=1; fi

echo "== Windows DLL (mingw cross) =="
if CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -buildmode=c-shared -o build/desktop/starriver.dll ./kernel/desktopbind; then echo "[OK] dll"; else echo "[FAIL] dll"; fail=1; fi

echo "== artifacts =="
mkdir -p build/artifacts
if cp build/mobilebind.aar ui-android/app/build/outputs/apk/debug/app-debug.apk build/desktop/starriver.dll build/artifacts/ && cp -r build/native build/artifacts/native && (cd build/artifacts && sha256sum mobilebind.aar app-debug.apk starriver.dll native/*/libdevice_probe.a native/*/libvector.a > SHA256SUMS); then echo "[OK] artifacts"; else echo "[FAIL] artifacts"; fail=1; fi

echo "== RESULT =="
[ "$fail" -eq 0 ] && echo "CI-LOCAL GREEN" || echo "CI-LOCAL RED"
exit "$fail"
