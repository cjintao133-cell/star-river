// Package browser 是"浏览器自动化"的抽象层：定义驱动接口（导航/快照/点击/输入/滚动/执行），
// 以及把无障碍树(accessibility tree)压缩成 token 友好文本、动作校验等纯逻辑。
//
// 真正的驱动（Playwright/CDP/Android WebView）由入口层实现并注入；本包零依赖、可离线测试
// （内置 FakeSession）。设计对齐 2026 主流做法：无障碍树优先、视觉/截图回退。
package browser

import (
	"errors"
	"fmt"
	"strings"
)

// A11yNode 无障碍树节点（跨端中性结构）。
type A11yNode struct {
	Role     string     `json:"role"` // button/textbox/link/text...
	Name     string     `json:"name,omitempty"`
	Value    string     `json:"value,omitempty"`
	Ref      string     `json:"ref,omitempty"` // 稳定引用（点击用）
	Children []A11yNode `json:"children,omitempty"`
}

// Page 一次页面快照。
type Page struct {
	URL   string   `json:"url"`
	Title string   `json:"title,omitempty"`
	Tree  A11yNode `json:"tree"`
}

// Action 一个浏览器动作。
type Action struct {
	Kind  string `json:"kind"` // navigate|click|type|scroll|wait|evaluate|back|press
	Ref   string `json:"ref,omitempty"`
	URL   string `json:"url,omitempty"`
	Text  string `json:"text,omitempty"`
	Key   string `json:"key,omitempty"`
	Delta int    `json:"delta,omitempty"`
	Expr  string `json:"expr,omitempty"`
}

// Valid 校验动作。
func (a Action) Valid() error {
	switch a.Kind {
	case "navigate":
		if strings.TrimSpace(a.URL) == "" {
			return errors.New("browser: navigate needs url")
		}
	case "click":
		if strings.TrimSpace(a.Ref) == "" {
			return errors.New("browser: click needs ref")
		}
	case "type":
		if strings.TrimSpace(a.Ref) == "" {
			return errors.New("browser: type needs ref")
		}
	case "scroll", "wait", "back":
	case "press":
		if strings.TrimSpace(a.Key) == "" {
			return errors.New("browser: press needs key")
		}
	case "evaluate":
		if strings.TrimSpace(a.Expr) == "" {
			return errors.New("browser: evaluate needs expr")
		}
	default:
		return fmt.Errorf("browser: unknown action %q", a.Kind)
	}
	return nil
}

// Session 浏览器会话驱动接口（入口层实现）。
type Session interface {
	Navigate(url string) (Page, error)
	Snapshot() (Page, error)
	Click(ref string) (Page, error)
	Type(ref, text string) (Page, error)
	Scroll(delta int) (Page, error)
	Back() (Page, error)
	Close() error
}

// Do 执行一个动作并返回新页面快照。
func Do(s Session, a Action) (Page, error) {
	if err := a.Valid(); err != nil {
		return Page{}, err
	}
	switch a.Kind {
	case "navigate":
		return s.Navigate(a.URL)
	case "snapshot", "wait":
		return s.Snapshot()
	case "click":
		return s.Click(a.Ref)
	case "type":
		return s.Type(a.Ref, a.Text)
	case "scroll":
		return s.Scroll(a.Delta)
	case "back":
		return s.Back()
	case "press", "evaluate":
		// 这两类需驱动具体实现，抽象层用快照占位。
		return s.Snapshot()
	}
	return Page{}, fmt.Errorf("browser: unsupported action %q", a.Kind)
}

// Compress 把无障碍树压成"token 友好"的缩进文本（供喂给模型）。
// 只保留有意义的节点（有 name 或有 ref 或是有子节点），限制最大行数。
func Compress(p Page, maxLines int) string {
	if maxLines <= 0 {
		maxLines = 200
	}
	var b strings.Builder
	lines := 0
	var walk func(n A11yNode, depth int)
	walk = func(n A11yNode, depth int) {
		if lines >= maxLines {
			return
		}
		meaningful := n.Name != "" || n.Ref != "" || len(n.Children) > 0
		if meaningful {
			indent := strings.Repeat("  ", depth)
			b.WriteString(indent)
			b.WriteString("- ")
			b.WriteString(n.Role)
			if n.Name != "" {
				b.WriteString(" \"")
				b.WriteString(truncate(n.Name, 80))
				b.WriteString("\"")
			}
			if n.Value != "" {
				b.WriteString(" = ")
				b.WriteString(truncate(n.Value, 60))
			}
			if n.Ref != "" {
				b.WriteString(" [")
				b.WriteString(n.Ref)
				b.WriteString("]")
			}
			b.WriteString("\n")
			lines++
		}
		for _, c := range n.Children {
			walk(c, depth+1)
			if lines >= maxLines {
				return
			}
		}
	}
	walk(p.Tree, 0)
	if lines >= maxLines {
		b.WriteString("…(已截断)\n")
	}
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// ---- FakeSession 供离线测试/演示 ----

// FakeSession 一个内存浏览器（按脚本响应导航/点击）。
type FakeSession struct {
	Pages   map[string]Page // url -> page
	Current Page
	Typed   map[string]string
}

// NewFake 建假浏览器。
func NewFake() *FakeSession {
	home := Page{URL: "about:home", Title: "首页", Tree: A11yNode{Role: "root", Children: []A11yNode{
		{Role: "link", Name: "搜索", Ref: "e1"},
		{Role: "textbox", Name: "关键字", Ref: "e2"},
	}}}
	return &FakeSession{Pages: map[string]Page{}, Current: home, Typed: map[string]string{}}
}

// Navigate 实现 Session。
func (f *FakeSession) Navigate(url string) (Page, error) {
	if p, ok := f.Pages[url]; ok {
		f.Current = p
		return p, nil
	}
	f.Current = Page{URL: url, Title: url, Tree: A11yNode{Role: "root", Children: []A11yNode{{Role: "text", Name: "内容 " + url}}}}
	return f.Current, nil
}

// Snapshot 实现 Session。
func (f *FakeSession) Snapshot() (Page, error) { return f.Current, nil }

// Click 实现 Session（返回当前页，ref 记入 title 便于断言）。
func (f *FakeSession) Click(ref string) (Page, error) {
	f.Current.Title = "clicked:" + ref
	return f.Current, nil
}

// Type 实现 Session。
func (f *FakeSession) Type(ref, text string) (Page, error) {
	f.Typed[ref] = text
	return f.Current, nil
}

// Scroll 实现 Session。
func (f *FakeSession) Scroll(delta int) (Page, error) { return f.Current, nil }

// Back 实现 Session。
func (f *FakeSession) Back() (Page, error) { return f.Navigate("about:home") }

// Close 实现 Session。
func (f *FakeSession) Close() error { return nil }
