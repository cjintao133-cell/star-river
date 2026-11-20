package cards

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"testing"
)

const v2json = `{
  "spec": "chara_card_v2",
  "spec_version": "2.0",
  "data": {
    "name": "示例角色",
    "description": "示例角色",
    "personality": "温和",
    "scenario": "海边",
    "first_mes": "哼，你来了",
    "mes_example": "<START>",
    "system_prompt": "你是示例角色",
    "post_history_instructions": "保持设定",
    "alternate_greetings": ["第一开场", "第二开场", "第三开场"],
    "character_book": {"entries": [
      {"keys": ["海", "示例"], "content": "示例设定内容", "enabled": true, "constant": false, "insertion_order": 1},
      {"keys": [], "content": "常驻世界观", "enabled": true, "constant": true},
      {"keys": ["禁用"], "content": "不该出现", "enabled": false}
    ]}
  }
}`

func TestParseV2(t *testing.T) {
	c, err := ParseJSON([]byte(v2json))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if c.Spec != "chara_card_v2" || c.Name != "示例角色" || c.FirstMessage != "哼，你来了" {
		t.Errorf("card = %+v", c)
	}
	if c.SystemPrompt != "你是示例角色" || c.PostHistoryInstructions != "保持设定" {
		t.Errorf("sys/config 缺失")
	}
	if len(c.AlternateGreetings) != 3 { // A4 全存
		t.Errorf("alternate_greetings 应全存 3 条, got %d", len(c.AlternateGreetings))
	}
	if len(c.Book) != 3 {
		t.Fatalf("book = %d", len(c.Book))
	}
	if c.Book[2].Enabled {
		t.Error("enabled:false 应保持 false")
	}
}

const v1json = `{
  "name": "旧卡",
  "description": "v1 扁平",
  "first_mes": "你好",
  "personality": "温和"
}`

func TestParseV1Flat(t *testing.T) {
	c, err := ParseJSON([]byte(v1json))
	if err != nil {
		t.Fatalf("ParseV1: %v", err)
	}
	if c.Name != "旧卡" || c.FirstMessage != "你好" || c.Personality != "温和" {
		t.Errorf("v1 = %+v", c)
	}
	if c.Spec != "" {
		t.Errorf("v1 spec 应为空, got %q", c.Spec)
	}
}

func TestParseBadJSON(t *testing.T) {
	if _, err := ParseJSON([]byte("not json")); err == nil {
		t.Error("坏 JSON 应报错")
	}
	if _, err := ParseJSON(nil); err == nil {
		t.Error("空应报错")
	}
}

// 构造一个带 tEXt "chara" 的最小 PNG。
func makePNGChunk(typ string, data []byte) []byte {
	var buf bytes.Buffer
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(data)))
	buf.Write(l[:])
	buf.WriteString(typ)
	buf.Write(data)
	crc := crc32.NewIEEE()
	crc.Write([]byte(typ))
	crc.Write(data)
	var c [4]byte
	binary.BigEndian.PutUint32(c[:], crc.Sum32())
	buf.Write(c[:])
	return buf.Bytes()
}

func makePNG(t *testing.T, kw string, cardJSON []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write(pngMagic)
	buf.Write(makePNGChunk("IHDR", []byte{0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0}))
	text := kw + "\x00" + base64.StdEncoding.EncodeToString(cardJSON)
	buf.Write(makePNGChunk("tEXt", []byte(text)))
	buf.Write(makePNGChunk("IEND", nil))
	return buf.Bytes()
}

func TestParsePNG(t *testing.T) {
	png := makePNG(t, "chara", []byte(v2json))
	c, err := ParsePNG(png)
	if err != nil {
		t.Fatalf("ParsePNG: %v", err)
	}
	if c.Name != "示例角色" {
		t.Errorf("name = %q", c.Name)
	}
	// 自动识别也应走 PNG。
	c2, err := Parse(png)
	if err != nil || c2.Name != "示例角色" {
		t.Errorf("Parse(PNG) = %+v, %v", c2, err)
	}
}

func TestParsePNGNoCard(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(pngMagic)
	buf.Write(makePNGChunk("IEND", nil))
	if _, err := ParsePNG(buf.Bytes()); err != ErrNoCard {
		t.Errorf("无 chara 块应 ErrNoCard, got %v", err)
	}
}

