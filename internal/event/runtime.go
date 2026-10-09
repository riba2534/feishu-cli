package event

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
)

// ConsumeOptions 控制 consume 行为。
type ConsumeOptions struct {
	AppID     string
	AppSecret string
	// EventKey 单个 EventKey（兼容旧调用方）；EventKeys 非空时以 EventKeys 为准。
	EventKey string
	// EventKeys 同一进程内一起订阅的多个 EventKey：共用一条 WebSocket 连接，
	// 避免同 App 多连接被服务端随机分发事件而互相"抢"事件。
	EventKeys []string
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

	// UserAccessToken 供需要服务端订阅注册的 EventKey（KeyDefinition.SubscribePath 非空，
	// 如审批 v4、VC participant/note/recording）在 consume 启动前以 User 身份注册订阅关系。
	UserAccessToken string

	// 守护进程协议
	Bus *Bus // 已构造好的 bus 句柄；nil 时不注册到 bus.json（test 模式）

	// StartWS 注入 WebSocket 启动（测试用）。onHandshake 必须在真实连接就绪后调用一次。
	// nil 时走 oapi-sdk-go ws.Client；SDK 无 OnConnected 回调，生产路径用其
	// Dial 成功后的 "connected to" Info 日志作为握手证明。
	StartWS func(ctx context.Context, onHandshake func()) error

	// ReadyOut 覆盖 ready marker 输出；nil 时写 os.Stderr（--quiet 也不吞，供父进程等待）。
	ReadyOut io.Writer

	// ConsumerPID 覆盖写入 bus.json 的 PID；0 表示 os.Getpid()。仅测试用于模拟并发 consumer。
	ConsumerPID int

	// AckUnsubscribed 为 true 时，未订阅的已知事件类型 ACK 后本地丢弃。
	// 仅应在确认本连接是该 App 唯一的长连接时开启（远端预检 online_instance_cnt=0）：
	// 存在其他连接（其他机器/服务）时，ACK 会让本应由对方处理的事件就此丢失；
	// 不开启时这类事件由 SDK 回 500，服务端可重投给其他连接。
	AckUnsubscribed bool
}

// Runtime 表示一次 consume 会话的运行时状态。
// 单次调用 Run 后由 GC 回收；不可重入。
type Runtime struct {
	opts ConsumeOptions

	received  atomic.Int64       // 已发出的事件计数（受 MaxEvents 约束）
	stopOnce  atomic.Bool        // 多触发源（signal/timeout/maxEvents）下保证 cancel 只触发一次
	readyOnce atomic.Bool        // ready marker 只发一次，且必须在 pre-consume + 握手之后
	cancel    context.CancelFunc // emit 触发 max-events 退出时调用，由 Run 在派生 subCtx 后注入
	reasonMu  sync.Mutex         // 串行写入 reason 字段，避免 timeout/maxEvents 并发竞争
	reason    string             // 多触发源时记录原因；Run 末尾读取

	dedup        *dedupFilter // 按 event_id 去重（服务端重投 / 重连补投）
	droppedTypes sync.Map     // 已提示过的"未订阅事件类型"，每种只提示一次
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
	return &Runtime{opts: opts, dedup: newDedupFilter()}
}

