package cards

import (
	"archive/zip"
	"bytes"
	"io"
)

// charx 解析：charx 是 zip 包，内含 card.json（可能还有 assets）。用标准库 archive/zip，零新依赖。

var zipMagic = []byte{'P', 'K', 0x03, 0x04}

// zip 炸弹防护上限（astra 评审：未限条目数/累计解压量）。
const (
	maxZipEntries    = 4096     // zip 内条目总数上限
	maxJSONScan      = 512      // 扫描的 *.json 文件数上限
	maxPerJSONBytes  = 32 << 20 // 单文件解压上限
	maxTotalJSONByte = 64 << 20 // 全部 json 累计解压上限
)

// IsCharx 报告是否 zip（charx）签名。
func IsCharx(b []byte) bool {
	return len(b) >= 4 && bytes.Equal(b[:4], zipMagic)
}

// ParseCharx 从 zip 取 card.json 解析。优先根目录 card.json，否则任意 *.json。
func ParseCharx(b []byte) (*Card, error) {
	if !IsCharx(b) {
		return nil, ErrUnknown
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, ErrBadFormat
	}
	if len(zr.File) > maxZipEntries {
		return nil, ErrBadFormat // 条目数超限，疑似 zip 炸弹
	}
	var fallback []byte
	var totalRead int64
	scanned := 0
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := f.Name
		isCardJSON := name == "card.json" || name == "./card.json" ||
			(len(name) >= 5 && name[len(name)-5:] == ".json")
		if !isCardJSON {
			continue
		}
		scanned++
		if scanned > maxJSONScan {
			break // 扫描数超限，停止（防大量小文件拖垮）
		}
		remaining := maxTotalJSONByte - totalRead
		if remaining <= 0 {
			break // 累计解压超限，停止
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		budget := int64(maxPerJSONBytes)
		if int64(remaining) < budget {
			budget = int64(remaining)
		}
		data, err := io.ReadAll(io.LimitReader(rc, budget))
		rc.Close()
		if err != nil {
			continue
		}
		totalRead += int64(len(data))
		if name == "card.json" || f.Name == "./card.json" {
			if c, err := ParseJSON(data); err == nil {
				return c, nil
			}
		}
		if fallback == nil {
			fallback = data
		}
	}
	if fallback != nil {
		return ParseJSON(fallback)
	}
	return nil, ErrNoCard
}

// Parse 自动识别容器（JSON / PNG / charx）并解析。
func Parse(b []byte) (*Card, error) {
	if len(b) == 0 {
		return nil, ErrEmpty
	}
	switch {
	case IsPNG(b):
		return ParsePNG(b)
	case IsCharx(b):
		return ParseCharx(b)
	default:
		return ParseJSON(b)
	}
}
