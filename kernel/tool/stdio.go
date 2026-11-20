package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// defaultStderrTail 诊断用 stderr 环形缓冲上限（字节）。
const defaultStderrTail = 8 << 10 // 8KiB

// StdioConfig stdio 传输的子进程配置。
type StdioConfig struct {
	Command string   // 可执行文件路径（如 MCP 服务器程序）
	Args    []string // 参数
	Env     []string // 追加环境变量（nil 表示继承父进程）
	Dir     string   // 工作目录（空=当前）
}

// NotificationHandler 处理服务器主动推送的通知（notifications/*）。
type NotificationHandler func(method string, params json.RawMessage)

// StdioTransport 实现 MCP 的 stdio 传输：把 MCP 服务器作为子进程拉起，
// 消息按行分隔（每条 JSON-RPC 独占一行，不得含内嵌换行），经子进程 stdin/stdout 往返。
//
// 依赖方向（红线 5 平台差异）：stdio MCP **产品能力仅 Windows**（见 PlatformCapability）。
// 本传输层用 os/exec 实现、平台中立，便于在 Linux 上以真实子进程做隔离测试；
// 是否在运行时启用由平台能力表决定（Android 仅远程 MCP）。
//
// 崩溃隔离：子进程退出只会让本进程内挂起请求返回错误并标记死亡，**不影响宿主进程**。
type StdioTransport struct {
	cfg StdioConfig

	mu           sync.Mutex
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	pending      map[int64]chan *rpcMessage
	nextID       int64
	started      bool
	exited       bool
	exitErr      error
	done         chan struct{}
	notifHandler NotificationHandler
	stderrBuf    []byte
	writeMu      sync.Mutex
}

// NewStdioTransport 建 stdio 传输（尚未拉起子进程）。
func NewStdioTransport(cfg StdioConfig) *StdioTransport {
	return &StdioTransport{cfg: cfg, pending: make(map[int64]chan *rpcMessage)}
}

// SetNotificationHandler 设置服务器通知回调（可为 nil）。
func (t *StdioTransport) SetNotificationHandler(h NotificationHandler) {
	t.mu.Lock()
	t.notifHandler = h
	t.mu.Unlock()
}

// Start 拉起子进程并启动读写协程。重复 Start 返回错误。
func (t *StdioTransport) Start() error {
	t.mu.Lock()
	if t.started && !t.exited {
		t.mu.Unlock()
		return &APIError{Kind: KindProtocol, Message: "stdio transport already started"}
	}
	t.mu.Unlock()

	cmd := exec.Command(t.cfg.Command, t.cfg.Args...)
	cmd.Dir = t.cfg.Dir
	if t.cfg.Env != nil {
		cmd.Env = append(cmd.Environ(), t.cfg.Env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return &APIError{Kind: KindProtocol, Message: "stdin pipe: " + err.Error()}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &APIError{Kind: KindProtocol, Message: "stdout pipe: " + err.Error()}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return &APIError{Kind: KindProtocol, Message: "stderr pipe: " + err.Error()}
	}
	if err := cmd.Start(); err != nil {
		return &APIError{Kind: KindNetwork, Message: "start subprocess: " + err.Error()}
	}

	done := make(chan struct{})
	t.mu.Lock()
	t.cmd = cmd
	t.stdin = stdin
	t.started = true
	t.exited = false
	t.exitErr = nil
	t.done = done
	t.pending = make(map[int64]chan *rpcMessage)
	t.mu.Unlock()

	go t.readStdout(stdout, done)
	go t.readStderr(stderr)
	go t.waitProcess(cmd, done)
	return nil
}

// readStdout 读取子进程 stdout，逐行解析 JSON-RPC 并分派。
func (t *StdioTransport) readStdout(r io.ReadCloser, done chan struct{}) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxSSELineBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// stdout 出现非 JSON 行：记录到 stderr 缓冲（不致命），继续。
			t.appendStderr("[non-json stdout] " + line)
			continue
		}
		t.dispatch(&msg)
	}
	// stdout 关闭（EOF）视为进程即将退出；waitProcess 会给出退出码。
	t.markDead(nil)
}