// NormalizeEventKeys 把命令行传入的 EventKey（支持逗号分隔）展开、去空白、去重保序。
func NormalizeEventKeys(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range raw {
		for _, k := range strings.Split(item, ",") {
			k = strings.TrimSpace(k)
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// eventKeys 返回本次要消费的 EventKey 列表（EventKeys 优先，兼容单 EventKey）。
func (r *Runtime) eventKeys() []string {
	if len(r.opts.EventKeys) > 0 {
		return NormalizeEventKeys(r.opts.EventKeys)
	}
	return NormalizeEventKeys([]string{r.opts.EventKey})
}

// eventKeyLabel 用于 ready marker 等展示：多个 key 以逗号连接（单 key 时与旧版完全一致）。
func (r *Runtime) eventKeyLabel() string {
	return strings.Join(r.eventKeys(), ",")
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
	keys := r.eventKeys()
	if len(keys) == 0 {
		return "error", fmt.Errorf("未指定 EventKey（运行 `feishu-cli event list` 查看支持的 key）")
	}
	defs := make([]KeyDefinition, 0, len(keys))
	for _, key := range keys {
		def, ok := Lookup(key)
		if !ok {
			return "error", fmt.Errorf("未知 EventKey: %q（运行 `feishu-cli event list` 查看支持的 key）", key)
		}
		defs = append(defs, def)
	}
	if err := ValidateDotPathExpr(r.opts.JQExpr); err != nil {
		return "error", err
	}
	if err := ValidateOutputDir(r.opts.OutputDir); err != nil {
		return "error", err
	}

	// 每个 consumer 都幂等 POST subscribe（服务端幂等），ready 前必须订阅成功。
	// 不能「first 才 subscribe」：first 的 subscribe 阻塞/失败时 second 会跳过并提前 ready。
	// 注销仍只由 last-consumer 执行，避免先退出者打断同伴。
	// 多 EventKey 时按 key 逐个登记 bus.json / 注册订阅，退出时逐个释放（defer 逆序执行）。
	pid := r.consumerPID()
	weSubscribed := map[string]bool{}
	for _, def := range defs {
		def := def
		if r.opts.Bus != nil {
			entry := ConsumerEntry{
				PID:        pid,
				EventKey:   def.Key,
				StartedAt:  time.Now(),
				OutputDir:  r.opts.OutputDir,
				JQExpr:     r.opts.JQExpr,
				MaxEvents:  r.opts.MaxEvents,
				TimeoutSec: int(r.opts.Timeout.Seconds()),
			}
			if _, claimErr := r.opts.Bus.ClaimConsumer(entry); claimErr != nil {
				if def.SubscribePath != "" || def.UnsubscribePath != "" {
					return "error", fmt.Errorf("注册到 bus.json 失败: %w", claimErr)
				}
				fmt.Fprintf(r.opts.ErrOut, "[event] 警告: 注册到 bus.json 失败: %v\n", claimErr)
				continue
			}
			defer r.releaseConsumer(pid, def, weSubscribed)
		} else if def.UnsubscribePath != "" {
			defer func() {
				if weSubscribed[def.Key] {
					r.unregisterSubscriptions(def)
				}
			}()
		}
	}

	for _, def := range defs {
		if def.SubscribePath == "" {
			continue
		}
		if err := r.registerSubscriptions(ctx, def); err != nil {
			return "error", err
		}
		weSubscribed[def.Key] = true
	}

	// 准备输出目录
	if r.opts.OutputDir != "" {
		if err := safefile.MkdirAll(r.opts.OutputDir, 0700); err != nil {
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

	// 构造 dispatcher：同一条连接注册全部订阅的事件类型；其余已知类型 ACK 后本地丢弃。
	dis := r.buildDispatcher(defs)

	handshakeCh := make(chan struct{})
	var handshakeOnce sync.Once
	onHandshake := func() {
		handshakeOnce.Do(func() { close(handshakeCh) })
	}

	// ws.Client.Start 在成功握手后会永远阻塞；失败则返回 error。
	// ready 只能在 pre-consume（上面已完成）且握手信号到达之后发出。
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.runWebSocket(subCtx, dis, onHandshake)
	}()

	select {
	case <-handshakeCh:
		r.emitReady()
		select {
		case <-subCtx.Done():
			return r.drainWSExit(errCh)
		case wsErr := <-errCh:
			return r.wsExit(wsErr)
		}
	case wsErr := <-errCh:
		// 握手前失败：禁止发 ready。
		return r.wsExit(wsErr)
	case <-subCtx.Done():
		return r.drainWSExit(errCh)
	}
}

// releaseConsumer 从 bus.json 移除 (PID, EventKey)；若本进程是该 key 的最后一个 consumer
// 且本进程注册过服务端订阅，则注销订阅（并复检竞态，必要时恢复）。
func (r *Runtime) releaseConsumer(pid int, def KeyDefinition, weSubscribed map[string]bool) {
	last, relErr := r.opts.Bus.ReleaseConsumer(pid, def.Key)
	if relErr != nil {
		fmt.Fprintf(r.opts.ErrOut, "[event] 警告: 从 bus.json 移除失败: %v\n", relErr)
		return
	}
	if def.UnsubscribePath == "" || !last || !weSubscribed[def.Key] {
		return
	}
	r.unregisterSubscriptions(def)
	// 注销是锁外的网络调用，期间可能有新 consumer 完成注册并订阅。
	// 复检：若此刻已有存活 consumer，说明我们刚把它的服务端订阅抹掉了，
	// 必须补回去——否则它仍在运行且已 ready，却静默收不到任何事件。
	if n, cntErr := r.opts.Bus.CountEventKeyConsumers(def.Key); cntErr == nil && n > 0 {
		fmt.Fprintf(r.opts.ErrOut, "[event] 注销后检测到 %d 个新 consumer，正在恢复服务端订阅\n", n)
		if subErr := r.registerSubscriptions(context.Background(), def); subErr != nil {
			fmt.Fprintf(r.opts.ErrOut, "[event] 警告: 恢复订阅失败，存活 consumer 可能收不到事件: %v\n", subErr)
		}
	}
}

// buildDispatcher 在同一个 dispatcher 上注册本进程订阅的全部事件类型；AckUnsubscribed 时
// 再为其余已知事件类型注册"ACK 后本地丢弃"的处理器。
//
// 原因：飞书长连接按 App 维度把事件随机投递到任一条连接，不管这条连接注册了哪些类型。
// 没注册处理器的类型 SDK 会回 500，服务端随后重投，事件被延迟甚至在多连接间来回漂移；
// 显式 ACK 并丢弃可以让未订阅类型干净地结束，订阅的类型照常输出。
// 卡片回调（card.action.trigger）是同步回调，未订阅时不代答，避免吞掉其他处理方的响应。
func (r *Runtime) buildDispatcher(defs []KeyDefinition) *dispatcher.EventDispatcher {
	dis := dispatcher.NewEventDispatcher("", "")
	registered := map[string]bool{}
	for _, def := range defs {
		if registered[def.EventType] {
			continue
		}
		registered[def.EventType] = true
		if def.CardCallback {
			dis.OnP2CardActionTrigger(func(ctx context.Context, ev *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
				if ev != nil && ev.EventReq != nil {
					_ = r.emit(ev.EventReq)
				}
				// 返回空响应 = ACK 且不更新卡片；卡片回写由消费方用 event.token 调 OpenAPI 完成
				return &callback.CardActionTriggerResponse{}, nil
			})
			continue
		}
		dis.OnCustomizedEvent(def.EventType, func(ctx context.Context, ev *larkevent.EventReq) error {
			return r.emit(ev)
		})
	}
	if !r.opts.AckUnsubscribed {
		return dis
	}
	for _, et := range ackOnlyEventTypes() {
		if registered[et] {
			continue
		}
		registered[et] = true
		eventType := et
		dis.OnCustomizedEvent(eventType, func(ctx context.Context, ev *larkevent.EventReq) error {
			r.noteDropped(eventType)
			return nil
		})
	}
	return dis
}

// noteDropped 每种未订阅事件类型只提示一次，避免刷屏。
func (r *Runtime) noteDropped(eventType string) {
	if _, loaded := r.droppedTypes.LoadOrStore(eventType, true); loaded {
		return
	}
	fmt.Fprintf(r.opts.ErrOut, "[event] 收到未订阅的事件类型 %s，已 ACK 并本地丢弃（本进程只输出 %s）\n", eventType, r.eventKeyLabel())
}

// wsShutdownGrace 取消后等待 WebSocket goroutine 收尾的窗口。
// cli.Start(ctx) 在 ctx 取消后应立即返回，这里只是防止极端情况下无限等待。
const wsShutdownGrace = 2 * time.Second

// drainWSExit 在 subCtx 取消（SIGINT / --timeout / 达到 MaxEvents）后收尾 WebSocket goroutine。
//
// 不能直接 return：那样 Run 会在 goroutine 仍阻塞在 cli.Start(ctx) 时返回，
// 既泄漏 goroutine 与其持有的 WS 连接，也会丢弃关停期间到达的真实连接错误，
// 使一次断链失败被报成干净的 "signal" 退出（event consume 以 exit 0 结束）。
// 取消后 Start 应很快返回，故给一个短窗口等待；超时则按取消原因退出，不再无限等。
func (r *Runtime) drainWSExit(errCh <-chan error) (string, error) {
	select {
	case wsErr := <-errCh:
		return r.wsExit(wsErr)
	case <-time.After(wsShutdownGrace):
		return r.exitReason(), nil
	}
}

func (r *Runtime) runWebSocket(ctx context.Context, dis *dispatcher.EventDispatcher, onHandshake func()) error {
	if r.opts.StartWS != nil {
		return r.opts.StartWS(ctx, onHandshake)
	}
	logger := &handshakeLogger{
		inner:       newQuietLogger(r.opts.ErrOut),
		onHandshake: onHandshake,
	}
	cli := larkws.NewClient(
		r.opts.AppID, r.opts.AppSecret,
		larkws.WithEventHandler(dis),
		larkws.WithDomain(r.opts.BaseURL),
		larkws.WithAutoReconnect(true),
		larkws.WithLogger(logger),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)
	return cli.Start(ctx)
}

func (r *Runtime) wsExit(wsErr error) (string, error) {
	if wsErr != nil && !isContextCanceled(wsErr) {
		return "error", fmt.Errorf("WebSocket 连接失败: %w", wsErr)
	}
	return r.exitReason(), nil
}

func (r *Runtime) exitReason() string {
	final := r.getReason()
	if final != "" {
		return final
	}
	if r.received.Load() >= int64(r.opts.MaxEvents) && r.opts.MaxEvents > 0 {
		return "limit"
	}
	return "signal"
}

// emitReady 写出 AI 面向的稳定 ready 行。必须在 pre-consume 与真实握手之后调用。
func (r *Runtime) emitReady() {
	if !r.readyOnce.CompareAndSwap(false, true) {
		return
	}
	w := r.opts.ReadyOut
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "[event] ready event_key=%s\n", r.eventKeyLabel())
}

// handshakeLogger 把 SDK 握手成功的 Info 日志转成 onHandshake。
// oapi-sdk-go v3.5.3 ws.Client 没有 OnConnected；connect() 在 Dial 得到 HTTP 101 后
// 会打 "connected to ..."。不得把 "disconnected to" 误判为就绪。
type handshakeLogger struct {
	inner       larkcore.Logger
	onHandshake func()
}

func (l *handshakeLogger) Debug(ctx context.Context, args ...interface{}) {
	if l.inner != nil {
		l.inner.Debug(ctx, args...)
	}
}
func (l *handshakeLogger) Info(ctx context.Context, args ...interface{}) {
	if looksLikeWSConnected(args) && l.onHandshake != nil {
		l.onHandshake()
	}
	if l.inner != nil {
		l.inner.Info(ctx, args...)
	}
}
func (l *handshakeLogger) Warn(ctx context.Context, args ...interface{}) {
	if l.inner != nil {
		l.inner.Warn(ctx, args...)
	}
}
func (l *handshakeLogger) Error(ctx context.Context, args ...interface{}) {
	if l.inner != nil {
		l.inner.Error(ctx, args...)
	}
}

func looksLikeWSConnected(args []interface{}) bool {
	for _, a := range args {
		s, ok := a.(string)
		if !ok {
			continue
		}
		if strings.Contains(s, "connected to ") && !strings.Contains(s, "disconnected") {
			return true
		}
	}
	return false
}

// subscribeHTTPTimeout 订阅注册请求的超时上限：注册是 consume 启动的前置步骤，
// 不能因端点挂起阻塞整个启动流程。
const subscribeHTTPTimeout = 15 * time.Second

// unsubscribeHTTPTimeout 对齐官方 PreConsume cleanup（5s），避免 last-consumer
// 注销被取消的 consume ctx 卡住，也不要把 15s 启动超时套到退出路径。
const unsubscribeHTTPTimeout = 5 * time.Second

func (r *Runtime) consumerPID() int {
	if r.opts.ConsumerPID != 0 {
		return r.opts.ConsumerPID
	}
	return os.Getpid()
}

// subscriptionRequestBodies 构造服务端订阅/退订请求体。
// VC 走 {"event_type": EventType}；审批走 {"subscription_type": ...}。
func subscriptionRequestBodies(def KeyDefinition) []map[string]string {
	if def.SubscribeEventType {
		return []map[string]string{{"event_type": def.EventType}}
	}
	types := def.SubscribeTypes
	if len(types) == 0 {
		types = []string{""}
	}
	out := make([]map[string]string, 0, len(types))
	for _, st := range types {
		body := map[string]string{}
		if st != "" {
			body["subscription_type"] = st
		}
		out = append(out, body)
	}
	return out
}

func subscriptionBodyLabel(body map[string]string) string {
	if v := body["event_type"]; v != "" {
		return "event_type=" + v
	}
	if v := body["subscription_type"]; v != "" {
		return "subscription_type=" + v
	}
	return ""
}

// registerSubscriptions 对 subscriptionRequestBodies 逐个 POST def.SubscribePath。
// 需要 User Access Token；任一请求失败即报错（已注册的类型服务端幂等处理）。
// fail-closed：HTTP 非 2xx、响应体不可解析都视为注册失败——订阅没建立时连上 WS 也收不到
// 事件，静默继续只会制造"看似在跑却永远无事件"的假象。
func (r *Runtime) registerSubscriptions(ctx context.Context, def KeyDefinition) error {
	if r.opts.UserAccessToken == "" {
		return fmt.Errorf("EventKey %s 需要以 User 身份注册服务端订阅，请先 `feishu-cli auth login`（scope: %s）",
			def.Key, strings.Join(def.Scopes, " "))
	}
	for _, body := range subscriptionRequestBodies(def) {
		label := subscriptionBodyLabel(body)
		if err := r.postSubscription(ctx, def.SubscribePath, body); err != nil {
			return fmt.Errorf("注册订阅（%s %s）失败: %w", def.SubscribePath, label, err)
		}
		fmt.Fprintf(r.opts.ErrOut, "[event] 已注册服务端订阅: %s %s\n", def.Key, label)
	}
	return nil
}

// unregisterSubscriptions 进程退出时 best-effort 注销 VC 等会话级订阅。
func (r *Runtime) unregisterSubscriptions(def KeyDefinition) {
	if def.UnsubscribePath == "" || r.opts.UserAccessToken == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), unsubscribeHTTPTimeout)
	defer cancel()
	for _, body := range subscriptionRequestBodies(def) {
		if err := r.postSubscription(ctx, def.UnsubscribePath, body); err != nil {
			fmt.Fprintf(r.opts.ErrOut, "[event] 注销订阅失败（可忽略，下次 subscribe 幂等覆盖）: %s %v\n", def.Key, err)
		}
	}
}

func (r *Runtime) postSubscription(ctx context.Context, path string, body map[string]string) error {
	timeout := subscribeHTTPTimeout
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl); rem > 0 && rem < timeout {
			timeout = rem
		}
	}
	// 带 Bearer 的请求必须走 config.NewHTTPClient（重定向校验 host + 剥离 Authorization）
	httpClient := config.NewHTTPClient(timeout)
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.opts.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("构造订阅请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.opts.UserAccessToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d, body: %s", resp.StatusCode, truncateForErr(respBody))
	}
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("响应不可解析（%v），body: %s", err, truncateForErr(respBody))
	}
	if apiResp.Code != 0 {
		return fmt.Errorf("code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return nil
}

// truncateForErr 把响应体截断到 512 字节内用于错误信息，避免超长 HTML 刷屏。
func truncateForErr(b []byte) string {
	const maxLen = 512
	if len(b) > maxLen {
		return string(b[:maxLen]) + "...(截断)"
	}
	return string(b)
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
		UUID   string `json:"uuid"` // schema 1.0 事件的唯一 ID
		Header struct {
			EventID   string `json:"event_id"`
			EventType string `json:"event_type"`
		} `json:"header"`
	}
	_ = json.Unmarshal(body, &meta)

	// 按 event_id 去重：服务端在未及时 ACK、断线重连时会重投同一事件（对齐官方 dedup）。
	eventID := meta.Header.EventID
	if eventID == "" {
		eventID = meta.UUID
	}
	if r.dedup.isDuplicate(eventID) {
		fmt.Fprintf(r.opts.ErrOut, "[event] 跳过重复投递的事件 event_id=%s\n", eventID)
		return nil
	}

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

// ValidateDotPathExpr 校验 consume --jq 的极简表达式。
// 当前只支持空值、"." 或 ".a.b.c" 这种 map 点路径，复杂过滤必须交给外部 jq。
func ValidateDotPathExpr(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" || expr == "." {
		return nil
	}
	if !strings.HasPrefix(expr, ".") {
		return fmt.Errorf("--jq 仅支持 .a.b.c 点路径表达式，复杂过滤请用管道接外部 jq")
	}
	segments := strings.Split(strings.TrimPrefix(expr, "."), ".")
	for _, seg := range segments {
		if seg == "" {
			return fmt.Errorf("--jq 仅支持 .a.b.c 点路径表达式，路径段不能为空")
		}
		for _, r := range seg {
			switch {
			case r >= 'A' && r <= 'Z':
			case r >= 'a' && r <= 'z':
			case r >= '0' && r <= '9':
			case r == '_' || r == '-':
			default:
				return fmt.Errorf("--jq 仅支持 .a.b.c 点路径表达式，不支持过滤器、管道或数组下标")
			}
		}
	}
	return nil
}

