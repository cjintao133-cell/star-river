package openai

import (
	"context"
	"net/http"
	"testing"
)

// /models 富字段（context_length/architecture/supported_parameters）应被解析。
func TestModelsRichFields(t *testing.T) {
	body := `{"data":[
		{"id":"gpt-4o","owned_by":"openai","created":1,"context_length":128000,
		 "architecture":{"modality":"text+image->text","input_modalities":["text","image"],"output_modalities":["text"]},
		 "supported_parameters":["tools","temperature"]},
		{"id":"plain-model","owned_by":"x"}
	]}`
	cl, _ := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) { writeJSON(w, 200, []byte(body)) })
	infos, err := cl.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Fatalf("应 2 条, got %d", len(infos))
	}
	if infos[0].ContextLength != 128000 {
		t.Errorf("context_length 未解析: %d", infos[0].ContextLength)
	}
	if len(infos[0].Architecture.InputModalities) != 2 || infos[0].Architecture.InputModalities[1] != "image" {
		t.Errorf("模态未解析: %v", infos[0].Architecture.InputModalities)
	}
	if len(infos[0].SupportedParameters) != 2 {
		t.Errorf("supported_parameters 未解析: %v", infos[0].SupportedParameters)
	}
	// 缺失时零值容错
	if infos[1].ContextLength != 0 || len(infos[1].Architecture.InputModalities) != 0 {
		t.Errorf("缺失字段应零值: %+v", infos[1])
	}
}
