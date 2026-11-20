package visual

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // 注册 JPEG 解码
	"image/png"
	"strings"
)

// watermarkKeyword / watermarkValue 机器可检测水印标记（PNG tEXt）。
const (
	watermarkKeyword = "Software"
	watermarkValue   = "Star River AIGC (AI-generated)"
	// watermarkMarker 子串，用于检测。
	watermarkMarker = "AIGC"
)

// ErrNotImage 输入不是可解码图像。
var ErrNotImage = errors.New("visual: input is not a decodable image")

// Watermarker 强制 AI 标识水印器。零值可用（默认透明度）。
type Watermarker struct {
	// Opacity 可见方块透明度 [0,255]，0 用默认 96。
	Opacity uint8
	// Stride 方块间隔（像素），0 用默认 48；Tile 为方块边长，0 用 16。
	Stride int
	Tile   int
}

// NewWatermarker 建水印器（默认参数）。
func NewWatermarker() Watermarker { return Watermarker{Opacity: 96, Stride: 48, Tile: 16} }

// Mark 给图像打上不可关水印：可见像素标记 + PNG tEXt 机器标记。返回 (新图字节, mime, err)。
// 输出统一为 PNG（无损，保留水印）。
func (w Watermarker) Mark(data []byte) ([]byte, string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", ErrNotImage
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, "", ErrNotImage
	}
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)

	alpha := w.Opacity
	if alpha == 0 {
		alpha = 96
	}
	stride := w.Stride
	if stride <= 0 {
		stride = 48
	}
	tile := w.Tile
	if tile <= 0 {
		tile = 16
	}
	// 稀疏半透明方块图样（可见溯源标记）。
	square := &image.Uniform{color.RGBA{R: 255, G: 255, B: 255, A: alpha}}
	for y := 0; y < rgba.Bounds().Dy(); y += stride {
		for x := 0; x < rgba.Bounds().Dx(); x += stride {
			r := image.Rect(x, y, x+tile, y+tile).Intersect(rgba.Bounds())
			if r.Empty() {
				continue
			}
			draw.Draw(rgba, r, square, image.Point{}, draw.Over)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		return nil, "", err
	}
	out := injectPNGText(buf.Bytes(), watermarkKeyword, watermarkValue)
	return out, "image/png", nil
}

// HasAIWatermark 检测 PNG 字节是否带本框架 AI 标识（tEXt Software 含 "AIGC"）。
func HasAIWatermark(data []byte) bool {
	if len(data) < 8 || !bytes.Equal(data[:8], pngSignature) {
		return false
	}
	pos := 8
	for pos+8 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		typ := string(data[pos+4 : pos+8])
		dataStart := pos + 8
		dataEnd := dataStart + length
		if dataEnd+4 > len(data) {
			return false
		}
		if typ == "tEXt" {
			chunk := data[dataStart:dataEnd]
			if i := bytes.IndexByte(chunk, 0); i >= 0 {
				kw := string(chunk[:i])
				val := string(chunk[i+1:])
				if kw == watermarkKeyword && strings.Contains(val, watermarkMarker) {
					return true
				}
			}
		}
		if typ == "IEND" {
			break
		}
		pos = dataEnd + 4
	}
	return false
}

// pngSignature PNG 文件签名。
var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

// injectPNGText 在 IEND 前插入一个 tEXt 块（keyword\0text）。
// 若解析失败则原样返回（不破坏图）。
func injectPNGText(pngBytes []byte, keyword, text string) []byte {
	if len(pngBytes) < 8 || !bytes.Equal(pngBytes[:8], pngSignature) {
		return pngBytes
	}
	// 找 IEND 起始偏移。
	insertAt := -1
	pos := 8
	for pos+8 <= len(pngBytes) {
		length := int(binary.BigEndian.Uint32(pngBytes[pos : pos+4]))
		typ := string(pngBytes[pos+4 : pos+8])
		if typ == "IEND" {
			insertAt = pos
			break
		}
		pos += 8 + length + 4
	}
	if insertAt < 0 || insertAt > len(pngBytes) {
		return pngBytes
	}
	// 组装 tEXt chunk。
	chunkData := append([]byte(keyword), 0x00)
	chunkData = append(chunkData, []byte(text)...)
	var chunk bytes.Buffer
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(chunkData)))
	chunk.Write(lenBuf[:])
	chunk.WriteString("tEXt")
	chunk.Write(chunkData)
	crc := crc32.NewIEEE()
	crc.Write([]byte("tEXt"))
	crc.Write(chunkData)
	var crcBuf [4]byte
	binary.BigEndian.PutUint32(crcBuf[:], crc.Sum32())
	chunk.Write(crcBuf[:])

	out := make([]byte, 0, len(pngBytes)+chunk.Len())
	out = append(out, pngBytes[:insertAt]...)
	out = append(out, chunk.Bytes()...)
	out = append(out, pngBytes[insertAt:]...)
	return out
}
