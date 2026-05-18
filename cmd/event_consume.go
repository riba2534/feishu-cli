package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/event"
	"github.com/spf13/cobra"
)

var eventConsumeCmd = &cobra.Command{
	Use:   "consume <event_key>",
	Short: "订阅 EventKey 并把事件流写到 stdout",
	Long: `通过飞书 WebSocket 长连接订阅指定 EventKey，每条事件作为一行 JSON（NDJSON）写到 stdout。

启动协议:
  本命令启动后会先在 stderr 输出一行 [event] ready event_key=<key>。
  AI Agent / subprocess 父进程应阻塞等待该 ready marker，再开始读 stdout。

退出条件（whichever 先触发）:
  - 接收 N 条事件后退出：--max-events N
  - 运行 D 时长后退出：--timeout 30s / 5m
  - Ctrl-C / SIGTERM
  - stdin EOF（非 TTY 模式）—— 适配子进程场景：父进程关闭 stdin 即触发优雅退出

简单 jq 支持:
  --jq 仅支持 . 点路径访问，例如 .event.message 提取消息子树。
  完整 jq 语法请通过 pipe 外部 jq 处理：feishu-cli event consume <key> | jq '.event'

文件输出:
  --output-dir 非空时，每条事件额外 dump 为 <event_id>.json 落盘。
  路径必须是相对路径或已存在的绝对路径；不做 ~ 展开。

重连:
  oapi-sdk-go ws.Client 默认 WithAutoReconnect(true)，断线后无限重试（间隔 2 分钟 + 首次抖动）。
  长时间断线建议结合 --timeout 主动退出，由父进程拉起。

权限要求:
  默认 App Token；具体 scope 见 event schema <key>。请在飞书开放平台:
  1. 开启「事件订阅 - 长连接接收事件」
  2. 在「事件与回调 - 事件订阅」选中目标 EventType 并发布版本

示例:
  # 基础订阅（Ctrl-C 退出）
  feishu-cli event consume im.message.receive_v1

  # 调试模式：抓 5 条事件，最多跑 60s
  feishu-cli event consume im.message.receive_v1 --max-events 5 --timeout 60s

  # 静默模式 + 落盘
  feishu-cli event consume im.message.receive_v1 --output-dir ./events --quiet

  # 配合 jq 实时过滤群消息
  feishu-cli event consume im.message.receive_v1 | jq 'select(.event.message.chat_type=="group")'`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		cfg := config.Get()

		key := args[0]
		def, ok := event.Lookup(key)
		if !ok {
			return fmt.Errorf("未知 EventKey: %q（运行 `feishu-cli event list` 查看支持的 key）", key)
		}

		maxEvents, _ := cmd.Flags().GetInt("max-events")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		jqExpr, _ := cmd.Flags().GetString("jq")
		outputDir, _ := cmd.Flags().GetString("output-dir")
		quiet, _ := cmd.Flags().GetBool("quiet")

		if outputDir != "" && strings.HasPrefix(outputDir, "~") {
			return fmt.Errorf("--output-dir 不支持 ~ 展开，请用相对路径如 ./events")
		}

		bus, err := event.NewBus(cfg.AppID)
		if err != nil {
			return fmt.Errorf("初始化事件状态文件失败: %w", err)
		}

		var errOut io.Writer = os.Stderr
		if quiet {
			errOut = io.Discard
		}

		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://open.feishu.cn"
		}

		runtime := event.NewRuntime(event.ConsumeOptions{
			AppID:     cfg.AppID,
			AppSecret: cfg.AppSecret,
			EventKey:  def.Key,
			BaseURL:   baseURL,
			Out:       os.Stdout,
			ErrOut:    errOut,
			JQExpr:    jqExpr,
			OutputDir: outputDir,
			MaxEvents: maxEvents,
			Timeout:   timeout,
			Bus:       bus,
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

		// 非 TTY 时把 stdin EOF 当作 shutdown 信号（适配 AI Agent 子进程场景）
		if !isTerminal(os.Stdin) {
			go func() {
				_, _ = io.Copy(io.Discard, os.Stdin)
				fmt.Fprintln(errOut, "[event] stdin 关闭，正在退出...")
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
	eventConsumeCmd.Flags().Int("max-events", 0, "接收到 N 条事件后退出（0=不限制）")
	eventConsumeCmd.Flags().Duration("timeout", 0, "运行 D 时长后退出（如 30s / 5m，0=不限制）")
	eventConsumeCmd.Flags().String("jq", "", "极简点路径过滤，如 .event.message（不支持完整 jq 语法）")
	eventConsumeCmd.Flags().String("output-dir", "", "把每条事件 dump 为 <event_id>.json 到该目录（不影响 stdout）")
	eventConsumeCmd.Flags().Bool("quiet", false, "静默模式：抑制 stderr 诊断（不影响 stdout 事件流；AI Agent 慎用——会一起抑制 ready marker）")
}
