package cmd

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/event"
	"github.com/spf13/cobra"
)

var eventStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看本机 consume 进程状态",
	Long: `查看当前活跃的 consume 进程列表（按当前配置的 AppID 过滤）。

输出每个 consumer 进程的：
  - PID
  - EventKey
  - 启动时间 / 运行时长
  - max-events / timeout 限制（若配置）
  - output-dir / jq 配置（若配置）

实现说明:
  状态来源：~/.feishu-cli/events/<app_id>/bus.json（每个 consume 启动时写入，退出时移除）。
  status 查询会主动剔除 bus.json 中已不存活的 PID 条目（kill -9 / 崩溃残留）。

示例:
  feishu-cli event status
  feishu-cli event status --json | jq '.consumers[] | .pid'`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		cfg := config.Get()

		bus, err := event.NewBus(cfg.AppID)
		if err != nil {
			return err
		}
		snap, err := bus.Snapshot()
		if err != nil {
			return fmt.Errorf("读取 bus.json 失败: %w", err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(snap)
		}

		fmt.Printf("App ID: %s\n", snap.AppID)
		fmt.Printf("State file: %s\n", bus.StateFile())
		fmt.Println()

		if len(snap.Consumers) == 0 {
			fmt.Println("当前无活跃 consume 进程。")
			return nil
		}

		// 按 EventKey 排序，方便扫描
		consumers := append([]event.ConsumerEntry(nil), snap.Consumers...)
		sort.Slice(consumers, func(i, j int) bool {
			if consumers[i].EventKey != consumers[j].EventKey {
				return consumers[i].EventKey < consumers[j].EventKey
			}
			return consumers[i].PID < consumers[j].PID
		})

		fmt.Printf("%-7s  %-40s  %-12s  %s\n", "PID", "EVENT_KEY", "UPTIME", "EXTRA")
		for _, c := range consumers {
			uptime := time.Since(c.StartedAt).Round(time.Second).String()
			extra := []string{}
			if c.MaxEvents > 0 {
				extra = append(extra, fmt.Sprintf("max=%d", c.MaxEvents))
			}
			if c.TimeoutSec > 0 {
				extra = append(extra, fmt.Sprintf("timeout=%ds", c.TimeoutSec))
			}
			if c.OutputDir != "" {
				extra = append(extra, "output-dir="+c.OutputDir)
			}
			if c.JQExpr != "" {
				extra = append(extra, "jq="+c.JQExpr)
			}
			extraStr := "-"
			if len(extra) > 0 {
				extraStr = stringJoin(extra, " ")
			}
			fmt.Printf("%-7d  %-40s  %-12s  %s\n", c.PID, c.EventKey, uptime, extraStr)
		}

		fmt.Fprintln(os.Stderr, "\n用 `feishu-cli event stop --pid <pid>` 或 `--all` 停止 consume 进程。")
		return nil
	},
}

// stringJoin 内部 helper（避免引入 strings 包 alias 与现有 cmd 包 strings 用法冲突）
func stringJoin(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += sep + p
	}
	return out
}

func init() {
	eventCmd.AddCommand(eventStatusCmd)
	eventStatusCmd.Flags().Bool("json", false, "以 JSON 输出全部状态（含 bus.json 时间戳）")
}
