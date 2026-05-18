package cmd

import (
	"github.com/spf13/cobra"
)

var eventCmd = &cobra.Command{
	Use:   "event",
	Short: "WebSocket 实时事件订阅",
	Long: `WebSocket 实时事件订阅，通过飞书长连接接收应用事件并以 NDJSON 输出到 stdout。

子命令:
  list           列出所有支持的 EventKey（按 domain 分组）
  schema         查看某个 EventKey 的字段说明
  consume        订阅 EventKey，事件流写到 stdout（阻塞）
  status         查看本机所有 consume 进程（PID/EventKey/启动时间）
  stop           停止 consume 进程（按 PID 或 EventKey）

输出协议:
  - stdout      每条事件一行 JSON（NDJSON），适合 jq / 脚本管道
  - stderr      诊断信息；启动完成会有一行 [event] ready event_key=<key>，
                AI Agent 父进程应阻塞 stderr 等该行出现后再读 stdout

状态文件:
  ~/.feishu-cli/events/<app_id>/bus.json     当前活跃 consumer 列表（PID/EventKey/启动时间）
  ~/.feishu-cli/events/<app_id>/bus.lock     跨进程文件锁（flock）

权限要求:
  默认 App Token（不强制 user token）。具体 scope 见 event schema <key>。
  通用必备权限：在飞书开放平台开启「事件订阅 - 长连接接收事件」并选中目标事件。

退出码:
  0       正常退出（达到 --max-events / --timeout / SIGTERM / Ctrl-C）
  非 0    启动失败 / WebSocket 不可恢复错误 / 参数错误

示例:
  # 1. 列出所有支持的 EventKey
  feishu-cli event list

  # 2. 查看消息接收事件的 payload 字段
  feishu-cli event schema im.message.receive_v1

  # 3. 订阅消息接收（阻塞，Ctrl-C 退出）
  feishu-cli event consume im.message.receive_v1

  # 4. 只跑 60 秒采集前 5 条消息（适合调试）
  feishu-cli event consume im.message.receive_v1 --max-events 5 --timeout 60s

  # 5. 后台跑并发到多个 EventKey（每个 EventKey 一个进程）
  feishu-cli event consume im.message.receive_v1 > receive.ndjson 2> receive.log &
  feishu-cli event consume im.message.reaction.created_v1 > reaction.ndjson 2> reaction.log &
  feishu-cli event status                       # 查看当前活跃进程
  feishu-cli event stop --all                   # 停掉所有 consume 进程`,
}

func init() {
	rootCmd.AddCommand(eventCmd)
}
