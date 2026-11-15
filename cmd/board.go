package cmd

import (
	"github.com/spf13/cobra"
)

var boardCmd = &cobra.Command{
	Use:   "board",
	Short: "画板操作命令",
	Long: `画板操作命令，用于操作飞书画板（白板）。

子命令:
  image         下载画板图片
  import        导入图表到画板
  nodes         获取画板节点列表
  create-notes  创建画板节点
  update        更新画板内容（支持覆盖）
  delete        删除画板节点

写命令（import/update/clone/svg-import/upload-image/delete/create-notes）默认 Bot 身份，
--as user 以当前登录用户身份执行，--as auto 为 User 优先、未登录回退 Bot。

示例:
  # 下载画板图片
  feishu-cli board image <whiteboard_id> output.png

  # 导入 PlantUML 图表
  feishu-cli board import <whiteboard_id> diagram.puml --syntax plantuml

  # 导入 Mermaid 图表
  feishu-cli board import <whiteboard_id> diagram.mmd --syntax mermaid

  # 获取画板节点列表
  feishu-cli board nodes <whiteboard_id>

  # 创建画板节点
  feishu-cli board create-notes <whiteboard_id> nodes.json

  # 更新画板内容（覆盖模式）
  feishu-cli board update <whiteboard_id> nodes.json --overwrite

  # 删除画板节点
  feishu-cli board delete <whiteboard_id> --all`,
}

func init() {
	rootCmd.AddCommand(boardCmd)
}
