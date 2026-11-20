package diag

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureAndWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dumps")
	d := Capture("boom: something broke")
	if d.Panic != "boom: something broke" || d.Stack == "" || d.GoVersion == "" || d.OS == "" || d.Arch == "" {
		t.Fatalf("Capture 字段不全: %+v", d)
	}
	path, err := d.Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	// 文件存在且 0600。
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("dump 文件未生成: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("dump 文件权限应 0600, got %o", info.Mode().Perm())
	}
	// 目录 0700。
	di, _ := os.Stat(dir)
	if di.Mode().Perm() != 0o700 {
		t.Errorf("dump 目录权限应 0700, got %o", di.Mode().Perm())
	}
	// 内容可解析且字段正确。
	b, _ := os.ReadFile(path)
	var back Dump
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("dump 非合法 JSON: %v", err)
	}
	if back.Panic != "boom: something broke" {
		t.Errorf("panic 字段不符: %q", back.Panic)
	}
}

func TestWriteEmptyDirError(t *testing.T) {
	d := Capture("x")
	if _, err := d.Write(""); err == nil {
		t.Error("空 dir 应报错")
	}
}

func TestGuardRecoversAndWrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	done := func() (ok bool) {
		defer func() {
			// Guard 内部 recover，不应再抛出 panic。
			ok = true
		}()
		func() {
			defer Guard(dir)
			panic("guard-test-panic")
		}()
		return ok
	}()
	if !done {
		t.Fatal("Guard 应吞掉 panic 并正常返回")
	}
	// 应生成一份 dump。
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("应生成 1 份 dump, got %v %v", entries, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if !strings.Contains(string(b), "guard-test-panic") {
		t.Errorf("dump 应含 panic 信息: %s", b)
	}
}

func TestGuardThen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	var captured Dump
	func() {
		defer GuardThen(dir, func(d Dump) { captured = d })
		panic("then-panic")
	}()
	if captured.Panic != "then-panic" {
		t.Errorf("after 回调应收到 dump: %+v", captured)
	}
}

func TestGuardNoPanicNoDump(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	func() {
		defer Guard(dir)
		// 无 panic
	}()
	if _, err := os.ReadDir(dir); !os.IsNotExist(err) {
		t.Error("无 panic 不应创建 dump 目录")
	}
}

func TestPrivacyBoundaryMarkdown(t *testing.T) {
	md := PrivacyBoundaryMarkdown()
	if md == "" {
		t.Fatal("隐私边界说明不应为空")
	}
	for _, kw := range []string{"零数据上传", "崩溃 dump", "不自动上传", "本地"} {
		if !strings.Contains(md, kw) {
			t.Errorf("隐私说明应含关键词 %q", kw)
		}
	}
}

// TestDumpDoesNotTouchNetwork 断网语义的结构性证明：本包不 import 网络包，
// 因此 Write 在无网络环境下必然成功（这里仅验证写入成功；网络能力由 check-diag-no-net.sh 静态守卫）。
func TestDumpDoesNotTouchNetwork(t *testing.T) {
	dir := t.TempDir()
	d := Capture("offline")
	if _, err := d.Write(dir); err != nil {
		t.Fatalf("无网络依赖下 dump 写入应成功: %v", err)
	}
}
