package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile   string
	debug     bool
	version   = "dev"
	buildTime = "unknown"
)

// SetVersionInfo sets version information from main package
func SetVersionInfo(v, bt string) {
	version = v
	buildTime = bt
	rootCmd.Version = fmt.Sprintf("%s (built %s)", version, buildTime)
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "feishu-cli",
	Short: "飞书开放平台命令行工具",
	Long: `飞书开放平台命令行工具，支持文档操作、Markdown 双向转换、消息发送、权限管理、审批查询、日历管理、搜索等功能。

命令模块:
  auth      OAuth 认证（Device Flow 登录、scope 预检、token 状态）
  doc       文档操作（创建、获取、编辑、导入导出、添加高亮块/画板）
  wiki      知识库操作（获取节点、列出空间、导出文档）
  file      云空间文件管理（列出、创建、移动、复制、删除）
  user      用户操作（获取用户信息）
  dept      部门操作（部门信息、成员列表）
  board     画板操作（下载图片、导入图表、创建节点）
  media     素材操作（上传、下载）
  comment   评论操作（列出、添加、删除评论）
  perm      权限操作（添加、更新权限）
  msg       消息操作（发送消息、搜索群聊、会话历史）
  bitable   多维表格操作（数据表、字段、记录、视图管理）
  task      任务操作（创建、查看、更新、完成）
  tasklist  任务清单操作（创建/获取/列表、成员管理）
  approval  审批操作（定义详情、当前登录用户任务查询）
  attendance  考勤操作（打卡记录查询、统计数据查询；tenant token）
  calendar  日历操作（日历、日程管理）
  vc        视频会议（多维搜索、纪要/AI 产物/逐字稿、录制查询；User Token）
  minutes   妙记（基础信息、AI 产物、媒体下载；User Token）
  mail      邮箱（发送/回复/转发/草稿/查询；User Token）
  drive     云盘增强（分块上传、有界轮询导出/导入、富文本评论、通用 task-result）
  markdown  Drive 原生 Markdown 文件 CRUD（.md 整体读写，不转换飞书 docx 块）
  search    搜索操作（消息、应用搜索，需要用户授权）
  event     实时事件订阅（WebSocket 长连接 + daemon 进程模型；list/consume/schema/status/stop）
  slides    Slides 演示文稿（创建 + 媒体上传）
  okr       OKR 操作（周期列表、进展记录列表与创建）
  schema    本地浏览飞书 OpenAPI 方法（纯本地查询，不需 token；service.resource.method 路径）
  sheet     电子表格（基础读写 + filter-view 创建/列表/删除 + dropdown 数据验证）
  chat      群聊管理（拉人/踢人/改名/成员列表；reaction/pin 等互动）
  profile   多 App 配置切换（add/list/use/current/rename/remove/migrate）
  doctor    环境健康检查（6 项：config / user_token / endpoints / proxy / deps）
  config    配置管理（初始化配置）

注意：bitable 命令已切换到 base/v3 API，flag 使用 --base-token。

配置方式:
  1. 环境变量（推荐）:
     export FEISHU_APP_ID="cli_xxx"
     export FEISHU_APP_SECRET="xxx"

  2. 配置文件:
     ~/.feishu-cli/config.yaml

  配置优先级: 环境变量 > 配置文件 > 默认值

快速开始:
  # 创建文档
  feishu-cli doc create --title "我的文档"

  # 导出为 Markdown
  feishu-cli doc export <document_id> --output doc.md

  # 从 Markdown 创建文档
  feishu-cli doc import doc.md --title "导入的文档"

  # 发送消息
  feishu-cli msg send --receive-id-type email --receive-id user@example.com --text "你好"

  # 查询当前登录用户的审批待办（需先 auth login）
  feishu-cli approval task query --topic todo

更多信息请访问: https://github.com/riba2534/feishu-cli`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// --config 仅影响 config.yaml 加载，token 仍按 profile 解析；显式提醒避免错配
		if cfgFile != "" {
			fmt.Fprintln(os.Stderr, "⚠️  使用 --config 时，token 仍读自当前 profile（~/.feishu-cli/[profiles/<active>/]token.json）。")
			fmt.Fprintln(os.Stderr, "    如需完整隔离，请改用 FEISHU_PROFILE=<name>。")
		}
		// Skip config initialization for group commands (those with subcommands but no own RunE)
		// and utility commands that don't need config
		if cmd.HasSubCommands() && cmd.RunE == nil && cmd.Run == nil {
			return nil
		}
		switch cmd.Name() {
		case "init", "help", "completion", "version", "doctor", "schema":
			return nil
		}
		// auth status/logout / schema list / profile 全子命令 不需要配置（纯本地操作或配置自身）
		if cmd.Parent() != nil {
			switch cmd.Parent().Name() {
			case "schema", "profile":
				return nil
			case "auth":
				if cmd.Name() == "status" || cmd.Name() == "logout" {
					return nil
				}
			}
		}

		if err := config.Init(cfgFile); err != nil {
			return err
		}

		// Override debug from flag
		if debug {
			cfg := config.Get()
			cfg.Debug = true
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if msg := err.Error(); msg != "" {
			fmt.Fprintln(os.Stderr, msg)
		}
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径（默认: ~/.feishu-cli/config.yaml）")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "启用调试模式")
	// R1 review fix: RunE 返回 error 时不再打印整页 usage 淹没真错误（11/13 PR 未单独设此 flag → root 统一处理）
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
}
