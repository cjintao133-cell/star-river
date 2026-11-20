package mobilebind

// 诊断桥（diag）：崩溃本地 dump（绝不上传）+ 隐私边界文本 + dump 列表/读取（供用户手动附 issue）。
// dump 目录 = 数据库同级的 dumps/（InitKernel 时定）；文件 0600、目录 0700（diag 包保证）。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"star-river/kernel/diag"
)

var dumpsDir string

// setDumpsDir 由 InitKernel 调，指向数据库同级 dumps/。
func setDumpsDir(dbPath string) {
	if dbPath == "" {
		dumpsDir = ""
		return
	}
	dumpsDir = filepath.Join(filepath.Dir(dbPath), "dumps")
}

// DiagPrivacyBoundary 返回隐私边界说明（Markdown，供 App 内展示）。
func DiagPrivacyBoundary() string { return diag.PrivacyBoundaryMarkdown() }

// DiagCrashDump 记录一次崩溃（panicMsg 可为 Java 异常栈文本）。仅写本地，不上传。返回落盘路径或 "ERR:..."。
func DiagCrashDump(panicMsg string) string {
	if dumpsDir == "" {
		return "ERR:dumps dir not configured"
	}
	d := diag.Capture(panicMsg)
	d.Stack = panicMsg // 用调用方传入的真实栈（Go 的 debug.Stack() 对 Java 异常无意义）
	p, err := d.Write(dumpsDir)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return p
}

// DiagListDumps 列出本地崩溃 dump（名称/大小/时间）。返回 JSON 数组。
func DiagListDumps() string {
	type item struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		Time int64  `json:"time"`
	}
	out := []item{}
	if dumpsDir != "" {
		ents, _ := os.ReadDir(dumpsDir)
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			out = append(out, item{Name: e.Name(), Size: info.Size(), Time: info.ModTime().UnixMilli()})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// DiagReadDump 读取某 dump 文件内容（仅限 dumps 目录内的 .json 文件名）。返回内容或 "ERR:..."。
func DiagReadDump(name string) string {
	if dumpsDir == "" {
		return "ERR:dumps dir not configured"
	}
	base := filepath.Base(name) // 防路径穿越
	if !strings.HasSuffix(base, ".json") {
		return "ERR:only .json dumps"
	}
	b, err := os.ReadFile(filepath.Join(dumpsDir, base))
	if err != nil {
		return "ERR:" + err.Error()
	}
	return string(b)
}
