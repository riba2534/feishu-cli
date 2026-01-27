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
	Long: `飞书开放平台命令行工具，支持文档操作、Markdown 双向转换、消息发送、权限管理、日历管理、搜索等功能。

命令模块:
  doc       文档操作（创建、获取、编辑、导入导出、添加高亮块/画板）
  wiki      知识库操作（获取节点、列出空间、导出文档）
  file      云空间文件管理（列出、创建、移动、复制、删除）
  user      用户操作（获取用户信息）
  board     画板操作（下载图片、导入图表、创建节点）
  media     素材操作（上传、下载）
  comment   评论操作（列出、添加、删除评论）
  perm      权限操作（添加、更新权限）
  msg       消息操作（发送消息、搜索群聊、会话历史）
  task      任务操作（创建、查看、更新、完成）
  calendar  日历操作（日历、日程管理）
  search    搜索操作（消息、应用搜索，需要用户授权）
  config    配置管理（初始化配置）

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

更多信息请访问: https://github.com/riba2534/feishu-cli`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config initialization for commands that don't need it
		switch cmd.Name() {
		case "init", "help", "completion", "version", "doc", "wiki", "file", "media", "comment", "perm", "msg", "config", "calendar", "task", "search", "user", "board":
			return nil
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
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径（默认: ~/.feishu-cli/config.yaml）")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "启用调试模式")
}
