#!/bin/sh
# T-M2-07 守卫：全部语言字符串表 key 集合必须与默认语言（zh）一致，保证非默认表「齐全」。
set -e

def=kernel/i18n/zh.json
tmpd=$(mktemp -d)
trap 'rm -rf "$tmpd"' EXIT

grep -oE '"[^"]+"[[:space:]]*:' "$def" | sed 's/[[:space:]]*:$//' | sort -u > "$tmpd/def"

fail=0
for f in kernel/i18n/*.json; do
	if [ "$f" = "$def" ]; then
		continue
	fi
	grep -oE '"[^"]+"[[:space:]]*:' "$f" | sed 's/[[:space:]]*:$//' | sort -u > "$tmpd/cur"
	missed=$(comm -23 "$tmpd/def" "$tmpd/cur")
	extra=$(comm -13 "$tmpd/def" "$tmpd/cur")
	if [ -n "$missed" ] || [ -n "$extra" ]; then
		echo "FAIL: $f 与 $def key 不一致"
		[ -n "$missed" ] && echo "  缺失: $missed"
		[ -n "$extra" ] && echo "  多余: $extra"
		fail=1
	fi
done
if [ "$fail" -ne 0 ]; then
	echo "PASS=FAILED"
	exit 1
fi
echo "PASS: 全部语言字符串表 key 与默认(zh)一致"
