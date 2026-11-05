package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/event"
	"github.com/spf13/cobra"
)

var eventConsumeCmd = &cobra.Command{
	Use:   "consume <event_key> [event_key...]",
	Short: "订阅 EventKey 并把事件流写到 stdout",
	Long: `通过飞书 WebSocket 长连接订阅一个或多个 EventKey，每条事件作为一行 JSON（NDJSON）写到 stdout。

单连接、单实例（重要）:
  飞书会把同一 App 的事件随机投递给该 App 的任一条长连接。同一 App 开多个 consume 进程
  （哪怕订阅不同 EventKey）= 多条连接互相抢事件，每个进程只能收到一部分。因此：
  - 需要多个 EventKey 时在同一进程里一起订阅：event consume k1 k2（或 k1,k2），共用一条连接；
  - 同一 App 在本机只允许一个 consume 进程（单实例锁）；第二个进程会报错退出。
    --force 可跳过该检查（不安全：事件会在多个进程间随机拆分）；
  - 启动前会查询该 App 在飞书侧已有的长连接数，大于 0 时在 stderr 告警
    （可能是其他机器/服务在用同一 App，本进程只能收到部分事件）；
  - 确认本进程是该 App 唯一的长连接时，收到未订阅的已知事件类型会 ACK 后本地丢弃（不输出），
    避免服务端反复重投；存在其他连接时不代答，让服务端可重投给其他连接；
  - 同一 event_id 的重复投递（5 分钟内）只输出一次。
  每行事件可用 .header.event_type 区分类型。

启动协议:
  服务端订阅（如审批/VC）注册完成、且 WebSocket 握手就绪后，才会在 stderr 输出
  一行 [event] ready event_key=<key>（多个 key 时以逗号连接，如 event_key=k1,k2）。
  父进程应阻塞等待该行再读 stdout；握手完成前不会发 ready。

服务端订阅（pre-consume）:
  审批、VC、妙记、云文档评论、任务、画板、邮箱等 EventKey 在连接前需先 POST 订阅接口，
  否则连上也收不到事件。身份由 --as 决定（默认 auto：User 优先；所选 key 都支持 bot 且未登录时用 Bot），
  必须落在 key 的 auth_types 内（event schema <key> 查看），否则 exit 2：
  - 审批 v4 / VC / 妙记 / 邮箱：仅 user；云文档评论 / 任务 / 画板：user 或 bot
  - 退出时注销：VC、妙记、画板、邮箱（会话级）；审批、云文档评论、任务为持久关系，不注销
  --param key=value 传 key 级参数（可重复）：
  - board.whiteboard.updated_v1 必填 whiteboard_id，且只输出该画板的事件
  - mail.user_mailbox.event.message_received_v1 可选 mailbox（默认 me）
  - 审批 v4 可选 subscription_type=INVOLVED_APPROVAL|MANAGED_APPROVAL（逗号分隔多值；不传两种都注册）
  --dry-run 只打印订阅计划（身份、订阅请求、需在后台勾选的事件）后 exit 0，不读凭据、不联网。

退出条件（whichever 先触发）:
  - 接收 N 条事件后退出：--max-events N
  - 运行 D 时长后退出：--timeout 30s / 5m
  - Ctrl-C / SIGTERM
  - stdin EOF（非 TTY 且未设 --max-events/--timeout 时）—— 适配子进程场景：父进程关闭 stdin 即触发优雅退出。
    设置了 --max-events 或 --timeout 的有界运行会忽略 stdin EOF（如 true | event consume ... --timeout 30s
    会跑满 30s），避免 stdin 被立即关闭的管道/后台调用提前结束

简单 jq 支持:
  --jq 仅支持 . 点路径访问，例如 .event.message 提取消息子树。
  完整 jq 语法请通过 pipe 外部 jq 处理：feishu-cli event consume <key> | jq '.event'

文件输出:
  --output-dir 非空时，每条事件额外 dump 为 <event_id>.json 落盘。
  路径必须是安全相对路径；不做 ~ 展开，不接受绝对路径或 ..。

重连:
  oapi-sdk-go ws.Client 默认 WithAutoReconnect(true)，断线后无限重试（间隔 2 分钟 + 首次抖动）。
  长时间断线建议结合 --timeout 主动退出，由父进程拉起。

权限要求:
  长连接本身用 App 凭证；具体 scope 见 event schema <key>。请在飞书开放平台:
  1. 开启「事件订阅 - 长连接接收事件」
  2. 在「事件与回调 - 事件订阅」选中目标 EventType 并发布版本

示例:
  # 基础订阅（Ctrl-C 退出）
  feishu-cli event consume im.message.receive_v1

  # 同一进程一起订阅消息与表情回复（共用一条连接）
  feishu-cli event consume im.message.receive_v1 im.message.reaction.created_v1

  # 调试模式：抓 5 条事件，最多跑 60s
  feishu-cli event consume im.message.receive_v1 --max-events 5 --timeout 60s

  # 静默模式 + 落盘
  feishu-cli event consume im.message.receive_v1 --output-dir ./events --quiet

  # 配合 jq 实时过滤群消息
  feishu-cli event consume im.message.receive_v1 | jq 'select(.event.message.chat_type=="group")'

  # 预览审批订阅计划（不联网）；只注册 INVOLVED_APPROVAL
  feishu-cli event consume approval.instance.status_changed_v4 --dry-run
  feishu-cli event consume approval.instance.status_changed_v4 --param subscription_type=INVOLVED_APPROVAL

  # 以 Bot 身份监听云文档评论 @；监听某个画板的变更
  feishu-cli event consume drive.notice.comment_add_v1 --as bot
  feishu-cli event consume board.whiteboard.updated_v1 --param whiteboard_id=<whiteboard_id>`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		keys := event.NormalizeEventKeys(args)
		if len(keys) == 0 {
			return clierr.Usagef("至少指定一个 EventKey（运行 `feishu-cli event list` 查看支持的 key）")
		}
		defs := make([]event.KeyDefinition, 0, len(keys))
		for _, key := range keys {
			def, ok := event.Lookup(key)
			if !ok {
				return clierr.Usagef("未知 EventKey: %q（运行 `feishu-cli event list` 查看支持的 key）", key)
			}
			defs = append(defs, def)
		}

		maxEvents, _ := cmd.Flags().GetInt("max-events")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		jqExpr, _ := cmd.Flags().GetString("jq")
		outputDir, _ := cmd.Flags().GetString("output-dir")
		quiet, _ := cmd.Flags().GetBool("quiet")
		force, _ := cmd.Flags().GetBool("force")

		if err := event.ValidateDotPathExpr(jqExpr); err != nil {
			return clierr.Usage(err)
		}
		if err := event.ValidateOutputDir(outputDir); err != nil {
			return clierr.Usage(err)
		}

		rawParams, _ := cmd.Flags().GetStringArray("param")
		params, err := event.ParseParams(defs, rawParams)
		if err != nil {
			return clierr.Usage(err)
		}
		as, _ := cmd.Flags().GetString("as")
		as = strings.ToLower(strings.TrimSpace(as))
		switch as {
		case "", "auto":
			as = "auto"
		case "user":
		case "bot", "tenant", "app":
			as = "bot"
		default:
			return clierr.Usagef("--as 仅支持 user|bot|auto，得到 %q", as)
		}
		if as != "auto" {
			if err := event.CheckIdentity(defs, as); err != nil {
				return clierr.Usage(err)
			}
		}
		plan := make([]event.SubscriptionCall, 0)
		for _, def := range defs {
			calls, planErr := event.SubscriptionPlan(def, params)
			if planErr != nil {
				return clierr.Usage(planErr)
			}
			plan = append(plan, calls...)
		}

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printEventConsumeDryRun(cmd, defs, params, as, plan)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		cfg := config.Get()

		bus, err := event.NewBus(cfg.AppID)
		if err != nil {
			return fmt.Errorf("初始化事件状态文件失败: %w", err)
		}

		var errOut io.Writer = os.Stderr
		if quiet {
			errOut = io.Discard
		}

		// 单实例锁：同一 App 同一机器只允许一个 consume 进程（多进程 = 多条连接互相抢事件）。
		if force {
			fmt.Fprintln(os.Stderr, "[event] 警告: --force 跳过单实例检查；同一 App 的多个 consume 进程会随机拆分事件，每个进程只能收到一部分")
		} else {
			lock, lockErr := event.AcquireConsumeLock(cfg.AppID)
			if lockErr != nil {
				if errors.Is(lockErr, event.ErrInstanceLockHeld) {
					return describeConsumeLockConflict(bus, cfg.AppID)
				}
				return fmt.Errorf("获取 event consume 单实例锁失败: %w", lockErr)
			}
			defer lock.Release()
		}

		// 远端预检：飞书侧已有长连接（其他机器/服务）时，事件会被随机分走一部分。best-effort，不阻断启动。
		// 只有确认本进程将是唯一连接时，才对未订阅的事件类型 ACK 后丢弃；否则不代答，
		// 让服务端可以把这些事件重投给真正处理它们的连接。
		ackUnsubscribed := false
		if n, checkErr := countRemoteEventConnections(); checkErr != nil {
			fmt.Fprintf(errOut, "[event] 远端长连接检查失败（忽略，继续启动；未订阅的事件类型不做代答）: %v\n", checkErr)
		} else if n > 0 {
			fmt.Fprintf(os.Stderr, "[event] 警告: 该 App 在飞书侧已有 %d 条事件长连接（可能是其他机器/服务在用同一 App）；"+
				"飞书会把事件随机分发到各连接，本进程只能收到其中一部分。如需完整事件流，请停掉其他连接或换用独立 App/profile\n", n)
		} else {
			ackUnsubscribed = true
		}

		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://open.feishu.cn"
		}

		subscribeIdentity, userToken, tenantToken, err := resolveEventSubscribeIdentity(cmd, defs, as)
		if err != nil {
			return err
		}

		runtime := event.NewRuntime(event.ConsumeOptions{
			AppID:             cfg.AppID,
			AppSecret:         cfg.AppSecret,
			EventKey:          keys[0],
			EventKeys:         keys,
			BaseURL:           baseURL,
			Out:               os.Stdout,
			ErrOut:            errOut,
			JQExpr:            jqExpr,
			OutputDir:         outputDir,
			MaxEvents:         maxEvents,
			Timeout:           timeout,
			UserAccessToken:   userToken,
			SubscribeAsBot:    subscribeIdentity == "bot",
			TenantAccessToken: tenantToken,
			Params:            params,
			Bus:               bus,
			AckUnsubscribed:   ackUnsubscribed,
		})

		// 信号 + stdin EOF 处理
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		go func() {
			select {
			case sig := <-sigCh:
				fmt.Fprintf(errOut, "[event] 收到 %s，正在关闭...\n", sig)
				cancel()
			case <-ctx.Done():
			}
		}()

		// 非 TTY 且无界运行时把 stdin EOF 当作 shutdown 信号（适配 AI Agent 子进程场景）。
		// 设了 --max-events / --timeout 的有界运行忽略 EOF（对齐官方 #1285）：
		// `true | event consume ... --timeout 30s` 这类调用 stdin 立即关闭，不能因此提前退出。
		if shouldWatchStdinEOF(isTerminal(os.Stdin), maxEvents, timeout) {
			go func() {
				_, _ = io.Copy(io.Discard, os.Stdin)
				fmt.Fprintln(errOut, "[event] stdin 关闭，正在退出（无界运行把 stdin EOF 视为退出信号；"+
					"如需在关闭 stdin 后继续运行，请设置 --timeout/--max-events，或保持 stdin 打开）...")
				cancel()
			}()
		}

		start := time.Now()
		reason, runErr := runtime.Run(ctx)
		elapsed := time.Since(start)

		fmt.Fprintf(errOut, "[event] exited — elapsed=%s reason=%s\n", elapsed.Round(time.Millisecond), reason)
		return runErr
	},
}

