package event

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// ConsumeOptions 控制 consume 行为。
type ConsumeOptions struct {
	AppID     string
	AppSecret string
	EventKey  string
	BaseURL   string // 飞书 API 域名（默认 https://open.feishu.cn）

	// 输出控制
	Out    io.Writer // 事件 NDJSON 写到这里（通常是 stdout）
	ErrOut io.Writer // 诊断日志写到这里（通常是 stderr）

	// 业务过滤
	JQExpr    string // 暂未实现完整 jq，留作未来扩展（v3.5.3 SDK 不带 jq 库）
	OutputDir string // 非空时把每条事件 dump 为 <event_id>.json 文件

	// 退出条件（whichever fires first）
	MaxEvents int           // 0 = 不限制
	Timeout   time.Duration // 0 = 不限制

	// 守护进程协议
	Bus *Bus // 已构造好的 bus 句柄；nil 时不注册到 bus.json（test 模式）
}

// Runtime 表示一次 consume 会话的运行时状态。
// 单次调用 Run 后由 GC 回收；不可重入。
type Runtime struct {
	opts ConsumeOptions

	received atomic.Int64       // 已发出的事件计数（受 MaxEvents 约束）
	stopOnce atomic.Bool        // 多触发源（signal/timeout/maxEvents）下保证 cancel 只触发一次
	cancel   context.CancelFunc // emit 触发 max-events 退出时调用，由 Run 在派生 subCtx 后注入
	reasonMu sync.Mutex         // 串行写入 reason 字段，避免 timeout/maxEvents 并发竞争
	reason   string             // 多触发源时记录原因；Run 末尾读取
}

// NewRuntime 构造一个 consume runtime。
func NewRuntime(opts ConsumeOptions) *Runtime {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.ErrOut == nil {
		opts.ErrOut = os.Stderr
	}
	if opts.BaseURL == "" {
		opts.BaseURL = "https://open.feishu.cn"
	}
	return &Runtime{opts: opts}
}

