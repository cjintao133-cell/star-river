package cards

import "testing"

func bookCard() *Card {
	return &Card{Book: []BookEntry{
		{Keys: []string{"海", "示例"}, Content: "示例里有鲸鱼", Enabled: true},
		{Keys: []string{"鲸鱼"}, Content: "鲸鱼会唱歌", Enabled: true},  // 由第1条内容触发（深度1）
		{Keys: []string{"唱歌"}, Content: "歌唱者", Enabled: true},    // 由第2条触发（深度2）
		{Keys: []string{"歌唱者"}, Content: "回环到示例", Enabled: true}, // 触发"海"→循环，应被循环检测拦截
		{Keys: []string{}, Content: "常驻", Enabled: true, Constant: true},
		{Keys: []string{"海"}, Content: "禁用不触发", Enabled: false},
	}}
}

// 递归激活：海→鲸鱼→唱歌（深度级联），循环检测防"歌唱者→回环"。
func TestActivateRecursiveWithDepthAndCycle(t *testing.T) {
	c := bookCard()
	act := c.Activate("今天去海边看海", 3)
	ids := map[int]bool{}
	for _, a := range act {
		ids[a.Index] = true
	}
	// 命中：0(海) 深度0，1(鲸鱼) 深度1，2(唱歌) 深度2。
	for _, must := range []int{0, 1, 2} {
		if !ids[must] {
			t.Errorf("条目 %d 应被激活; act=%v", must, act)
		}
	}
	// 3(歌唱者) 会被"唱歌"触发（深度3），但它的内容含"海"想回环到0——0 已激活，循环检测拦截。
	// constant(4) 不计入激活；disabled(5) 不激活。
	if ids[4] {
		t.Error("constant 不该出现在 Activate")
	}
	if ids[5] {
		t.Error("disabled 不该被激活")
	}
}

// 深度上限：maxDepth=0（用默认3）会到唱歌；maxDepth=1 只到鲸鱼。
func TestActivateDepthLimit(t *testing.T) {
	c := bookCard()
	act := c.Activate("海边", 1) // 深度 0 命中0(海)，深度1 命中1(鲸鱼)，深度2 超上限
	ids := map[int]bool{}
	for _, a := range act {
		ids[a.Index] = true
	}
	if !ids[0] || !ids[1] {
		t.Errorf("深度<=1 应含 0,1; act=%v", act)
	}
	if ids[2] {
		t.Error("深度上限 1 不应到条目2(唱歌, 需深度2)")
	}
}

// 无触发词命中 → 无激活。
func TestActivateNoMatch(t *testing.T) {
	c := bookCard()
	if act := c.Activate("今天天气不错", 3); len(act) != 0 {
		t.Errorf("无关文本不应激活, got %v", act)
	}
}

// constant 条目单独取，且 enabled 过滤。
func TestConstantEntries(t *testing.T) {
	c := bookCard()
	cs := c.ConstantEntries()
	if len(cs) != 1 || cs[0].Content != "常驻" {
		t.Errorf("constant = %+v", cs)
	}
}

// 大小写不敏感匹配。
func TestActivateCaseInsensitive(t *testing.T) {
	c := &Card{Book: []BookEntry{{Keys: []string{"Hello"}, Content: "x", Enabled: true}}}
	if act := c.Activate("say hello world", 3); len(act) != 1 {
		t.Errorf("大小写应不敏感, act=%v", act)
	}
}
