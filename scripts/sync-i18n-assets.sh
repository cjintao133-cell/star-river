#!/usr/bin/env bash
# 把 kernel/i18n 的单一真源 JSON 同步到 Android assets（禁 UI 侧另抄一份）。
set -euo pipefail
# UI 已移出主树时跳过（同步目标是 ui-android assets）。
[ -d "$(dirname "$0")/../ui-android" ] || { echo "SKIP: ui-android 不在主树，跳过 i18n assets 同步"; exit 0; }
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/kernel/i18n"
DST="$ROOT/ui-android/app/src/main/assets/i18n"
mkdir -p "$DST"
cp "$SRC/zh.json" "$DST/zh.json"
cp "$SRC/en.json" "$DST/en.json"
echo "synced i18n json -> $DST"
