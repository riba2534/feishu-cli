package cmd

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/event"
	"github.com/spf13/cobra"
)

var eventStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "停止 consume 进程",
	Long: `停止本机活跃的 consume 进程。可以按以下三种方式之一指定目标：

  --pid <N>          按 PID 精确停止
  --event-key <key>  停止订阅该 EventKey 的所有进程
  --all              停止所有 consume 进程（当前 AppID）

实现:
  默认 SIGTERM（优雅退出，consume 进程会自动 unregister bus.json）。
  --force 升级为 SIGKILL（不推荐，会留下 bus.json 僵尸条目，status 命令会自动清理）。

示例:
  feishu-cli event stop --pid 12345
  feishu-cli event stop --event-key im.message.receive_v1
  feishu-cli event stop --all
  feishu-cli event stop --all --force        # 紧急情况下硬杀`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		cfg := config.Get()

		pid, _ := cmd.Flags().GetInt("pid")
		eventKey, _ := cmd.Flags().GetString("event-key")
		all, _ := cmd.Flags().GetBool("all")
		force, _ := cmd.Flags().GetBool("force")
		asJSON, _ := cmd.Flags().GetBool("json")

		if !all && pid == 0 && eventKey == "" {
			return fmt.Errorf("必须指定 --pid / --event-key / --all 之一")
		}

		bus, err := event.NewBus(cfg.AppID)
		if err != nil {
			return err
		}
		snap, err := bus.Snapshot()
		if err != nil {
			return err
		}

		// 决定要停的 consumer 列表
		var targets []event.ConsumerEntry
		for _, c := range snap.Consumers {
			if all {
				targets = append(targets, c)
				continue
			}
			if pid != 0 && c.PID == pid {
				targets = append(targets, c)
				continue
			}
			if eventKey != "" && c.EventKey == eventKey {
				targets = append(targets, c)
			}
		}

		if len(targets) == 0 {
			if asJSON {
				return printJSON(map[string]any{"stopped": []any{}, "message": "未找到匹配的 consume 进程"})
			}
			fmt.Println("未找到匹配的 consume 进程。")
			return nil
		}

		sig := syscall.SIGTERM
		if force {
			sig = syscall.SIGKILL
		}

		results := make([]map[string]any, 0, len(targets))
		for _, t := range targets {
			r := map[string]any{
				"pid":       t.PID,
				"event_key": t.EventKey,
			}
			err := syscall.Kill(t.PID, sig)
			if err != nil {
				r["status"] = "error"
				r["reason"] = err.Error()
			} else {
				r["status"] = "signaled"
				r["signal"] = sig.String()
			}
			results = append(results, r)
		}

		// 等最多 3 秒，验证进程已退出
		if !force {
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				snap, _ = bus.Snapshot()
				stillAlive := 0
				for _, c := range snap.Consumers {
					for _, t := range targets {
						if c.PID == t.PID {
							stillAlive++
						}
					}
				}
				if stillAlive == 0 {
					break
				}
				time.Sleep(200 * time.Millisecond)
			}
		}

		if asJSON {
			return printJSON(map[string]any{"stopped": results})
		}

		fmt.Printf("已发送 %s 给 %d 个 consume 进程:\n", sig.String(), len(targets))
		for _, r := range results {
			status := r["status"].(string)
			line := fmt.Sprintf("  pid=%v event_key=%v status=%s", r["pid"], r["event_key"], status)
			if reason, ok := r["reason"]; ok {
				line += fmt.Sprintf(" reason=%v", reason)
			}
			fmt.Println(line)
		}

		if force {
			fmt.Fprintln(os.Stderr, "\n提示: --force 不会触发 consume 进程的 bus.json 清理；status 命令会自动剔除僵尸条目。")
		}
		return nil
	},
}

func init() {
	eventCmd.AddCommand(eventStopCmd)
	eventStopCmd.Flags().Int("pid", 0, "按 PID 停止")
	eventStopCmd.Flags().String("event-key", "", "按 EventKey 停止（停掉所有订阅该 key 的进程）")
	eventStopCmd.Flags().Bool("all", false, "停止当前 AppID 下所有 consume 进程")
	eventStopCmd.Flags().Bool("force", false, "用 SIGKILL 而非 SIGTERM（紧急情况）")
	eventStopCmd.Flags().Bool("json", false, "以 JSON 输出停止结果")
}