// resolveEventSubscribeIdentity 决定服务端订阅注册所用身份与 token。
//
//   - --as 未传 / auto：与旧版一致先按 User 优先解析（只含 user-only 订阅 key 时沿用
//     resolveOptionalUserTokenWithFallback，缺失时由 runtime 报「需要 auth login」）；
//     所选订阅 key 都支持 bot 且未配置 User 时改用 App 身份；已配置 User 但不可用时 fail-closed，
//     不静默切 Bot（只有可用 bot 的新 key 走这条）；
//   - --as user：必须 User Token（缺失 exit 3）；
//   - --as bot：用 App Token 注册（CheckIdentity 已保证所有 key 支持 bot）。
//
// 不含订阅 key 时仍按旧版 best-effort 解析 User Token（不影响 WebSocket）。
func resolveEventSubscribeIdentity(cmd *cobra.Command, defs []event.KeyDefinition, as string) (identity, userToken, tenantToken string, err error) {
	needsSub := false
	for _, def := range defs {
		if def.HasSubscription() {
			needsSub = true
			break
		}
	}
	switch as {
	case "user":
		userToken, err = requireUserToken(cmd, "event consume --as user")
		return "user", userToken, "", err
	case "bot":
		if needsSub {
			if tenantToken, err = fetchTenantAccessToken(); err != nil {
				return "", "", "", fmt.Errorf("获取 App Token 失败（--as bot 注册服务端订阅需要）: %w", err)
			}
		}
		return "bot", "", tenantToken, nil
	}
	if !event.AnySupportsBot(defs) {
		return "user", resolveOptionalUserTokenWithFallback(cmd), "", nil
	}
	userToken, err = resolveAutoUserToken(cmd)
	if err != nil {
		return "", "", "", err
	}
	if event.ResolveAutoIdentity(defs, userToken != "") == "bot" {
		if tenantToken, err = fetchTenantAccessToken(); err != nil {
			return "", "", "", fmt.Errorf("获取 App Token 失败（未登录 User，改用 Bot 身份注册服务端订阅）: %w", err)
		}
		return "bot", "", tenantToken, nil
	}
	return "user", userToken, "", nil
}

