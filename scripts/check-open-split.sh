#!/bin/sh
# T-M7-05 开源/闭源物理拆分验证（红线 2 守卫）。
#
# 断言：
#   1) 闭源包（hippocampus/endocrine/proactive）不被任何开源包 import（含传递依赖）。
#   2) 开源区单独可编译（go build 通过）。
#
# 用法：sh scripts/check-open-split.sh
set -e

CLOSED="star-river/kernel/hippocampus
star-river/kernel/endocrine
star-river/kernel/proactive"

# 开源区包（可按发布时口径增删）。
OPEN="star-river/kernel/core/types
star-river/kernel/core/clock
star-river/kernel/bus
star-river/kernel/adapter/openai
star-river/kernel/adapter/anthropic
star-river/kernel/adapter/gemini
star-river/kernel/adapter/discovery
star-river/kernel/adapter/router
star-river/kernel/cards
star-river/kernel/tool
star-river/kernel/orchestrator"

echo "== 1) 开源区单独编译 =="
# shellcheck disable=SC2086
go build $OPEN
echo "   OK: 开源包全部可编译"

echo "== 2) 开源包不得（传递）依赖闭源包 =="
fail=0
for pkg in $OPEN; do
	for c in $CLOSED; do
		if go list -deps "$pkg" 2>/dev/null | grep -qx "$c"; then
			echo "   FAIL: $pkg 依赖闭源包 $c"
			fail=1
		fi
	done
done
if [ "$fail" -ne 0 ]; then
	echo "PASS=FAILED"
	exit 1
fi
echo "   OK: 开源区不依赖任何闭源包"

echo "PASS: 开源区物理拆分成立（单独可编译 + 零闭源依赖）"
