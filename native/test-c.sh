#!/usr/bin/env bash
# C 门禁（T-M0-08）：device_probe 单测三跑，串行（内存纪律：ASan/Valgrind 禁并行）。
# 矩阵：普通+wrap注入 → ASan+wrap注入 → Valgrind 正常版（无注入）。
# Valgrind 不跑 wrap 版：注入态干扰其 syscall 包装层，正常版覆盖内存安全即可。
set -uo pipefail
cd "$(dirname "$0")/.."
export TMPDIR=/root/build-tmp
SRC="native/device_probe/device_probe.c native/device_probe/test/test_device_probe.c"
OUT=build/native/test
mkdir -p "$OUT"
CC=gcc
FLAGS="-O2 -Wall -Wextra -Werror -std=c11"
fail=0

echo "== C tests: normal + fault-injection (--wrap=sysinfo) =="
if $CC $FLAGS $SRC -Wl,--wrap=sysinfo -o "$OUT/t-normal" && "$OUT/t-normal"; then
  echo "[OK] c-normal"
else
  echo "[FAIL] c-normal"; fail=1
fi

echo "== C tests: ASan + fault-injection =="
if $CC -O1 -g -fsanitize=address -fno-omit-frame-pointer -Wall -Wextra -Werror -std=c11 \
    $SRC -Wl,--wrap=sysinfo -o "$OUT/t-asan" && "$OUT/t-asan"; then
  echo "[OK] c-asan"
else
  echo "[FAIL] c-asan"; fail=1
fi

echo "== C tests: Valgrind (normal, no injection) =="
if $CC $FLAGS -DTEST_NO_WRAP $SRC -o "$OUT/t-vg" && \
   valgrind --leak-check=full --error-exitcode=99 "$OUT/t-vg"; then
  echo "[OK] c-valgrind"
else
  echo "[FAIL] c-valgrind"; fail=1
fi

echo "== C RESULT =="
if [ "$fail" -eq 0 ]; then echo "C-GATE GREEN"; else echo "C-GATE RED"; fi
exit "$fail"
