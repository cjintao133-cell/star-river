package browser

import (
	"strings"
	"testing"
)

func TestActionValid(t *testing.T) {
	bad := []Action{{Kind: "navigate"}, {Kind: "click"}, {Kind: "type"}, {Kind: "press"}, {Kind: "evaluate"}, {Kind: "bogus"}}
	for _, a := range bad {
		if a.Valid() == nil {
			t.Errorf("%+v 应非法", a)
		}
	}
	good := []Action{{Kind: "navigate", URL: "https://a"}, {Kind: "click", Ref: "e1"}, {Kind: "type", Ref: "e2", Text: "x"}, {Kind: "scroll", Delta: 100}, {Kind: "back"}, {Kind: "press", Key: "Enter"}, {Kind: "evaluate", Expr: "1+1"}}
	for _, a := range good {
		if err := a.Valid(); err != nil {
			t.Errorf("%+v 应合法: %v", a, err)
		}
	}
}

func TestDoFlow(t *testing.T) {
	s := NewFake()
	p, err := Do(s, Action{Kind: "navigate", URL: "https://example.com"})
	if err != nil || p.URL != "https://example.com" {
		t.Fatalf("navigate 失败: %+v %v", p, err)
	}
	p, _ = Do(s, Action{Kind: "click", Ref: "e1"})
	if !strings.Contains(p.Title, "clicked:e1") {
		t.Fatalf("click 未生效: %+v", p)
	}
	if _, err := Do(s, Action{Kind: "bogus"}); err == nil {
		t.Fatal("非法动作应报错")
	}
}

func TestCompress(t *testing.T) {
	p := Page{URL: "u", Tree: A11yNode{Role: "root", Children: []A11yNode{
		{Role: "button", Name: "提交", Ref: "e1"},
		{Role: "textbox", Name: "关键词", Ref: "e2"},
		{Role: "container", Children: []A11yNode{{Role: "text", Name: "说明文字"}}},
	}}}
	out := Compress(p, 100)
	if !strings.Contains(out, `button "提交" [e1]`) {
		t.Fatalf("压缩应含按钮: %s", out)
	}
	if !strings.Contains(out, "textbox") || !strings.Contains(out, "说明文字") {
		t.Fatalf("压缩应含其它节点: %s", out)
	}
	// 截断
	p2 := Page{Tree: A11yNode{Role: "root"}}
	nodes := make([]A11yNode, 0, 300)
	for i := 0; i < 300; i++ {
		nodes = append(nodes, A11yNode{Role: "text", Name: "n", Ref: "r"})
	}
	p2.Tree.Children = nodes
	out2 := Compress(p2, 10)
	if !strings.Contains(out2, "已截断") {
		t.Fatalf("应截断: %s", out2)
	}
}
