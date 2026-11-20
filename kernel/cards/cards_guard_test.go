package cards

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
)

func buildZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseCharxValid(t *testing.T) {
	b := buildZip(t, map[string][]byte{
		"card.json":    []byte(`{"name":"示例角色","description":"示例"}`),
		"assets/a.png": []byte("xx"),
	})
	c, err := ParseCharx(b)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if c.Name != "示例角色" {
		t.Fatalf("name=%q", c.Name)
	}
}

// 超量条目（zip 炸弹）必须拒绝。
func TestParseCharxTooManyEntries(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i <= maxZipEntries; i++ {
		w, err := zw.Create(fmt.Sprintf("f%d.txt", i))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(nil)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCharx(buf.Bytes()); !errors.Is(err, ErrBadFormat) {
		t.Fatalf("应拒绝超量条目, err=%v", err)
	}
}

// 恶意 PNG：chunk 声明超大 length 但数据不足，解析不得 panic，应优雅返回错误。
func TestParsePNGHugeChunkNoPanic(t *testing.T) {
	b := append([]byte{}, pngMagic...)
	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], 0xFFFFFFFF) // 声明 4GB
	b = append(b, lb[:]...)
	b = append(b, []byte("tEXt")...)
	b = append(b, []byte("xx")...) // 实际数据远不足
	if c, err := ParsePNG(b); err == nil {
		t.Fatalf("应返回错误而非解析出卡: %+v", c)
	}
}
