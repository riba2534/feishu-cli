package cmd

import (
	"github.com/spf13/cobra"
)

var docCmd = &cobra.Command{
	Use:   "doc",
	Short: "文档操作命令",
	Long: `文档操作命令：创建、读取、导入导出、编辑与删除飞书云文档（docx）内容。

子命令:
  读取
    get            获取文档信息（标题、版本号）
    read           选择性读取（大纲 / 按标题取节 / 关键词定位，适合大文档）
    blocks         获取文档所有块（原始块结构）
    export         导出文档为 Markdown（--download-images 落地图片/画板）
    export-file    导出文档为文件（PDF / Word / Excel）
    media-download 下载文档素材（图片 / 附件 / 画板缩略图，单个素材上限 100MB）
    media-preview  预览文档素材（图片 / 附件 / 评论图片，保存源文件并自动补扩展名）
    history        历史版本：列出 / 回滚 / 查询回滚状态
  创建与导入
    create         创建新文档
    import         从 Markdown 导入创建文档或追加到已有文档
    import-file    导入 Word/Excel 等文件为云文档
  编辑
    content-update 更新文档内容（追加 / 覆盖 / 替换 / 插入 / 删除，docs_ai 原子协议）
    add            向文档添加内容块（JSON 或 Markdown）
    add-board      添加空白画板
    add-callout    添加高亮块
    media-insert   插入图片或附件
    resource       文档封面图：下载 / 设置（文件、HTTPS 图片、剪贴板）/ 删除
    htmlbox        妙笔BOX HTML 小组件块（create/update/get/delete，可跑动画/图表）
    update         更新单个块
    batch-update   批量更新块
    delete         删除父块下的子块
    table          表格操作（插入/删除行列、合并/取消合并单元格）

文档参数（<document_id> 等）除裸 ID 外也接受 /docx/ 与 /wiki/ URL（wiki 自动解析为底层文档；htmlbox 暂只接受裸 ID）。

示例:
  # 创建文档
  feishu-cli doc create --title "我的文档"

  # 获取文档信息（ID 或 URL 均可）
  feishu-cli doc get <document_id>
  feishu-cli doc get https://xxx.feishu.cn/wiki/wikcnXXXXXX

  # 大文档按标题读取某一节
  feishu-cli doc read <document_id> --heading "背景"

  # 导出为 Markdown
  feishu-cli doc export <document_id> --output doc.md

  # 导出为 PDF
  feishu-cli doc export-file <document_id> --type pdf -o output.pdf

  # 导入 Word 文档
  feishu-cli doc import-file report.docx --type docx

  # 插入图片到文档
  feishu-cli doc media-insert DOC_ID --file photo.png --type image

  # 下载文档图片
  feishu-cli doc media-download boxcnXXX -o image.png`,
}

func init() {
	rootCmd.AddCommand(docCmd)
}
