package cmd

import (
	"github.com/spf13/cobra"
)

var wikiCmd = &cobra.Command{
	Use:   "wiki",
	Short: "知识库操作命令",
	Long: `知识库操作命令，包括创建、更新、删除、移动节点，获取节点信息、列出空间和节点、导出文档等。

子命令:
  create    创建知识库节点
  update    更新知识库节点标题
  delete    删除知识库节点
  move      移动知识库节点
  get       获取知识库节点信息
  spaces    列出知识空间
  nodes     列出空间下的节点
  export    导出知识库文档为 Markdown

知识库 URL 格式:
  https://xxx.feishu.cn/wiki/<node_token>
  https://xxx.larkoffice.com/wiki/<node_token>

节点类型（obj_type）:
  docx      新版文档
  doc       旧版文档
  sheet     电子表格
  bitable   多维表格
  mindnote  思维笔记
  file      文件
  slides    幻灯片

示例:
  # 创建节点
  feishu-cli wiki create --space-id <space_id> --title "新文档"

  # 更新节点标题
  feishu-cli wiki update <node_token> --title "新标题"

  # 删除节点
  feishu-cli wiki delete <node_token>

  # 移动节点
  feishu-cli wiki move <node_token> --target-space <space_id>

  # 获取节点信息
  feishu-cli wiki get <node_token>

  # 列出知识空间
  feishu-cli wiki spaces

  # 列出空间下的节点
  feishu-cli wiki nodes <space_id>

  # 导出为 Markdown（仅支持 docx 类型）
  feishu-cli wiki export <node_token> --output doc.md`,
}

func init() {
	rootCmd.AddCommand(wikiCmd)
}