// Run 启动 WebSocket 连接 → 注册到 bus.json → 阻塞接收事件直到上下文取消或退出条件触发。
//
// 退出 reason：
//   - "limit"   : 达到 MaxEvents
//   - "timeout" : 达到 Timeout
//   - "signal"  : 上下文取消（Ctrl-C / SIGTERM / stdin EOF）
//   - "error"   : WebSocket 连接持续失败
//
// 退出码 0 表示正常完成；非 0 表示 startup 失败或不可恢复错误。
func (r *Runtime) Run(ctx context.Context) (reason string, err error) {
	def, ok := Lookup(r.opts.EventKey)
	if !ok {
		return "error", fmt.Errorf("未知 EventKey: %q（运行 `feishu-cli event list` 查看支持的 key）", r.opts.EventKey)
	}

	// Register 到 bus.json
	if r.opts.Bus != nil {
		entry := ConsumerEntry{
			PID:        os.Getpid(),
			EventKey:   r.opts.EventKey,
			StartedAt:  time.Now(),
			OutputDir:  r.opts.OutputDir,
			JQExpr:     r.opts.JQExpr,
			MaxEvents:  r.opts.MaxEvents,
			TimeoutSec: int(r.opts.Timeout.Seconds()),
		}
		if err := r.opts.Bus.Register(entry); err != nil {
			fmt.Fprintf(r.opts.ErrOut, "[event] 警告: 注册到 bus.json 失败: %v\n", err)
		}
		defer func() {
			_ = r.opts.Bus.Unregister(os.Getpid(), r.opts.EventKey)
		}()
	}

	// 准备输出目录
	if r.opts.OutputDir != "" {
		if err := os.MkdirAll(r.opts.OutputDir, 0700); err != nil {
			return "error", fmt.Errorf("创建输出目录失败: %w", err)
		}
	}

	// 派生子上下文以便多触发源 cancel
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	r.cancel = cancel // emit 在 max-events 触发时通过 r.cancel 退出

	// 超时
	if r.opts.Timeout > 0 {
		go func() {
			select {
			case <-time.After(r.opts.Timeout):
				if !r.stopOnce.Swap(true) {
					r.setReason("timeout")
				}
				cancel()
			case <-subCtx.Done():
			}
		}()
	}

	// 构造 dispatcher
	dis := dispatcher.NewEventDispatcher("", "")
	dis.OnCustomizedEvent(def.EventType, func(ctx context.Context, ev *larkevent.EventReq) error {
		return r.emit(ev)
	})

	// 安装 panic recover 包装的 logger，避免 SDK 日志炸 stderr
	cli := larkws.NewClient(
		r.opts.AppID, r.opts.AppSecret,
		larkws.WithEventHandler(dis),
		larkws.WithDomain(r.opts.BaseURL),
		larkws.WithAutoReconnect(true),
		larkws.WithLogger(newQuietLogger(r.opts.ErrOut)),
		larkws.WithLogLevel(larkcore.LogLevelWarn),
	)

	// 触发 ready marker：宣告进程初始化完成（WS 握手在 cli.Start 异步执行）。
	// ★ marker 走真实 os.Stderr 而非 r.opts.ErrOut，避免 --quiet 时 ErrOut=io.Discard
	//   导致 orchestrator 父进程永远等不到 marker。
	// ★ 语义提示：父进程看到 marker 后**还需额外等 1-3s 让 WS 握手完成**才能可靠收到事件；
	//   生产环境推荐父进程发"自检事件"+ 等待 echo 来确认链路通。
	fmt.Fprintf(os.Stderr, "[event] ready event_key=%s (init complete; WS handshake in progress)\n", r.opts.EventKey)

	// ws.Client.Start 阻塞，需要外部 cancel；包一层 goroutine 让 ctx 控制退出
	errCh := make(chan error, 1)
	go func() {
		errCh <- cli.Start(subCtx)
	}()

	select {
	case <-subCtx.Done():
		// 正常退出（signal/timeout/maxEvents）— 优先用 emit/timeout goroutine 写入的 reason；
		// 都没写时按计数兜底（仅作为防御性 fallback）。
		final := r.getReason()
		if final == "" {
			if r.received.Load() >= int64(r.opts.MaxEvents) && r.opts.MaxEvents > 0 {
				final = "limit"
			} else {
				final = "signal"
			}
		}
		return final, nil
	case wsErr := <-errCh:
		if wsErr != nil && !isContextCanceled(wsErr) {
			return "error", fmt.Errorf("WebSocket 连接失败: %w", wsErr)
		}
		return "signal", nil
	}
}

// setReason 串行写入 reason 字段。
// 多个触发源（timeout goroutine / emit max-events）可能并发调用，
// 需要 mutex 保证最先到达的 reason 不被覆盖。
func (r *Runtime) setReason(reason string) {
	r.reasonMu.Lock()
	defer r.reasonMu.Unlock()
	if r.reason == "" {
		r.reason = reason
	}
}

// getReason 读取最终 reason。
func (r *Runtime) getReason() string {
	r.reasonMu.Lock()
	defer r.reasonMu.Unlock()
	return r.reason
}

