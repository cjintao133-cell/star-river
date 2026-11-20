// Command srchat 最小 CLI 聊天闭环（T-M1-08）。
//
// 跑通链路：命令行 → 七层上下文最小版（人设 + 近期消息 + 本次输入，orchestrator.BuildMinimal）
// → openai 兼容适配器流式输出。用于日常对话与真实端点验收。
//
// key 只从 -keyfile 或环境变量 SR_API_KEY 读取，不进命令行参数、不进日志（红线 3）。
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"star-river/kernel/adapter/openai"
	"star-river/kernel/core/types"
	"star-river/kernel/orchestrator"
)

type config struct {
	base     string
	model    string
	persona  string
	keyFile  string
	once     string
	maxMsgs  int
	maxChars int
	maxToken int
	timeout  time.Duration
}

// cleanOut 去掉可能出现的 key（红线 3，双保险）。
func cleanOut(s, key string) string {
	if key != "" {
		s = strings.ReplaceAll(s, key, "[REDACTED]")
	}
	return s
}

func loadKey(keyFile string) (string, error) {
	if keyFile != "" {
		b, err := os.ReadFile(keyFile)
		if err != nil {
			return "", fmt.Errorf("读取 keyfile 失败: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	k := strings.TrimSpace(os.Getenv("SR_API_KEY"))
	if k == "" {
		return "", fmt.Errorf("未提供 key：设 -keyfile 或 环境变量 SR_API_KEY")
	}
	return k, nil
}

func userMsg(text string) types.Message {
	return types.Message{Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: text}}}
}

func assistantMsg(text string) types.Message {
	return types.Message{Role: types.RoleAssistant, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: text}}}
}

// turn 跑一轮：装配上下文 → 流式调用 → 返回助手正文与结果元数据。
// 正文（StreamDelta）写入 stdout 与历史；推理（StreamReasoning）只走 stderr，
// 不污染正文与后续上下文（避免推理串味多轮）。
func turn(ctx context.Context, c *openai.Client, cfg config, key string, history []types.Message, input string) (string, *openai.ChatResult, error) {
	msgs := orchestrator.BuildMinimal(cfg.persona, history, userMsg(input),
		orchestrator.Limits{MaxMessages: cfg.maxMsgs, MaxChars: cfg.maxChars})
	var b strings.Builder
	res, err := c.Chat(ctx, openai.ChatRequest{Model: cfg.model, MaxTokens: cfg.maxToken, Messages: msgs},
		func(e types.StreamEvent) {
			switch e.Kind {
			case types.StreamDelta:
				b.WriteString(e.Text)
				fmt.Print(e.Text)
			case types.StreamReasoning:
				fmt.Fprint(os.Stderr, e.Text)
			case types.StreamError:
				fmt.Printf("\n[error %s]\n", e.ErrCode)
			}
		})
	if err != nil {
		return b.String(), nil, fmt.Errorf("%s", cleanOut(err.Error(), key))
	}
	return b.String(), res, nil
}

func main() {
	var cfg config
	flag.StringVar(&cfg.base, "base", "", "OpenAI 兼容端点 base，如 https://host/v1（必填）")
	flag.StringVar(&cfg.model, "model", "", "模型 id（必填）")
	flag.StringVar(&cfg.persona, "persona", "", "人设系统提示（可选）")
	flag.StringVar(&cfg.keyFile, "keyfile", "", "API key 文件路径（或设环境变量 SR_API_KEY）")
	flag.StringVar(&cfg.once, "once", "", "单发一条消息后退出；留空则进入 REPL")
	flag.IntVar(&cfg.maxMsgs, "maxmsgs", 20, "上下文总消息条数上限（0=不限）")
	flag.IntVar(&cfg.maxChars, "maxchars", 8000, "上下文文本字符预算（0=不限）")
	flag.IntVar(&cfg.maxToken, "maxtokens", 1024, "单次生成 max_tokens")
	flag.DurationVar(&cfg.timeout, "timeout", 90*time.Second, "单次请求超时")
	flag.Parse()

	if cfg.base == "" || cfg.model == "" {
		fmt.Fprintln(os.Stderr, "必须提供 -base 与 -model")
		os.Exit(2)
	}
	key, err := loadKey(cfg.keyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	c := openai.New(cfg.base, key)
	c.HTTP.Timeout = cfg.timeout

	ctx := context.Background()
	var history []types.Message

	if cfg.once != "" {
		reply, res, err := turn(ctx, c, cfg, key, history, cfg.once)
		fmt.Println()
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			os.Exit(1)
		}
		fmt.Printf("[model=%s finish=%s usage=%d/%d/%d]\n",
			res.Model, res.FinishReason, res.Usage.PromptTokens, res.Usage.CompletionTokens, res.Usage.TotalTokens)
		_ = reply
		return
	}

	// REPL
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for {
		fmt.Print("> ")
		if !sc.Scan() {
			fmt.Println()
			return
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if line == ":quit" || line == ":q" {
			return
		}
		reply, res, err := turn(ctx, c, cfg, key, history, line)
		fmt.Println()
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			continue
		}
		fmt.Printf("[finish=%s tokens=%d]\n", res.FinishReason, res.Usage.TotalTokens)
		history = append(history, userMsg(line), assistantMsg(reply))
	}
}