// printEventConsumeDryRun 静态预览：不读凭据、不联网、不加单实例锁，打印订阅计划后退出。
// auto 身份按本地是否配置 User Token 静态推断（不刷新 token）。
func printEventConsumeDryRun(cmd *cobra.Command, defs []event.KeyDefinition, params map[string]string, as string, plan []event.SubscriptionCall) error {
	identity := as
	if as == "auto" {
		flagToken, _ := cmd.Flags().GetString("user-access-token")
		hasUser := auth.HasUserTokenConfigured(flagToken, config.Get().UserAccessToken)
		identity = event.ResolveAutoIdentity(defs, hasUser)
		if identity == "" {
			identity = "无需（未涉及服务端订阅）"
		}
	}
	keys := make([]string, 0, len(defs))
	var consoleEvents, scopes []string
	for _, def := range defs {
		keys = append(keys, def.Key)
		consoleEvents = append(consoleEvents, def.RequiredConsoleEvents...)
		scopes = append(scopes, def.Scopes...)
	}
	out := map[string]any{
		"dry_run":                 true,
		"event_keys":              keys,
		"identity":                identity,
		"subscriptions":           plan,
		"required_console_events": consoleEvents,
		"scopes":                  scopes,
		"desc":                    "仅预览：未注册订阅、未建立长连接。subscriptions 为启动前要 POST 的服务端订阅（unsubscribe_path 非空的会在退出时注销）",
	}
	if len(params) > 0 {
		out["params"] = params
	}
	return printJSON(out)
}

