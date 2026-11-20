package anthropic

import (
	"encoding/base64"
	"strings"
	"testing"

	"star-river/kernel/core/types"
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
	if !strings.Contains(s, `"type":"image"`) || !strings.Contains(s, `"media_type":"image/png"`) {
		t.Errorf("图片未按 image/source 序列化: %s", s)
	}
	if !strings.Contains(s, base64.StdEncoding.EncodeToString(data)) {
		t.Errorf("图片 base64 数据缺失: %s", s)
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
