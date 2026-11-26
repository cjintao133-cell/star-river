package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	mb "github.com/cjintao133-cell/star-river/kernel/mobilebind"
)

var pass, fail int

func chk(name, got string, wantPrefixOK bool) {
	bad := false
	if wantPrefixOK {
		bad = strings.HasPrefix(got, "ERR")
	} else {
		bad = !strings.HasPrefix(got, "ERR")
	}
	if bad {
		fail++
		fmt.Printf("  [FAIL] %-28s => %s\n", name, trunc(got, 160))
	} else {
		pass++
		fmt.Printf("  [ok]   %-28s => %s\n", name, trunc(got, 90))
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func main() {
	dir, _ := os.MkdirTemp("", "srreal")
	db := dir + "/k.db"
	key := base64.StdEncoding.EncodeToString(bytes32(7))
	fmt.Println("== 1. InitKernel ==")
	r := mb.InitKernel(db, key)
	chk("InitKernel", r, true)
	if r != "OK" {
		fmt.Println("  内核起不来，后续跳过")
		return
	}
	defer mb.CloseKernel()

	fmt.Println("== 2. 角色/会话 ==")
	chk("CreateCharacter", mb.CreateCharacter(`{"id":"c1","name":"小星"}`), true)
	var cs map[string]string
	json.Unmarshal([]byte(mb.CreateSession("c1", "测试会话", "m1")), &cs)
	sid := cs["session_id"]
	if sid == "" {
		fmt.Println("  [FAIL] CreateSession 没返回 session_id")
		fail++
	} else {
		pass++
		fmt.Printf("  [ok]   CreateSession => sid=%s\n", sid)
	}

	fmt.Println("== 3. 多模态消息（图片/音频落库） ==")
	png := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G', 1, 2, 3})
	aud := base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 3, 0xAA})
	blocks := `[{"type":"text","text":"看图"},{"type":"image","mime_type":"image/png","data_b64":"` + png + `"},{"type":"audio","mime_type":"audio/mp4","data_b64":"` + aud + `"}]`
	chk("AppendMessageBlocks", mb.AppendMessageBlocks(sid, "user", blocks), true)
	msgs := mb.ListMessages(sid, 50)
	if strings.Contains(msgs, `"type":"image"`) && strings.Contains(msgs, `"has_data":true`) {
		pass++
		fmt.Printf("  [ok]   ListMessages 含图片块元信息\n")
	} else {
		fail++
		fmt.Printf("  [FAIL] ListMessages 丢块: %s\n", trunc(msgs, 200))
	}
	// 取回图片
	var arr []map[string]any
	json.Unmarshal([]byte(msgs), &arr)
	if len(arr) > 0 {
		mid, _ := arr[0]["id"].(string)
		gm := mb.GetMessageMedia(sid, mid, 1)
		if gm == png {
			pass++
			fmt.Printf("  [ok]   GetMessageMedia 字节一致\n")
		} else {
			fail++
			fmt.Printf("  [FAIL] GetMessageMedia 不一致: %s\n", trunc(gm, 80))
		}
	}

	fmt.Println("== 4. 记忆 ==")
	chk("AddMemory", mb.AddMemory("semantic", "用户生日是5月1日", 0.9), true)
	sm := mb.SearchMemory("生日", 5)
	if strings.Contains(sm, "生日") {
		pass++
		fmt.Printf("  [ok]   SearchMemory 命中\n")
	} else {
		fail++
		fmt.Printf("  [FAIL] SearchMemory 没命中: %s\n", trunc(sm, 160))
	}

	fmt.Println("== 5. 上下文压缩 ==")
	for i := 0; i < 8; i++ {
		mb.AppendMessage(sid, "user", fmt.Sprintf("第%d条消息。内容", i))
	}
	cc := mb.BuildChatContext(sid, 3, 0)
	if strings.Contains(cc, "foldedCount") && strings.Contains(cc, "digest") {
		pass++
		fmt.Printf("  [ok]   BuildChatContext 返回摘要字段\n")
	} else {
		fail++
		fmt.Printf("  [FAIL] BuildChatContext: %s\n", trunc(cc, 160))
	}
	chk("CompactSession", mb.CompactSession(sid, 3, 0), true)

	fmt.Println("== 6. 任务运行时（状态机，不用模型） ==")
	tk := mb.TaskCreate("", "", "测试任务", "把事办成")
	if strings.Contains(tk, `"status":"pending"`) {
		pass++
	} else {
		fail++
		fmt.Printf("  [FAIL] TaskCreate: %s\n", trunc(tk, 140))
	}
	tid := ""
	var tmap map[string]any
	json.Unmarshal([]byte(tk), &tmap)
	if v, ok := tmap["id"].(string); ok {
		tid = v
	}
	if tid != "" {
		chk("TaskPause", mb.TaskPause(tid), true)
		chk("TaskResume", mb.TaskResume(tid), true)
		chk("TaskDelete", mb.TaskDelete(tid), true)
	} else {
		fail++
		fmt.Println("  [FAIL] 任务没 id")
	}

	fmt.Println("== 7. 策略闸 ==")
	chk("SetPolicyConfig", mb.SetPolicyConfig(`{"total_budget":100,"auto_approve_spend":true}`), true)
	pc := mb.PolicyCheck("spend", "buy", `{"amount":30}`)
	if strings.Contains(pc, `"decision":"allow"`) {
		pass++
	} else {
		fail++
		fmt.Printf("  [FAIL] PolicyCheck: %s\n", pc)
	}
	pc2 := mb.PolicyCheck("spend", "buy", `{"amount":200}`)
	if strings.Contains(pc2, `"decision":"over_budget"`) {
		pass++
	} else {
		fail++
		fmt.Printf("  [FAIL] 超预算没拦: %s\n", pc2)
	}

	fmt.Println("== 8. 追踪 ==")
	tr := mb.TraceStart("", "", "llm", "chat", "s1")
	if strings.Contains(tr, "trace_id") {
		pass++
	} else {
		fail++
		fmt.Printf("  [FAIL] TraceStart: %s\n", tr)
	}
	var trm map[string]string
	json.Unmarshal([]byte(tr), &trm)
	if trm["trace_id"] != "" {
		chk("TraceEnd", mb.TraceEnd(trm["trace_id"], trm["span_id"], "ok", `{"t":1}`), true)
		if strings.Contains(mb.ListTraces(10), trm["trace_id"]) {
			pass++
		} else {
			fail++
			fmt.Println("  [FAIL] ListTraces 查不到刚写 span")
		}
	}

	fmt.Println("== 9. 钩子 / 身份 / 产物 / 卫生 ==")
	chk("HooksFire", mb.HooksFire("demo", `{}`), true)
	chk("GetIdentity", mb.GetIdentity("小星", "助手"), true)
	chk("RecordTrajectory", mb.RecordTrajectory("note", "跑通", ""), true)
	chk("UIArtifactValidate", mb.UIArtifactValidate(`{"id":"a","kind":"markdown","content":"x"}`), true)
	chk("UIRenderSpecValidate", mb.UIRenderSpecValidate(`{"type":"card","children":[{"type":"text"}]}`), true)
	chk("HygieneAnalyze", mb.HygieneAnalyze(`[]`, ``, 0), true)

	fmt.Println("== 10. MCP ==")
	chk("McpInit", mb.McpInit(dir+"/mcp.json"), true)
	chk("McpServers", mb.McpServers(), true)

	fmt.Printf("\n==== 结果: PASS=%d FAIL=%d ====\n", pass, fail)
	if fail > 0 {
		os.Exit(1)
	}
}

func bytes32(b byte) []byte {
	x := make([]byte, 32)
	for i := range x {
		x[i] = b
	}
	return x
}