// ValidateOutputDir 校验 consume --output-dir 的路径边界。
// 为避免把事件写出项目/工作目录，只允许安全相对路径，不展开 ~，不接受绝对路径或 ..。
func ValidateOutputDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	if strings.HasPrefix(dir, "~") {
		return fmt.Errorf("--output-dir 不支持 ~ 展开，请用安全相对路径如 ./events")
	}
	if filepath.IsAbs(dir) || filepath.VolumeName(dir) != "" {
		return fmt.Errorf("--output-dir 只支持安全相对路径，不支持绝对路径")
	}
	for _, seg := range strings.FieldsFunc(dir, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if seg == ".." {
			return fmt.Errorf("--output-dir 不能包含 .. 路径段")
		}
	}
	// 相对路径仍可能落进敏感目录（如 cwd 为家目录时的 .ssh），按 safefile 拒绝名单再校验
	return safefile.ValidateOutputPath(dir)
}

// applyDotPath 实现极简 jq：仅支持 `.a.b.c` 形式（不支持过滤器/管道/数组下标）。
// 返回 (子树 JSON, 是否命中)。
func applyDotPath(data []byte, expr string) ([]byte, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "." || expr == "" {
		return data, true
	}
	if err := ValidateDotPathExpr(expr); err != nil {
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