// readStderr 收集子进程 stderr 到有界环形缓冲（诊断用）。
func (t *StdioTransport) readStderr(r io.ReadCloser) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			t.appendStderr(string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

// waitProcess 回收子进程并记录退出原因。
func (t *StdioTransport) waitProcess(cmd *exec.Cmd, done chan struct{}) {
	err := cmd.Wait()
	t.markDead(err)
}

// markDead 标记进程死亡：关闭 done（唤醒所有挂起请求）并记录退出原因。幂等。
func (t *StdioTransport) markDead(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.exited {
		return
	}
	t.exited = true
	if err != nil {
		t.exitErr = err
	} else {
		t.exitErr = errors.New("process exited")
	}
	if t.done != nil {
		close(t.done)
	}
	t.pending = make(map[int64]chan *rpcMessage)
}

// dispatch 分派一条入站消息：响应投递到挂起请求；通知回调；服务端请求回 method-not-found。
func (t *StdioTransport) dispatch(msg *rpcMessage) {
	if msg.ID != nil && msg.Method == "" {
		t.mu.Lock()
		ch := t.pending[*msg.ID]
		delete(t.pending, *msg.ID)
		t.mu.Unlock()
		if ch != nil {
			select {
			case ch <- msg:
			default:
			}
		}
		return
	}
	if msg.Method != "" && msg.ID == nil {
		t.mu.Lock()
		h := t.notifHandler
		t.mu.Unlock()
		if h != nil {
			h(msg.Method, msg.Params)
		}
		return
	}
	if msg.Method != "" && msg.ID != nil {
		// v1 不处理服务端发起的请求，礼貌回 method-not-found。
		id := *msg.ID
		resp, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": id,
			"error": map[string]any{"code": -32601, "message": "method not found (client ignores server requests in v1)"},
		})
		_ = t.writeLine(resp)
	}
}

// appendStderr 追加到有界 stderr 缓冲。
func (t *StdioTransport) appendStderr(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stderrBuf = append(t.stderrBuf, s...)
	if len(t.stderrBuf) > defaultStderrTail {
		t.stderrBuf = t.stderrBuf[len(t.stderrBuf)-defaultStderrTail:]
	}
}

// StderrTail 返回最近 stderr 文本。
func (t *StdioTransport) StderrTail() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.stderrBuf)
}

// writeLine 以换行分隔写一条消息到子进程 stdin。
func (t *StdioTransport) writeLine(body []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	t.mu.Lock()
	stdin := t.stdin
	exited := t.exited
	t.mu.Unlock()
	if stdin == nil || exited {
		return &APIError{Kind: KindNetwork, Message: "process not running"}
	}
	if _, err := stdin.Write(append(body, '\n')); err != nil {
		return &APIError{Kind: KindNetwork, Message: "write stdin: " + err.Error()}
	}
	return nil
}

// Alive 报告子进程是否在运行。
func (t *StdioTransport) Alive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.started && !t.exited
}

// ExitErr 子进程退出原因（未退出返回 nil）。
func (t *StdioTransport) ExitErr() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exitErr
}

// request 发送 JSON-RPC 请求并等待匹配应答，受 ctx 约束。
func (t *StdioTransport) request(ctx context.Context, method string, params any) (*rpcMessage, error) {
	t.mu.Lock()
	if !t.started || t.exited {
		ex := t.exitErr
		t.mu.Unlock()
		if ex != nil {
			return nil, &APIError{Kind: KindNetwork, Message: "process exited: " + ex.Error()}
		}
		return nil, &APIError{Kind: KindNetwork, Message: "process not running"}
	}
	t.nextID++
	id := t.nextID
	ch := make(chan *rpcMessage, 1)
	t.pending[id] = ch
	done := t.done
	t.mu.Unlock()

	body, err := encodeRequest(id, method, params)
	if err != nil {
		t.removePending(id)
		return nil, err
	}
	if err := t.writeLine(body); err != nil {
		t.removePending(id)
		return nil, err
	}

	select {
	case <-ctx.Done():
		t.removePending(id)
		k := KindTimeout
		if errors.Is(ctx.Err(), context.Canceled) {
			k = KindCanceled
		}
		return nil, &APIError{Kind: k, Message: "request " + method + " " + ctx.Err().Error()}
	case m := <-ch:
		return m, nil
	case <-done:
		t.removePending(id)
		ex := t.ExitErr()
		msg := "process exited during request"
		if ex != nil {
			msg = "process exited: " + ex.Error()
		}
		return nil, &APIError{Kind: KindNetwork, Message: msg}
	}
}

// removePending 清理挂起请求。
func (t *StdioTransport) removePending(id int64) {
	t.mu.Lock()
	delete(t.pending, id)
	t.mu.Unlock()
}

// notify 发送 JSON-RPC 通知（无 id，不等应答）。
func (t *StdioTransport) notify(method string, params any) error {
	body, err := encodeNotification(method, params)
	if err != nil {
		return err
	}
	return t.writeLine(body)
}

// Close 终止子进程（先关 stdin 让其优雅退出，超时不候由调用方 Restart）。
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	cmd := t.cmd
	stdin := t.stdin
	t.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return nil
}
