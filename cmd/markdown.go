package cmd

import (
	"github.com/spf13/cobra"
)

// markdownCmd 是 Drive 原生 Markdown 文件 (.md) 的 CRUD 入口。
// 注意：与 `doc import` / `doc export` 不同，本模块不做 Markdown ↔ 飞书文档块的转换，
// 而是把 .md 当作一个普通的 Drive 文件（file_type=file）整体读写——
// 保留原始 Markdown 格式，适合 AI agent 自动化场景。
var markdownCmd = &cobra.Command{
	Use:   "markdown",
	Short: "Drive 原生 Markdown 文件（.md）的 CRUD",
	Long: `Drive 原生 Markdown 文件（.md）的 CRUD。

与 ` + "`doc import` / `doc export`" + ` 的区别：

  doc import/export   — 把 Markdown 转换为飞书 docx 块（标题/列表/表格/Callout 等），创建的是 docx 类型文档
  markdown create/...  — 把 .md 当作普通 Drive 文件整体读写，不做转换，保留原始 Markdown 格式

适合场景：
  - AI agent 把生成的 Markdown 文档存为 .md 文件，下次读回时仍是原汁原味的 Markdown
  - 团队协作时希望保留 Markdown 源码（而不是飞书 docx 渲染）

子命令:
  create     创建 .md 文件（从字符串或本地文件）
  fetch      读取 .md 文件内容
  overwrite  覆盖现有 .md 文件

权限要求:
  - User Access Token（推荐，避免 owner 是 bot）
  - drive:drive 或 drive:file:upload + drive:file.content:read`,
}

func init() {
	rootCmd.AddCommand(markdownCmd)
}
