package cards

import (
	"bytes"
	"encoding/binary"
)

// PNG 解析：从 PNG 的 tEXt / iTXt 元数据 chunk 取 base64 编码的卡 JSON。
// chara（V2 常用）/ ccv3（V3）关键字。纯标准库。

var pngMagic = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

// IsPNG 报告是否 PNG 签名。
func IsPNG(b []byte) bool {
	return len(b) >= 8 && bytes.Equal(b[:8], pngMagic)
}

// ParsePNG 从 PNG 提取角色卡。找不到关键字块返回 ErrNoCard。
func ParsePNG(b []byte) (*Card, error) {
	if !IsPNG(b) {
		return nil, ErrUnknown
	}
	off := 8
	n := uint64(len(b))
	var candidates []string // 优先 ccv3，其次 chara
	for uint64(off)+8 <= n {
		length := uint64(binary.BigEndian.Uint32(b[off : off+4])) // 用 uint64，杜绝 32 位 int 溢出
		typ := string(b[off+4 : off+8])
		dataStart := uint64(off) + 8
		dataEnd := dataStart + length
		if dataEnd+4 > n { // 越界防抖（uint64 加法，无符号回绕不可能到这）
			break
		}
		data := b[dataStart:dataEnd]
		switch typ {
		case "tEXt":
			if kw, text, ok := splitTextChunk(data); ok {
				if kw == "ccv3" {
					candidates = append([]string{text}, candidates...) // ccv3 最优先
				} else if kw == "chara" {
					candidates = append(candidates, text)
				}
			}
		case "iTXt":
			if kw, text, ok := splitITXtChunk(data); ok {
				if kw == "ccv3" {
					candidates = append([]string{text}, candidates...)
				} else if kw == "chara" {
					candidates = append(candidates, text)
				}
			}
		case "IEND":

			goto parse
		}
		off = int(dataEnd) + 4 // 跳过 CRC
	}
parse:
	for _, txt := range candidates {
		if raw, err := decodeB64(txt); err == nil {
			if c, err := ParseJSON(raw); err == nil {
				return c, nil
			}
		}
	}
	return nil, ErrNoCard
}

// splitTextChunk tEXt: keyword\0text（Latin-1）。返回关键字与文本（base64）。
func splitTextChunk(data []byte) (kw, text string, ok bool) {
	i := bytes.IndexByte(data, 0)
	if i < 0 {
		return "", "", false
	}
	return string(data[:i]), string(data[i+1:]), true
}

// splitITXtChunk iTXt: keyword\0 compflag(1) compmethod(1) langtag\0 transkeyword\0 text
// 仅支持未压缩（compflag=0）。若压缩（=1）不支持，返回 ok=false。
func splitITXtChunk(data []byte) (kw, text string, ok bool) {
	i := bytes.IndexByte(data, 0)
	if i < 0 || i+3 > len(data) {
		return "", "", false
	}
	keyword := string(data[:i])
	compFlag := data[i+1]
	rest := data[i+3:] // 跳过 compflag + compmethod
	if compFlag != 0 {
		return "", "", false // 压缩态不支持（v1 极少见）
	}
	// langtag\0
	j := bytes.IndexByte(rest, 0)
	if j < 0 {
		return "", "", false
	}
	rest = rest[j+1:]
	// translatedkeyword\0
	k := bytes.IndexByte(rest, 0)
	if k < 0 {
		return "", "", false
	}
	textBytes := rest[k+1:]
	return keyword, string(textBytes), true
}
