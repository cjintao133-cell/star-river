package mobilebind

// P0-3 策略闸 + 注入防护桥：
//   - 全局 policy.Engine：对工具调用做准入（花钱/对外/危险需确认，超预算拒绝，黑白名单）。
//   - 工具动作分类：按工具名/参数推断 Action（send/spend/dangerous/external/tool）。
//   - 注入防护：工具输出经 trust.Sanitize，可疑则加围栏；提供 DetectInjection 供 UI 展示。
// 依赖 kernel/policy、kernel/trust（零依赖），不 import 能力插件。

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"

	"github.com/cjintao133-cell/star-river/kernel/policy"
	"github.com/cjintao133-cell/star-river/kernel/trust"
)

const (
	metaPolicyCfgKey   = "policy.config.v1"
	metaPolicySpentKey = "policy.spent.v1"
)

var (
	polMu     sync.Mutex
	polEngine *policy.Engine
	polLoaded bool
)

// ensurePolicyLocked 惰性建引擎并从 app_meta 恢复配置/花费（调用方持 polMu）。
func ensurePolicyLocked() {
	if polLoaded {
		return
	}
	polLoaded = true
	cfg := policy.Config{}
	var spent float64
	if st := storeRef(); st != nil {
		if v := metaGet(st, metaPolicyCfgKey); v != "" {
			_ = json.Unmarshal([]byte(v), &cfg)
		}
		if v := metaGet(st, metaPolicySpentKey); v != "" {
			var f float64
			if json.Unmarshal([]byte(v), &f) == nil {
				spent = f
			}
		}
	}
	polEngine = policy.New(cfg)
	if spent > 0 {
		polEngine.Commit(policy.Action{Kind: policy.KindSpend, Amount: spent})
	}
}

// resetPolicy 重开库时清空单例，下次访问从新库恢复。
func resetPolicy() {
	polMu.Lock()
	polEngine, polLoaded = nil, false
	polMu.Unlock()
}

func policyEngineRef() *policy.Engine {
	polMu.Lock()
	defer polMu.Unlock()
	ensurePolicyLocked()
	return polEngine
}

func persistPolicySpentLocked() {
	st := storeRef()
	if st == nil || polEngine == nil {
		return
	}
	spent, _ := polEngine.Budget()
	b, _ := json.Marshal(spent)
	metaSet(st, metaPolicySpentKey, string(b))
}

// classifyTool 按工具名与参数推断动作类别与可逆性/金额。
func classifyTool(name, argsJSON string) policy.Action {
	n := strings.ToLower(name)
	a := policy.Action{Kind: policy.KindTool, Name: name, Reversible: true}
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(n, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("send", "email", "message", "post", "tweet", "sms", "notify", "publish", "share", "发送", "邮件", "短信"):
		a.Kind = policy.KindSend
	case has("pay", "purchase", "buy", "checkout", "transfer", "order", "billing", "付款", "支付", "购买", "转账", "下单"):
		a.Kind = policy.KindSpend
		a.Amount = extractAmount(argsJSON)
	case has("delete", "remove", "destroy", "drop", "wipe", "format", "rm_", "uninstall", "删除", "销毁", "清空"):
		a.Kind = policy.KindTool
		a.Reversible = false
	case has("fetch", "http", "request", "download", "upload", "web", "browse", "crawl", "http_request", "请求", "抓取"):
		a.Kind = policy.KindExternal
		a.External = true
	}
	return a
}

func extractAmount(argsJSON string) float64 {
	if strings.TrimSpace(argsJSON) == "" {
		return 0
	}
	var m map[string]any
	if json.Unmarshal([]byte(argsJSON), &m) != nil {
		return 0
	}
	for _, k := range []string{"amount", "price", "cost", "total", "value"} {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case float64:
				if t > 0 {
					return t
				}
			case string:
				var f float64
				if _, err := json.Number(t).Float64(); err == nil {
					return f
				}
			}
		}
	}
	return 0
}

// ---- 桥 ----

// SetPolicyConfig 更新策略配置（JSON: policy.Config）。持久化到 app_meta。
func SetPolicyConfig(cfgJSON string) string {
	var cfg policy.Config
	if strings.TrimSpace(cfgJSON) != "" {
		if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
			return "ERR:bad json"
		}
	}
	polMu.Lock()
	ensurePolicyLocked()
	polEngine.SetConfig(cfg)
	if st := storeRef(); st != nil {
		if b, err := json.Marshal(polEngine.Config()); err == nil {
			metaSet(st, metaPolicyCfgKey, string(b))
		}
	}
	polMu.Unlock()
	return "OK"
}

// GetPolicyConfig 返回当前配置 JSON。
func GetPolicyConfig() string {
	e := policyEngineRef()
	b, _ := json.Marshal(e.Config())
	return string(b)
}

// PolicyCheck 请求判定（不登记为已确认，仅判定 + 必要时生成 pending）。
// kind 可空（会按工具名自动分类）。返回 JSON {decision,reason,pending_id,kind}。
func PolicyCheck(kind, name, argsJSON string) string {
	e := policyEngineRef()
	a := classifyTool(name, argsJSON)
	if strings.TrimSpace(kind) != "" {
		a.Kind = policy.ActionKind(kind)
	}
	d, reason, pid := e.Check(a)
	b, _ := json.Marshal(map[string]any{
		"decision": string(d), "reason": reason, "pending_id": pid, "kind": string(a.Kind), "amount": a.Amount,
	})
	return string(b)
}

// GetBudget 返回 {spent,total}。
func GetBudget() string {
	e := policyEngineRef()
	spent, total := e.Budget()
	b, _ := json.Marshal(map[string]float64{"spent": spent, "total": total})
	return string(b)
}

// ResetBudget 清零已花费。
func ResetBudget() string {
	polMu.Lock()
	ensurePolicyLocked()
	polEngine.ResetSpent()
	persistPolicySpentLocked()
	polMu.Unlock()
	return "OK"
}

// ListPendingConfirms 返回待确认动作 JSON 数组。
func ListPendingConfirms() string {
	e := policyEngineRef()
	b, _ := json.Marshal(e.ListPending())
	return string(b)
}

// ResolveConfirm 处理待确认：approve=true 放行。返回 "OK:allow"/"OK:deny"/"ERR:not_found"。
func ResolveConfirm(id string, approve bool) string {
	e := policyEngineRef()
	_, ok, _ := e.Resolve(id, approve)
	if !ok {
		return "ERR:not_found"
	}
	if approve {
		return "OK:allow"
	}
	return "OK:deny"
}

// FenceToolOutput 对工具输出做注入防护：可疑则加围栏，否则原样。返回处理后文本。
func FenceToolOutput(source, content string) string {
	out, _ := trust.Sanitize(source, content)
	return out
}

// DetectInjection 返回注入信号 JSON 数组（供 UI/审计展示）。
func DetectInjection(text string) string {
	b, _ := json.Marshal(trust.Detect(text))
	return string(b)
}

var _ = base64.StdEncoding
