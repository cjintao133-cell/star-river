#!/usr/bin/env bash
# 桌面 JNI 全链路无头冒烟（T-M5-03）。构建 c-shared .so → 编译 KernelBridge+smoke → 运行断言。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/build/desktop"; mkdir -p "$OUT"
echo "[1/3] build libstarriver.so"
go build -C "$ROOT" -buildmode=c-shared -o "$OUT/libstarriver.so" ./kernel/desktopbind
echo "[2/3] javac KernelBridge + smoke"
JAVAC=/usr/lib/jvm/java-17-openjdk-amd64/bin/javac
JAVA=/usr/lib/jvm/java-17-openjdk-amd64/bin/java
CLS="$OUT/smoke-classes"; rm -rf "$CLS"; mkdir -p "$CLS"
"$JAVAC" -d "$CLS" "$ROOT/ui-desktop/src/main/java/com/starriver/desktop/KernelBridge.java" "$ROOT/scripts/desktop-jni-smoke/DesktopKernelSmoke.java"
echo "[3/3] run"
KEY=$(python3 -c "import os,base64;print(base64.b64encode(os.urandom(32)).decode())")
TMPD=$(mktemp -d)
"$JAVA" -cp "$CLS" DesktopKernelSmoke "$OUT/libstarriver.so" "$TMPD" "$KEY"
echo "DESKTOP-JNI-GATE: PASS"
