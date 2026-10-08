package cmd

import (
	"github.com/spf13/cobra"
)

var slidesCmd = &cobra.Command{
	Use:   "slides",
	Short: "演示文稿（Slides）操作命令",
	Long: `飞书 Slides 演示文稿操作命令。

子命令:
  create        创建新的 Slides 演示文稿（可用 --slide/--slides 一次带最多 10 页，<img src="@./a.png"> 自动上传）
  get           读取全文或单页 XML（别名 xml-get；--slide-id/--slide-number/--output-file/--remove-attr-id）
  add-slide     向已有演示文稿追加或插入一页
  delete-slide  按 slide_id 删除一页（需确认，非交互环境加 --yes）
  replace-slide 元素级编辑：block_replace / block_insert（自动注入 id 与 <content/>）
  update-slide  整页覆盖：把完整 <slide> XML 写回该页（保留 slide_id 与页序）
  screenshot    把页面或一段 slide XML 渲染成本地图片
  media-upload  上传本地图片到演示文稿，返回 file_token（可作为 <img src=...> 使用）

写页面的命令默认由服务端做 XML lint（error 级问题拒绝写入，错误码 4000153），--no-lint 跳过。

权限要求（User Token 推荐）:
  - slides:presentation:create / slides:presentation:write_only  创建/写入
  - slides:presentation:update                                   编辑页面
  - slides:presentation:read                                     读取演示文稿
  - slides:presentation:screenshot                               截图
  - docs:document.media:upload                                   上传媒体

示例:
  # 创建演示文稿
  feishu-cli slides create --title "My Deck"

  # 读取演示文稿 XML / 单页 XML
  feishu-cli slides get <xml_presentation_id>
  feishu-cli slides get <xml_presentation_id> --slide-number 1

  # 追加一页、整页覆盖、截图
  feishu-cli slides add-slide <xml_presentation_id> --slide @page.xml
  feishu-cli slides update-slide <xml_presentation_id> --slide-id <slide_id> --content @page.xml
  feishu-cli slides screenshot <xml_presentation_id> --slide-number 1 --output cover

  # 上传图片
  feishu-cli slides media-upload --file ./cover.png --presentation-token <xml_presentation_id>`,
}

func init() {
	rootCmd.AddCommand(slidesCmd)
}