// countRemoteEventConnections 远端长连接计数（可在测试中替换）。
var countRemoteEventConnections = client.CountEventConnections

// shouldWatchStdinEOF 仅在非 TTY 且无界（未设 --max-events / --timeout）时监听 stdin EOF。
func shouldWatchStdinEOF(stdinIsTerminal bool, maxEvents int, timeout time.Duration) bool {
	if stdinIsTerminal {
		return false
	}
	return maxEvents <= 0 && timeout <= 0
}

// describeConsumeLockConflict 单实例锁被占用时给出可执行的报错：列出正在运行的 consume 进程。
func describeConsumeLockConflict(bus *event.Bus, appID string) error {
	var holders []string
	if snap, err := bus.Snapshot(); err == nil {
		byPID := map[int][]string{}
		for _, c := range snap.Consumers {
			byPID[c.PID] = append(byPID[c.PID], c.EventKey)
		}
		pids := make([]int, 0, len(byPID))
		for pid := range byPID {
			pids = append(pids, pid)
		}
		sort.Ints(pids)
		for _, pid := range pids {
			holders = append(holders, fmt.Sprintf("PID %d（%s）", pid, strings.Join(byPID[pid], ",")))
		}
	}
	detail := ""
	if len(holders) > 0 {
		detail = "\n  正在运行: " + strings.Join(holders, "；")
	}
	return fmt.Errorf("App %s 在本机已有 event consume 进程在运行，同一 App 只允许一个消费进程"+
		"（飞书会把事件随机分给同 App 的多条长连接，多进程会互相抢事件）。%s\n"+
		"  解决：把需要的 EventKey 合并到一个进程订阅：feishu-cli event consume <key1> <key2>；\n"+
		"        或先停止已有进程：feishu-cli event status / feishu-cli event stop --all；\n"+
		"        确需并行（接受事件被随机拆分）时加 --force", appID, detail)
}

