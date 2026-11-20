#!/bin/sh
# sbom.sh —— 生成软件物料清单（SBOM）。
# 优先产出 CycloneDX（若装了 cyclonedx-gomod），并总是输出一份 Go 模块清单。
# 用法： sh scripts/sbom.sh [输出目录]
set -eu
cd "$(dirname "$0")/.." || exit 2
OUT="${1:-build/sbom}"
mkdir -p "$OUT"

echo "== Go module 依赖清单 =="
go list -m all > "$OUT/go-modules.txt"
echo "   -> $OUT/go-modules.txt"

if command -v cyclonedx-gomod >/dev/null 2>&1; then
  echo "== CycloneDX SBOM =="
  cyclonedx-gomod mod -json -output "$OUT/sbom.cdx.json"
  echo "   -> $OUT/sbom.cdx.json"
else
  echo "提示：未安装 cyclonedx-gomod，跳过 CycloneDX。"
  echo "    安装： go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest"
fi
