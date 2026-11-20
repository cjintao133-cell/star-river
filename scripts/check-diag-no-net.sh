#!/bin/sh
# T-M7-08 守卫：诊断包不得具备网络能力（崩溃 dump 只落本地、绝不自动上传）。
# 断言 kernel/diag 的（传递）依赖里不含任何网络包。
set -e

BAD="net
net/http
net/url
net/rpc
net/smtp
net/mail"

deps=$(go list -deps ./kernel/diag/ 2>/dev/null)
fail=0
for bad in $BAD; do
	# 精确匹配包名（去空白）
	if echo "$deps" | grep -qx "$bad"; then
		echo "FAIL: kernel/diag 依赖了网络包 $bad（违反零上传边界）"
		fail=1
	fi
done
if [ "$fail" -ne 0 ]; then
	echo "PASS=FAILED"
	exit 1
fi
echo "PASS: kernel/diag 无任何网络包依赖（崩溃 dump 仅本地）"