// emit 把一条事件输出到 stdout（NDJSON）+ 可选 output-dir 文件，并维护计数。
func (r *Runtime) emit(ev *larkevent.EventReq) error {
	// 解析事件以提取 event_id（用于文件名）；失败也不阻塞输出。
	body := ev.Body
	var meta struct {
		Header struct {
			EventID   string `json:"event_id"`
			EventType string `json:"event_type"`
		} `json:"header"`
	}
	_ = json.Unmarshal(body, &meta)

	// 简单 jq 支持：仅支持 `.event.xxx` / `.header.xxx` 这种点路径（避免引入 itchyny/gojq 依赖）
	output := body
	if r.opts.JQExpr != "" {
		filtered, ok := applyDotPath(body, r.opts.JQExpr)
		if !ok {
			// jq 不匹配则 skip 该事件
			return nil
		}
		output = filtered
	}

	// 写 stdout（NDJSON）：每条事件一行 + \n
	// 注意：原始 body 已是 JSON，保持紧凑序列化（不 indent）
	var line []byte
	if isCompactJSON(output) {
		line = output
	} else {
		var v interface{}
		if err := json.Unmarshal(output, &v); err == nil {
			line, _ = json.Marshal(v)
		} else {
			line = output
		}
	}
	if _, err := r.opts.Out.Write(append(line, '\n')); err != nil {
		// stdout 关闭（下游 pipe broken）= 立刻退出。
		// ★ 必须主动 cancel，否则 Run 卡在 select{<-subCtx.Done()}
		//   直到外部 Ctrl-C；这是 fix 引入 stopOnce 控制 cancel 后的对称要求。
		if !r.stopOnce.Swap(true) {
			r.setReason("signal") // pipe broken 归 signal（与 Ctrl-C 同义）
			if r.cancel != nil {
				r.cancel()
			}
		}
		return err
	}

	// 写文件（可选）
	// 安全：event_id 来自服务端 payload，恶意/异常 ID 含路径分隔符或 .. 会写到 OutputDir 外面，
	// 必须用 allowlist 限制为 [A-Za-z0-9_-]，并把生成 filename 限制在 OutputDir 子树内。
	if r.opts.OutputDir != "" && meta.Header.EventID != "" {
		safeID := sanitizeEventID(meta.Header.EventID)
		if safeID != "" {
			filename := filepath.Join(r.opts.OutputDir, safeID+".json")
			_ = os.WriteFile(filename, body, 0600)
		}
	}

	// 计数 + 触发 max-events 退出
	n := r.received.Add(1)
	if r.opts.MaxEvents > 0 && n >= int64(r.opts.MaxEvents) {
		fmt.Fprintf(r.opts.ErrOut, "[event] reached max-events=%d\n", r.opts.MaxEvents)
		// stopOnce 保证多触发源（timeout + max-events 同时撞）只 cancel 一次。
		if !r.stopOnce.Swap(true) {
			r.setReason("limit")
			if r.cancel != nil {
				r.cancel()
			}
		}
	}
	return nil
}

// applyDotPath 实现极简 jq：仅支持 `.a.b.c` 形式（不支持过滤器/管道/数组下标）。
// 返回 (子树 JSON, 是否命中)。
func applyDotPath(data []byte, expr string) ([]byte, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "." || expr == "" {
		return data, true
	}
	if !strings.HasPrefix(expr, ".") {
		return nil, false
	}
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, false
	}
	cur := v
	for _, seg := range strings.Split(strings.TrimPrefix(expr, "."), ".") {
		if seg == "" {
			continue
		}
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		next, exists := m[seg]
		if !exists {
			return nil, false
		}
		cur = next
	}
	out, err := json.Marshal(cur)
	if err != nil {
		return nil, false
	}
	return out, true
}

// isCompactJSON 粗略判断 b 是否已是 compact JSON（无换行）。SDK 推过来的 body 通常就是。
func isCompactJSON(b []byte) bool {
	for _, c := range b {
		if c == '\n' {
			return false
		}
	}
	return true
}

// isContextCanceled 判断 err 是否来自 context cancel/deadline（正常退出，不算 error）。
func isContextCanceled(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "context canceled") || strings.Contains(s, "context deadline exceeded")
}

// quietLogger 把 SDK 日志重定向到 errOut（避免污染 stdout NDJSON）。
type quietLogger struct {
	out io.Writer
}

func newQuietLogger(out io.Writer) *quietLogger {
	return &quietLogger{out: out}
}

func (l *quietLogger) Debug(_ context.Context, args ...interface{}) {}
func (l *quietLogger) Info(_ context.Context, args ...interface{}) {
	fmt.Fprintln(l.out, append([]interface{}{"[event/sdk]"}, args...)...)
}
func (l *quietLogger) Warn(_ context.Context, args ...interface{}) {
	fmt.Fprintln(l.out, append([]interface{}{"[event/sdk]"}, args...)...)
}
func (l *quietLogger) Error(_ context.Context, args ...interface{}) {
	fmt.Fprintln(l.out, append([]interface{}{"[event/sdk]"}, args...)...)
}

// sanitizeEventID 把 event_id 限制为 allowlist 字符 [A-Za-z0-9_-]，长度 ≤ 128。
// 不合法字符直接丢弃；结果为空时调用方应跳过文件写入避免空文件名。
// 用于防御 event_id 路径穿越（来自服务端 payload 不可信）。
func sanitizeEventID(id string) string {
	if len(id) > 128 {
		id = id[:128]
	}
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		}
	}
	return b.String()
}
