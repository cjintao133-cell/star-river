// Package diag 是诊断与崩溃边界（T-M7-08）：崩溃默认**仅本地 dump，不自动上传**。
//
// 零数据承诺的代码边界：本包**不 import 任何网络包**（net / net/http 等），
// 因此在能力上不可能发起上传；有 `scripts/check-diag-no-net.sh` 静态守卫。
// dump 只写本地文件（目录 0700 / 文件 0600），由用户**手动**附带进 issue。
//
// 「应用内可达的隐私边界说明」：PrivacyBoundaryMarkdown() 内嵌 docs/privacy-boundary.md 正文，
// 供宿主在设置页展示（UI 接线押后）。
package diag

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"
)

//go:embed privacy-boundary.md
var privacyBoundaryMD string

// PrivacyBoundaryMarkdown 返回隐私边界说明全文（内嵌，应用内可直接展示）。
func PrivacyBoundaryMarkdown() string { return privacyBoundaryMD }

// Dump 一份崩溃 dump（纯本地）。JSON 可序列化。
type Dump struct {
	Time       int64  `json:"time"`  // UnixMilli
	Panic      string `json:"panic"` // recover 到的值
	Stack      string `json:"stack"` // 堆栈
	GoVersion  string `json:"go_version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	AppVersion string `json:"app_version,omitempty"`
	Path       string `json:"path,omitempty"` // 落盘路径（仅 GuardThen 回填，供回调定位）
}

// Capture 由 recover 到的值构造 Dump（不在本函数 recover，调用方在 defer 里 recover 后传入）。
func Capture(recovered any) Dump {
	return Dump{
		Time:      time.Now().UnixMilli(),
		Panic:     fmt.Sprintf("%v", recovered),
		Stack:     string(debug.Stack()),
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// Write 把 dump 写入 dir（目录 0700，文件 0600），返回文件绝对路径。
// 只落本地文件系统，绝不联网。
func (d Dump) Write(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("diag: empty dump dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("diag: mkdir %s: %w", dir, err)
	}
	name := fmt.Sprintf("crash-%d.json", d.Time)
	path := filepath.Join(dir, name)
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	// 0600：仅本用户可读（dump 可能含堆栈/路径线索）。
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", fmt.Errorf("diag: write dump: %w", err)
	}
	return path, nil
}

// Guard 便捷：作为 defer 使用，捕获 panic 仅写本地 dump（不上传、不重新抛出）。
// 用法：defer diag.Guard(dir)；若需上报，由用户在 UI 手动把 dump 文件附进 issue。
func Guard(dir string) {
	if r := recover(); r != nil {
		d := Capture(r)
		_, _ = d.Write(dir)
	}
}

// GuardThen 捕获 panic 后调用 after（如本地记日志），同样不上传。
func GuardThen(dir string, after func(Dump)) {
	if r := recover(); r != nil {
		d := Capture(r)
		if p, err := d.Write(dir); err == nil {
			d.Path = p // 回传落盘路径给 after
		}
		if after != nil {
			after(d)
		}
	}
}
