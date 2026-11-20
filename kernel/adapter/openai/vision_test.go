package openai

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

func TestVisionImageWire(t *testing.T) {
	data := []byte("PNGDATA")
	req := ChatRequest{Model: "m", MaxTokens: 10, Messages: []types.Message{{
		ID: "u1", SessionID: "s", Role: types.RoleUser, Blocks: []types.ContentBlock{
			{Type: types.BlockText, Text: "看图"},
			{Type: types.BlockImage, MIMEType: "image/png", Data: data},
		},
	}}}
	b, err := buildBody(req, false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"image_url"`) || !strings.Contains(s, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(data)) {
		t.Errorf("图片未按 image_url data URI 序列化: %s", s)
	}
	if !strings.Contains(s, "看图") {
		t.Errorf("文本段丢失: %s", s)
	}
}

func TestVisionMissingData(t *testing.T) {
	req := ChatRequest{Model: "m", MaxTokens: 10, Messages: []types.Message{{
		ID: "u", SessionID: "s", Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockImage, MIMEType: "", Data: nil}},
	}}}
	if _, err := buildBody(req, false); err == nil {
		t.Error("缺 mime/data 的图片应报错")
	}
}

func TestTextOnlyStaysString(t *testing.T) {
	req := ChatRequest{Model: "m", MaxTokens: 10, Messages: []types.Message{{
		ID: "u", SessionID: "s", Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}},
	}}}
	b, _ := buildBody(req, false)
	if !strings.Contains(string(b), `"content":"hi"`) {
		t.Errorf("纯文本应保持 string content（不破坏旧语义）: %s", b)
	}
}
