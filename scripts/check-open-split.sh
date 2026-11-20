#!/bin/sh
# 分层依赖守卫：底座包不得（传递）依赖伴侣插件包（保持底座可单独编译、可插拔）。
#
# 断言：
#   1) 伴侣插件包（hippocampus/endocrine/proactive）不被任何底座包 import（含传递依赖）。
#   2) 底座包单独可编译（go build 通过）。
#
# 用法：sh scripts/check-open-split.sh
set -e

CLOSED="star-river/kernel/hippocampus
star-river/kernel/endocrine
star-river/kernel/proactive"

# 底座包清单（可按发布时口径增删）。
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

echo "== 1) 底座包单独编译 =="
# shellcheck disable=SC2086
go build $OPEN
echo "   OK: 底座包全部可编译"

echo "== 2) 底座包不得（传递）依赖伴侣插件包 =="
fail=0
for pkg in $OPEN; do
	for c in $CLOSED; do
		if go list -deps "$pkg" 2>/dev/null | grep -qx "$c"; then
			echo "   FAIL: $pkg 依赖伴侣插件包 $c"
			fail=1
		fi
	done
done
if [ "$fail" -ne 0 ]; then
	echo "PASS=FAILED"
	exit 1
fi
echo "   OK: 底座不依赖任何伴侣插件包"

echo "PASS: 分层约束成立（底座单独可编译 + 零插件依赖）"
