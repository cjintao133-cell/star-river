package mobilebind

// diag 桥：崩溃 dump 本地落盘 + 列表/读取 + 隐私边界文本；路径穿越防护。

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagBridge(t *testing.T) {
	InitKernel(filepath.Join(t.TempDir(), "d.db"), base64.StdEncoding.EncodeToString(make([]byte, 32)))

	if pb := DiagPrivacyBoundary(); len(pb) == 0 {
		t.Fatalf("隐私边界文本应非空")
	}
	p := DiagCrashDump("java.lang.RuntimeException: boom\n  at Foo.bar(Foo.java:1)")
	if strings.HasPrefix(p, "ERR:") {
		t.Fatalf("dump 写入失败: %s", p)
	}
	list := DiagListDumps()
	if !strings.Contains(list, "crash-") {
		t.Fatalf("列表应含 crash dump: %s", list)
	}
	// 读取（从路径取基名）
	base := filepath.Base(p)
	content := DiagReadDump(base)
	if !strings.Contains(content, "boom") {
		t.Fatalf("读取应含 panic 内容: %s", content)
	}
	// 路径穿越防护
	if r := DiagReadDump("../../etc/passwd"); !strings.HasPrefix(r, "ERR:") {
		t.Fatalf("应拒绝非 .json: %s", r)
	}
}