// isTerminal 判断 fd 是否连接到 tty；非 tty 时启用 stdin EOF 退出协议。
// 用 fstat 的 ModeCharDevice 位粗略判断（标准库 term/isatty 也是这思路）。
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func init() {
	eventCmd.AddCommand(eventConsumeCmd)
	eventConsumeCmd.Flags().Int("max-events", 0, "接收到 N 条事件后退出（0=不限制；有界运行忽略 stdin EOF）")
	eventConsumeCmd.Flags().Duration("timeout", 0, "运行 D 时长后退出（如 30s / 5m，0=不限制；有界运行忽略 stdin EOF）")
	eventConsumeCmd.Flags().String("jq", "", "极简点路径过滤，如 .event.message（不支持完整 jq 语法）")
	eventConsumeCmd.Flags().String("user-access-token", "", "User Access Token（审批/VC 等需服务端订阅注册的 EventKey 使用）")
	eventConsumeCmd.Flags().String("output-dir", "", "把每条事件 dump 为 <event_id>.json 到该目录（不影响 stdout）")
	eventConsumeCmd.Flags().Bool("quiet", false, "静默模式：抑制 stderr 诊断（不影响 stdout 事件流；ready marker 仍会输出，便于父进程判断就绪）")
	eventConsumeCmd.Flags().StringArrayP("param", "p", nil, "EventKey 参数 key=value（可重复），如 whiteboard_id=xxx、subscription_type=INVOLVED_APPROVAL；见 event schema <key>")
	eventConsumeCmd.Flags().String("as", "auto", "注册服务端订阅的身份: user | bot | auto（auto: User 优先；所选 key 都支持 bot 且未登录时用 Bot；必须与 key 的 auth_types 一致）")
	eventConsumeCmd.Flags().Bool("dry-run", false, "只预览订阅计划（身份、订阅请求、需勾选的事件）后退出，不读凭据、不联网")
	eventConsumeCmd.Flags().Bool("force", false, "跳过同一 App 单实例检查（不安全：多进程会随机拆分事件，每个进程只收到一部分）")
}