// 写最小 zip（charx）含 card.json。
func TestParseCharx(t *testing.T) {
	zw := newTestZip(t)
	zw.add("card.json", []byte(v2json))
	zw.add("assets/a.png", []byte("fakepng"))
	blob := zw.close()
	c, err := ParseCharx(blob)
	if err != nil {
		t.Fatalf("ParseCharx: %v", err)
	}
	if c.Name != "示例角色" {
		t.Errorf("name = %q", c.Name)
	}
	if c2, err := Parse(blob); err != nil || c2.Name != "示例角色" {
		t.Errorf("Parse(charx) = %+v, %v", c2, err)
	}
}

func TestParseUnknown(t *testing.T) {
	if _, err := Parse([]byte("\x00\x01\x02garbage")); err == nil {
		t.Error("无法识别容器应报错")
	}
}

// —— 测试用最小 zip 构造 ——
type testZip struct {
	buf *bytes.Buffer
	w   *zip.Writer
}

func newTestZip(t *testing.T) *testZip {
	t.Helper()
	b := &bytes.Buffer{}
	return &testZip{buf: b, w: zip.NewWriter(b)}
}

func (z *testZip) add(name string, data []byte) {
	f, err := z.w.Create(name)
	if err != nil {
		panic(err)
	}
	f.Write(data)
}

func (z *testZip) close() []byte {
	z.w.Close()
	return z.buf.Bytes()
}

// iTXt（未压缩）载体。
func makeITXtPNG(t *testing.T, kw string, cardJSON []byte) []byte {
	t.Helper()
	text := base64.StdEncoding.EncodeToString(cardJSON)
	// keyword\0 compflag(0) compmethod(0) langtag\0 transkeyword\0 text
	var d []byte
	d = append(d, []byte(kw)...)
	d = append(d, 0, 0, 0)
	d = append(d, []byte("en")...)
	d = append(d, 0)
	d = append(d, []byte("desc")...)
	d = append(d, 0)
	d = append(d, []byte(text)...)

	var buf bytes.Buffer
	buf.Write(pngMagic)
	buf.Write(makePNGChunk("IHDR", []byte{0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0}))
	buf.Write(makePNGChunk("iTXt", d))
	buf.Write(makePNGChunk("IEND", nil))
	return buf.Bytes()
}

func TestParsePNGiTXt(t *testing.T) {
	png := makeITXtPNG(t, "chara", []byte(v2json))
	c, err := ParsePNG(png)
	if err != nil {
		t.Fatalf("iTXt ParsePNG: %v", err)
	}
	if c.Name != "示例角色" {
		t.Errorf("name = %q", c.Name)
	}
}

func TestDecodeB64Variants(t *testing.T) {
	raw := []byte("hello world")
	std := base64.StdEncoding.EncodeToString(raw)
	if d, err := decodeB64(std); err != nil || string(d) != "hello world" {
		t.Errorf("std decode = %q, %v", d, err)
	}
	// 带空白容错。
	if d, err := decodeB64(" " + std + "\n"); err != nil || string(d) != "hello world" {
		t.Errorf("whitespace decode = %q, %v", d, err)
	}
	// 非法。
	if _, err := decodeB64("!!!not base64!!!"); err == nil {
		t.Error("非法 base64 应报错")
	}
}

// charx 里只有非 card.json 的 .json → fallback 解析。
func TestParseCharxFallbackJSON(t *testing.T) {
	zw := newTestZip(t)
	zw.add("data/character.json", []byte(v2json))
	c, err := ParseCharx(zw.close())
	if err != nil || c.Name != "示例角色" {
		t.Errorf("fallback json = %+v, %v", c, err)
	}
}

// charx 里没有 json → ErrNoCard。
func TestParseCharxNoJSON(t *testing.T) {
	zw := newTestZip(t)
	zw.add("readme.txt", []byte("hi"))
	if _, err := ParseCharx(zw.close()); err != ErrNoCard {
		t.Errorf("无 json 应 ErrNoCard, got %v", err)
	}
}

// splitTextChunk 无 0 分隔 → false。
func TestSplitTextChunkNoNull(t *testing.T) {
	if _, _, ok := splitTextChunk([]byte("noNullHere")); ok {
		t.Error("无 0 应 false")
	}
}
