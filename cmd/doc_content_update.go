package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/converter"
	"github.com/spf13/cobra"
)

var docContentUpdateCmd = &cobra.Command{
	Use:   "content-update <document_id|url>",
	Short: "更新文档内容（文本级替换 / 块级原子更新）",
	Long: `更新飞书文档内容，全部走官方 docs_ai 单操作原子更新协议（PUT /open-apis/docs_ai/v1/documents/{id}）。

模式（--mode）:
  append         追加到文档末尾
  overwrite      完全覆盖文档内容（会丢失评论与 Markdown 无法表达的内容，非必要不用）
  replace_range  替换定位到的内容（别名 block_replace）
  replace_all    全文查找替换所有匹配
  insert_before  在定位内容前插入
  insert_after   在定位内容后插入（别名 block_insert_after）
  delete_range   删除定位的内容（别名 block_delete）
  str_replace    文本级替换唯一匹配（--pattern，--markdown 为空表示删除该文本）
  block_move_after         把 --src-block-ids 移动到 --block-id 之后
  block_copy_insert_after  把 --src-block-ids 复制一份插入到 --block-id 之后

文档参数支持 docx token、/docx/ URL 与 /wiki/ URL（wiki 自动解析为底层 docx）。

定位方式（二选一，按粒度区分）:
  文本级  --selection-with-ellipsis "纯文本"（不含 ...）：replace_all / replace_range / delete_range
          只替换/删除这段文字本身（服务端 str_replace），所在段落的其余文字与样式原样保留；
          replace_range / delete_range 要求全文唯一命中，replace_all 替换全部命中。
  块级    --selection-by-title "## 标题"            标题及其下属内容（到下一个同级/更高级标题）
          --selection-with-ellipsis "开头...结尾"   从含"开头"的块到其后最近一个含"结尾"的块
          --block-id ID[,ID...]                     指定块（逗号分隔多个块用于批量替换/删除）
          --start-block-id A --end-block-id B       同一父块下的连续闭区间（0=文档开头，-1=文档末尾）
          块级的 replace_range / delete_range / insert_* 命中多处时报错，不会静默取第一处。

block id 用 'feishu-cli doc read <doc> --with-ids' 获取。

内容格式: 默认 --doc-format markdown；--doc-format xml 时按 docs_ai XML 写入。
Markdown 中由 'doc export' 产生的本地方言（> [!NOTE] 高亮块、<image token/>、<mention-user/>、
<mention-doc/>、<file token/>、<grid cols>、<span style> 颜色）会自动转换为 docs_ai 写法；
无法无损写回的占位（<whiteboard token=... type="blank"/>、<bitable/>、<sheet token/>、未下载视频等）
会 fail-closed 拒绝执行，避免把画板/表格写成空白。

本地图片/附件: ![说明](./a.png)（相对 --markdown-file 所在目录）、![说明](@./a.png)、
<img path="@./a.png"/>、<source path="@./r.pdf" name="r.pdf"/> 会自动上传并绑定（占位标记协议，
>20MB 自动分片），仅支持 append/overwrite/insert_*/块级 replace_range；失败项清理占位块并非零退出。

结果: 服务端返回 partial_success 或 failed 时以非零退出码结束，并输出 warnings 与 log_id。

示例:
  # 追加内容
  feishu-cli doc content-update DOC_ID --mode append --markdown "## 新章节\n\n内容"

  # 按标题替换章节（块级）
  feishu-cli doc content-update DOC_ID --mode replace_range \
    --selection-by-title "## 旧章节" --markdown "## 新章节\n\n更新后的内容"

  # 全文查找替换（文本级，只改这几个字，段落其余文字保留）
  feishu-cli doc content-update DOC_ID --mode replace_all \
    --selection-with-ellipsis "旧文本" --markdown "新文本"

  # 文本级替换唯一匹配 / 删除一段文字
  feishu-cli doc content-update DOC_ID --mode str_replace --pattern "v1.0" --markdown "v2.0"
  feishu-cli doc content-update DOC_ID --mode str_replace --pattern "（草稿）" --markdown ""

  # 按 block id 替换 / 删除区间 / 移动
  feishu-cli doc content-update DOC_ID --mode replace_range --block-id doxcnA --markdown "新段落"
  feishu-cli doc content-update DOC_ID --mode delete_range --start-block-id doxcnA --end-block-id doxcnB
  feishu-cli doc content-update DOC_ID --mode block_move_after --block-id doxcnT --src-block-ids doxcnA,doxcnB

  # 在指定章节前插入
  feishu-cli doc content-update DOC_ID --mode insert_before \
    --selection-by-title "## 目标章节" --markdown "## 插入的章节\n\n内容"

  # 从文件读取 markdown
  feishu-cli doc content-update DOC_ID --mode append --markdown-file content.md`,
	Args: cobra.ExactArgs(1),
	RunE: runDocContentUpdate,
}

func init() {
	docCmd.AddCommand(docContentUpdateCmd)
	f := docContentUpdateCmd.Flags()
	f.String("mode", "", "更新模式: append/overwrite/replace_range/replace_all/insert_before/insert_after/delete_range/str_replace/block_move_after/block_copy_insert_after（另接受 block_replace/block_delete/block_insert_after 别名）")
	f.String("markdown", "", "写入内容（默认 Markdown；--doc-format xml 时为 XML）")
	f.String("markdown-file", "", "从文件读取写入内容")
	f.String("content", "", "写入内容（--markdown 的别名，与官方 docs +update 对齐）")
	f.String("content-file", "", "从文件读取写入内容（--markdown-file 的别名）")
	f.String("doc-format", "markdown", "写入内容格式: markdown | xml")
	f.String("selection-by-title", "", "按标题定位（块级，如 \"## 章节标题\"）")
	f.String("selection-with-ellipsis", "", "按内容定位：纯文本=文本级；\"开头...结尾\"=块级范围")
	f.String("pattern", "", "str_replace 模式的匹配文本（按文档 Markdown/XML 序列化逐字匹配，须全文唯一）")
	f.String("block-id", "", "目标块 ID（逗号分隔多个块用于 replace_range/delete_range；insert_*/block_move_after/block_copy_insert_after 为锚点块，-1=文末、0=文首）")
	f.String("start-block-id", "", "块级区间起点（含），与 --end-block-id 成对使用；0 表示文档开头")
	f.String("end-block-id", "", "块级区间终点（含），与 --start-block-id 成对使用；-1 表示文档末尾")
	f.String("src-block-ids", "", "block_move_after / block_copy_insert_after 的源块 ID（逗号分隔）")
	f.StringP("output", "o", "", "输出格式 (json)")
	f.String("user-access-token", "", "User Access Token")
	f.Bool("upload-images", false, "兼容旧参数：内容中的本地图片/附件会自动上传，可省略")
	f.String("table-column-width", "auto",
		"Markdown 表格列宽策略：auto | fixed | 像素列表如 80,200,*,120（* 表示该列走 auto）")
	f.Int("revision-id", -1, "文档版本号（用于并发冲突保护，-1 表示自动基于当前版本）")
	mustMarkFlagRequired(docContentUpdateCmd, "mode")
}

// contentUpdateParams 汇总一次 content-update 的全部参数。
type contentUpdateParams struct {
	documentID   string
	mode         string // 归一化后的模式
	content      string
	contentSet   bool   // 是否显式给了内容（str_replace 允许空内容表示删除）
	docFormat    string // markdown | xml
	formatSet    bool   // --doc-format 是否显式指定（未指定时纯文本替换自动走 XML 以保留样式）
	selByTitle   string
	selEllipsis  string
	pattern      string
	blockID      string // 原样（可能逗号分隔）
	startBlockID string
	endBlockID   string
	srcBlockIDs  string
	output       string
	userToken    string
	revisionID   int
	stdout       io.Writer
	stderr       io.Writer
	resources    []*localDocResource // 本地图片/附件（占位标记协议）
}

func (p *contentUpdateParams) out() io.Writer {
	if p.stdout != nil {
		return p.stdout
	}
	return os.Stdout
}

func (p *contentUpdateParams) errOut() io.Writer {
	if p.stderr != nil {
		return p.stderr
	}
	return os.Stderr
}

func (p *contentUpdateParams) format() string {
	if p.docFormat == "" {
		return "markdown"
	}
	return p.docFormat
}

// contentUpdateModeAliases 接受官方 docs +update 的指令名作为别名。
var contentUpdateModeAliases = map[string]string{
	"block_replace":      "replace_range",
	"block_delete":       "delete_range",
	"block_insert_after": "insert_after",
}

var contentUpdateValidModes = map[string]bool{
	"append": true, "overwrite": true, "replace_range": true, "replace_all": true,
	"insert_before": true, "insert_after": true, "delete_range": true,
	"str_replace": true, "block_move_after": true, "block_copy_insert_after": true,
}

// runDocContentUpdate 是 content-update 命令的主入口
func runDocContentUpdate(cmd *cobra.Command, args []string) error {
	if err := config.Validate(); err != nil {
		return err
	}

	p := &contentUpdateParams{stdout: cmd.OutOrStdout(), stderr: cmd.ErrOrStderr()}
	flags := cmd.Flags()
	p.mode, _ = flags.GetString("mode")
	p.mode = strings.TrimSpace(p.mode)
	if alias, ok := contentUpdateModeAliases[p.mode]; ok {
		p.mode = alias
	}
	p.docFormat, _ = flags.GetString("doc-format")
	p.docFormat = strings.ToLower(strings.TrimSpace(p.docFormat))
	p.formatSet = flags.Changed("doc-format")
	p.selByTitle, _ = flags.GetString("selection-by-title")
	p.selEllipsis, _ = flags.GetString("selection-with-ellipsis")
	p.pattern, _ = flags.GetString("pattern")
	p.blockID, _ = flags.GetString("block-id")
	p.startBlockID, _ = flags.GetString("start-block-id")
	p.endBlockID, _ = flags.GetString("end-block-id")
	p.srcBlockIDs, _ = flags.GetString("src-block-ids")
	p.blockID = strings.TrimSpace(p.blockID)
	p.startBlockID = strings.TrimSpace(p.startBlockID)
	p.endBlockID = strings.TrimSpace(p.endBlockID)
	p.srcBlockIDs = strings.TrimSpace(p.srcBlockIDs)
	output, _ := flags.GetString("output")
	p.output = strings.ToLower(strings.TrimSpace(output))
	if p.output != "" && p.output != "json" {
		return clierr.Usagef("不支持的 --output %q，仅支持 json（或留空使用默认格式）", output)
	}
	if p.docFormat != "markdown" && p.docFormat != "xml" {
		return clierr.Usagef("不支持的 --doc-format %q，仅支持 markdown 或 xml", p.docFormat)
	}

	uploadImages, _ := flags.GetBool("upload-images")
	colWidthRaw, _ := flags.GetString("table-column-width")
	if _, _, errFlag := parseTableColumnWidthFlag(colWidthRaw); errFlag != nil {
		return errFlag
	}
	p.userToken = resolveOptionalUserToken(cmd)
	p.revisionID, _ = flags.GetInt("revision-id")
	if p.revisionID < -1 {
		return clierr.Usagef("--revision-id 必须 >= -1（-1 表示忽略校验或自动最新，当前输入: %d）", p.revisionID)
	}

	// 内容：--markdown/--markdown-file 与别名 --content/--content-file 四选一
	markdownStr, _ := flags.GetString("markdown")
	markdownFile, _ := flags.GetString("markdown-file")
	contentStr, _ := flags.GetString("content")
	contentFile, _ := flags.GetString("content-file")
	// 冲突只看非空值（--markdown "" 可显式表示 str_replace 删除文本）
	nonEmpty, changed := 0, 0
	for _, kv := range []struct{ name, val string }{
		{"markdown", markdownStr}, {"markdown-file", markdownFile}, {"content", contentStr}, {"content-file", contentFile},
	} {
		if kv.val != "" {
			nonEmpty++
		}
		if flags.Changed(kv.name) {
			changed++
		}
	}
	if nonEmpty > 1 {
		return clierr.Usagef("--markdown、--markdown-file、--content、--content-file 只能使用其中一个")
	}
	if contentStr != "" {
		markdownStr = contentStr
	}
	if contentFile != "" {
		markdownFile = contentFile
	}
	p.contentSet = changed > 0
	content, err := resolveMarkdownContent(markdownStr, markdownFile)
	if err != nil && contentUpdateModeNeedsContent(p.mode) {
		return err
	}
	p.content = content

	if err := validateContentUpdateRequest(p); err != nil {
		return err
	}

	// 列宽指令：flag 与内容注释两条入口都须 fail closed 并给出迁移提示
	colWidthContent := ""
	if p.format() == "markdown" {
		colWidthContent = p.content
	}
	if err := validateNoColumnWidthDirective(flags.Changed("table-column-width"), colWidthRaw, colWidthContent); err != nil {
		return err
	}
	if uploadImages {
		fmt.Fprintln(p.errOut(), "提示: content-update 会自动上传内容中的本地图片/附件，--upload-images 可省略")
	}

	// 本地导出方言 → docs_ai 写法（无法无损转换时 fail-closed，发生在任何网络请求之前）
	if p.content != "" && p.format() == "markdown" {
		converted, conv, err := convertLocalDialectForDocsAI(p.content)
		if err != nil {
			return err
		}
		if s := conv.summary(); s != "" {
			fmt.Fprintf(p.errOut(), "提示: 已将 doc export 本地方言转换为 docs_ai 写法（%s）\n", s)
		}
		p.content = converted
	}

	// 本地图片/附件 → 占位标签（离线校验文件存在与路径安全；上传绑定在写入成功后进行）
	if p.content != "" {
		baseDir := ""
		if markdownFile != "" {
			baseDir = filepath.Dir(markdownFile)
		}
		rewritten, resources, err := prepareLocalDocResources(p.content, p.format(), baseDir)
		if err != nil {
			return err
		}
		if len(resources) > 0 {
			if !localResourceModes[p.mode] || (p.mode == "replace_range" && isPlainTextSelector(p.selEllipsis)) {
				return clierr.Usagef("内容含本地图片/附件，只支持 --mode append / overwrite / insert_before / insert_after / replace_range（块级定位）；文本级替换无法插入图片")
			}
			p.content = rewritten
			p.resources = resources
		}
	}

	// 参数校验通过后再解析文档（wiki URL 需要一次 node_by_token 请求）
	documentID, err := resolveDocxArg(args[0], "<document_id|url>", p.userToken)
	if err != nil {
		return err
	}
	p.documentID = documentID
	return executeContentUpdate(p)
}

// resolveMarkdownContent 从 --markdown 或 --markdown-file 获取内容
func resolveMarkdownContent(markdownStr, markdownFile string) (string, error) {
	if markdownStr != "" && markdownFile != "" {
		return "", fmt.Errorf("--markdown 和 --markdown-file 不能同时使用")
	}
	if markdownFile != "" {
		return loadJSONInput("", markdownFile, "markdown", "markdown-file", "Markdown 内容")
	}

	content, err := loadJSONInput(markdownStr, "", "markdown", "markdown-file", "Markdown 内容")
	if err != nil {
		return "", err
	}
	// 处理命令行中的 \n 转义；文件读取必须保持 LaTeX 反斜杠原样。
	return strings.ReplaceAll(content, "\\n", "\n"), nil
}

// contentUpdateModeNeedsContent 判断模式是否必须提供非空内容。
func contentUpdateModeNeedsContent(mode string) bool {
	switch mode {
	case "delete_range", "block_move_after", "block_copy_insert_after", "str_replace":
		return false
	}
	return true
}

// isPlainTextSelector 判断 --selection-with-ellipsis 是否为纯文本（文本级）选择器。
func isPlainTextSelector(selWithEllipsis string) bool {
	return selWithEllipsis != "" && !strings.Contains(strings.TrimSpace(selWithEllipsis), "...")
}

// validateContentUpdateParams 验证模式与选择器组合（兼容旧调用方；新参数见 validateContentUpdateRequest）。
func validateContentUpdateParams(mode, markdown, selByTitle, selWithEllipsis string) error {
	return validateContentUpdateRequest(&contentUpdateParams{
		mode: mode, content: markdown, contentSet: markdown != "",
		selByTitle: selByTitle, selEllipsis: selWithEllipsis,
	})
}

// validateContentUpdateRequest 离线校验参数组合，全部错误按用法错误（exit 2）返回。
func validateContentUpdateRequest(p *contentUpdateParams) error {
	if !contentUpdateValidModes[p.mode] {
		return clierr.Usagef("不支持的模式: %s", p.mode)
	}

	hasTitle := p.selByTitle != ""
	hasEllipsis := p.selEllipsis != ""
	hasBlockID := p.blockID != ""
	hasStart, hasEnd := p.startBlockID != "", p.endBlockID != ""
	hasRange := hasStart || hasEnd
	hasPattern := p.pattern != ""
	hasSrc := p.srcBlockIDs != ""

	if contentUpdateModeNeedsContent(p.mode) && p.content == "" {
		return clierr.Usagef("模式 %s 需要 --markdown 或 --markdown-file（或 --content/--content-file）", p.mode)
	}
	switch p.mode {
	case "delete_range", "block_move_after", "block_copy_insert_after":
		if p.content != "" {
			return clierr.Usagef("模式 %s 不接受写入内容（--markdown/--content）", p.mode)
		}
	}
	if hasPattern && p.mode != "str_replace" {
		return clierr.Usagef("--pattern 只用于 --mode str_replace；replace_all 请用 --selection-with-ellipsis")
	}
	if hasSrc && p.mode != "block_move_after" && p.mode != "block_copy_insert_after" {
		return clierr.Usagef("--src-block-ids 只用于 --mode block_move_after / block_copy_insert_after")
	}
	if hasRange && p.mode != "replace_range" && p.mode != "delete_range" {
		return clierr.Usagef("--start-block-id / --end-block-id 只用于 --mode replace_range / delete_range（block_replace / block_delete）")
	}
	if hasRange && hasStart != hasEnd {
		return clierr.Usagef("--start-block-id 与 --end-block-id 必须成对使用")
	}
	if p.startBlockID == "-1" {
		return clierr.Usagef("--start-block-id 不能为 -1；-1 只能用于 --end-block-id（文档末尾）")
	}
	if p.endBlockID == "0" {
		return clierr.Usagef("--end-block-id 不能为 0；0 只能用于 --start-block-id（文档开头）")
	}
	if hasEllipsis {
		trimmed := strings.TrimSpace(p.selEllipsis)
		if trimmed == "..." {
			return clierr.Usagef("--selection-with-ellipsis 不能仅为省略号 '...'")
		}
		if trimmed == "" {
			return clierr.Usagef("--selection-with-ellipsis 不能为空白")
		}
		if strings.Contains(trimmed, "...") {
			parts := strings.SplitN(trimmed, "...", 2)
			if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				return clierr.Usagef("--selection-with-ellipsis 的起始和结束端点均不能为空，当前输入: %q", p.selEllipsis)
			}
		}
	}

	locators := 0
	for _, b := range []bool{hasTitle, hasEllipsis, hasBlockID, hasRange} {
		if b {
			locators++
		}
	}

	switch p.mode {
	case "append", "overwrite":
		if locators > 0 {
			return clierr.Usagef("模式 %s 不接受定位参数（--selection-*/--block-id/--start-block-id）", p.mode)
		}
	case "str_replace":
		if !hasPattern {
			return clierr.Usagef("模式 str_replace 需要 --pattern")
		}
		if locators > 0 {
			return clierr.Usagef("模式 str_replace 只按 --pattern 匹配，不接受其它定位参数")
		}
	case "block_move_after", "block_copy_insert_after":
		if !hasBlockID || !hasSrc {
			return clierr.Usagef("模式 %s 需要 --block-id（锚点）与 --src-block-ids（源块）", p.mode)
		}
		if strings.Contains(p.blockID, ",") {
			return clierr.Usagef("模式 %s 的 --block-id 只能是单个锚点块", p.mode)
		}
		if hasTitle || hasEllipsis {
			return clierr.Usagef("模式 %s 只按 --block-id / --src-block-ids 定位", p.mode)
		}
	case "replace_all":
		if hasBlockID || hasRange {
			return clierr.Usagef("replace_all 只支持 --selection-by-title 或 --selection-with-ellipsis；按块替换请用 --mode replace_range --block-id")
		}
		fallthrough
	default: // replace_range / delete_range / insert_before / insert_after / replace_all
		if locators == 0 {
			msg := fmt.Sprintf("模式 %s 需要 --selection-by-title、--selection-with-ellipsis、--block-id 或 --start-block-id/--end-block-id 之一", p.mode)
			if p.mode == "replace_all" {
				msg = "模式 replace_all 需要 --selection-by-title 或 --selection-with-ellipsis；如需替换整篇文档请改用 --mode overwrite"
			}
			return clierr.Usagef("%s", msg)
		}
		if locators > 1 {
			return clierr.Usagef("--selection-by-title、--selection-with-ellipsis、--block-id、--start-block-id/--end-block-id 只能使用其中一种定位方式")
		}
		if (p.mode == "insert_before" || p.mode == "insert_after") && strings.Contains(p.blockID, ",") {
			return clierr.Usagef("模式 %s 的 --block-id 只能是单个锚点块", p.mode)
		}
	}
	return nil
}

// ============================================================
// 块文本提取（用于定位匹配）
// ============================================================

// getBlockText 提取块的纯文本内容
func getBlockText(block *larkdocx.Block) string {
	if block == nil || block.BlockType == nil {
		return ""
	}

	extractFromText := func(t *larkdocx.Text) string {
		if t == nil || len(t.Elements) == 0 {
			return ""
		}
		var sb strings.Builder
		for _, elem := range t.Elements {
			if elem == nil {
				continue
			}
			if elem.TextRun != nil && elem.TextRun.Content != nil {
				sb.WriteString(*elem.TextRun.Content)
			}
			if elem.MentionUser != nil && elem.MentionUser.UserId != nil {
				sb.WriteString("@" + *elem.MentionUser.UserId)
			}
			if elem.MentionDoc != nil && elem.MentionDoc.Title != nil {
				sb.WriteString(*elem.MentionDoc.Title)
			}
			if elem.Equation != nil && elem.Equation.Content != nil {
				sb.WriteString(*elem.Equation.Content)
			}
		}
		return sb.String()
	}

	bt := converter.BlockType(*block.BlockType)
	switch bt {
	case converter.BlockTypeText:
		return extractFromText(block.Text)
	case converter.BlockTypeHeading1:
		return extractFromText(block.Heading1)
	case converter.BlockTypeHeading2:
		return extractFromText(block.Heading2)
	case converter.BlockTypeHeading3:
		return extractFromText(block.Heading3)
	case converter.BlockTypeHeading4:
		return extractFromText(block.Heading4)
	case converter.BlockTypeHeading5:
		return extractFromText(block.Heading5)
	case converter.BlockTypeHeading6:
		return extractFromText(block.Heading6)
	case converter.BlockTypeHeading7:
		return extractFromText(block.Heading7)
	case converter.BlockTypeHeading8:
		return extractFromText(block.Heading8)
	case converter.BlockTypeHeading9:
		return extractFromText(block.Heading9)
	case converter.BlockTypeBullet:
		return extractFromText(block.Bullet)
	case converter.BlockTypeOrdered:
		return extractFromText(block.Ordered)
	case converter.BlockTypeQuote:
		return extractFromText(block.Quote)
	case converter.BlockTypeTodo:
		return extractFromText(block.Todo)
	case converter.BlockTypeCode:
		return extractFromText(block.Code)
	default:
		return ""
	}
}

// getBlockHeadingLevel 返回块的标题级别（1-9），非标题返回 0
func getBlockHeadingLevel(block *larkdocx.Block) int {
	if block == nil || block.BlockType == nil {
		return 0
	}
	bt := converter.BlockType(*block.BlockType)
	if bt >= converter.BlockTypeHeading1 && bt <= converter.BlockTypeHeading9 {
		return int(bt - converter.BlockTypeHeading1 + 1)
	}
	return 0
}

// ============================================================
// 定位逻辑（块级）
// ============================================================

// blockRange 表示匹配到的块范围（在 Page 子块中的索引，左闭右开）
type blockRange struct {
	startIndex int // 起始索引（包含）
	endIndex   int // 结束索引（不包含）
}

// findByTitle 按标题定位块范围
// title 格式如 "## 标题文本"，解析出级别和文本
// 范围：从该标题到下一个同级/更高级标题（或文档末尾）
func findByTitle(children []*larkdocx.Block, title string) ([]blockRange, error) {
	// 解析标题级别和文本
	level, text := parseTitleSelector(title)
	if text == "" {
		return nil, fmt.Errorf("标题选择器格式无效: %q", title)
	}

	var ranges []blockRange
	for i, block := range children {
		blockLevel := getBlockHeadingLevel(block)
		if blockLevel == 0 {
			continue // 非标题块不匹配
		}
		if level > 0 && blockLevel != level {
			continue // 指定级别时不匹配其它级别
		}
		blockText := getBlockText(block)
		if !strings.Contains(blockText, text) {
			continue
		}

		// 找到匹配的标题，确定范围终点（遇到同级或更高级标题结束）
		matchLevel := blockLevel
		end := len(children) // 默认到文档末尾
		for j := i + 1; j < len(children); j++ {
			nextLevel := getBlockHeadingLevel(children[j])
			if nextLevel > 0 && nextLevel <= matchLevel {
				end = j
				break
			}
		}
		ranges = append(ranges, blockRange{startIndex: i, endIndex: end})
	}

	if len(ranges) == 0 {
		return nil, fmt.Errorf("未找到匹配的标题: %q", title)
	}
	if outer, inner, ok := firstNestedPair(ranges); ok {
		return nil, fmt.Errorf("标题选择器 %q 同时命中父标题与其子标题（父范围块 %d-%d 完整包含子范围块 %d-%d），"+
			"替换父范围会连带删除其中未匹配的兄弟章节，意图存在歧义；"+
			"请用带级别的选择器精确定位（如 \"# %s\" 或 \"## %s\"），或改用 --mode replace_range 逐个处理",
			title, outer.startIndex, outer.endIndex, inner.startIndex, inner.endIndex, text, text)
	}
	return ranges, nil
}

// firstNestedPair 找出第一对「一个范围完整包含另一个」的组合。
//
// 不带级别的选择器（如 "部署"）会同时命中 H1「部署总览」与其子节 H2「部署检查」，
// 而 H1 的范围按定义包含 H2 及其后所有兄弟子节。doReplaceAll 逆序逐个
// block_replace 时，外层范围会连带吞掉内层与外层之间**未匹配**的章节：
// 实测 7 块文档一次 `replace_all --selection-by-title 部署` 后只剩 2 块，
// 明确无关的「其它章节」被销毁，而命令报告"成功替换 2 处"且 exit 0。
//
// 这种命中下用户意图本身是歧义的（要替父节还是子节？），任何静默选择都可能毁数据，
// 因此 fail-closed 要求用户用带级别的选择器或 replace_range 明确表达。
func firstNestedPair(ranges []blockRange) (outer, inner blockRange, ok bool) {
	for i, a := range ranges {
		for j, b := range ranges {
			if i == j {
				continue
			}
			if a.startIndex <= b.startIndex && a.endIndex >= b.endIndex &&
				(a.startIndex != b.startIndex || a.endIndex != b.endIndex) {
				return a, b, true
			}
		}
	}
	return blockRange{}, blockRange{}, false
}

// parseTitleSelector 解析标题选择器，如 "## 标题" → (2, "标题")
func parseTitleSelector(title string) (int, string) {
	title = strings.TrimSpace(title)
	level := 0
	for _, ch := range title {
		if ch == '#' {
			level++
		} else {
			break
		}
	}
	if level == 0 {
		// 没有 # 前缀，当作 text 精确匹配，默认级别 0 表示匹配任意标题
		return 0, title
	}
	text := strings.TrimSpace(title[level:])
	return level, text
}

// findByEllipsis 按省略号定位块范围
// 格式："开头内容...结尾内容"（块级范围）或不含 "..." 的文本（返回所有含该文本的块）
func findByEllipsis(children []*larkdocx.Block, selector string) ([]blockRange, error) {
	parts := strings.SplitN(selector, "...", 2)
	if len(parts) == 2 {
		startText := strings.TrimSpace(parts[0])
		endText := strings.TrimSpace(parts[1])
		return findByStartEnd(children, startText, endText)
	}

	// 精确匹配：找所有包含该文本的块
	text := strings.TrimSpace(selector)
	var ranges []blockRange
	for i, block := range children {
		if strings.Contains(getBlockText(block), text) {
			ranges = append(ranges, blockRange{startIndex: i, endIndex: i + 1})
		}
	}
	if len(ranges) == 0 {
		return nil, fmt.Errorf("未找到包含文本的块: %q", text)
	}
	return ranges, nil
}

// findByStartEnd 查找所有「含 startText 的块 → 其后最近一个含 endText 的块」范围（互不重叠，按文档顺序）。
//
// 结束锚点取起点之后**最近**的一次出现：此前取最后一次出现，`开头...结尾` 在"结尾"多次出现时
// 会一路删/替换到文末。起点块自身在 startText 之后出现 endText 时，范围就是该块本身。
func findByStartEnd(children []*larkdocx.Block, startText, endText string) ([]blockRange, error) {
	var ranges []blockRange
	pos := 0
	for pos < len(children) {
		startIdx := -1
		for i := pos; i < len(children); i++ {
			if strings.Contains(getBlockText(children[i]), startText) {
				startIdx = i
				break
			}
		}
		if startIdx < 0 {
			break
		}
		endIdx := -1
		startBlockText := getBlockText(children[startIdx])
		if at := strings.Index(startBlockText, startText); at >= 0 &&
			strings.Contains(startBlockText[at+len(startText):], endText) {
			endIdx = startIdx
		} else {
			for i := startIdx + 1; i < len(children); i++ {
				if strings.Contains(getBlockText(children[i]), endText) {
					endIdx = i
					break
				}
			}
		}
		if endIdx < 0 {
			if len(ranges) == 0 {
				return nil, fmt.Errorf("未找到起始文本 %q 之后包含结束文本的块: %q", startText, endText)
			}
			break
		}
		ranges = append(ranges, blockRange{startIndex: startIdx, endIndex: endIdx + 1})
		pos = endIdx + 1
	}
	if len(ranges) == 0 {
		return nil, fmt.Errorf("未找到包含起始文本的块: %q", startText)
	}
	return ranges, nil
}

// findSelection 统一定位入口（返回全部命中范围）
func findSelection(children []*larkdocx.Block, selByTitle, selWithEllipsis string) ([]blockRange, error) {
	if selByTitle != "" {
		return findByTitle(children, selByTitle)
	}
	return findByEllipsis(children, selWithEllipsis)
}

// findSingleSelection 块级单范围模式（replace_range/delete_range/insert_*）的定位：
// 命中多处时报错而不是静默取第一处。
func findSingleSelection(children []*larkdocx.Block, mode, selByTitle, selWithEllipsis string) (blockRange, error) {
	ranges, err := findSelection(children, selByTitle, selWithEllipsis)
	if err != nil {
		return blockRange{}, err
	}
	if len(ranges) > 1 {
		selector := selByTitle
		if selector == "" {
			selector = selWithEllipsis
		}
		var where []string
		for i, r := range ranges {
			if i >= 5 {
				where = append(where, "…")
				break
			}
			where = append(where, fmt.Sprintf("块 %d-%d", r.startIndex, r.endIndex-1))
		}
		hint := "请使用更精确的选择器（带级别的标题、更长的文本），或用 --block-id 指定块"
		if mode == "replace_range" || mode == "delete_range" {
			hint += "；确需处理全部命中请改用 --mode replace_all"
		}
		return blockRange{}, clierr.Usagef("选择器 %q 命中 %d 处（%s），%s 只处理唯一命中，已拒绝执行以免改错位置；%s",
			selector, len(ranges), strings.Join(where, "、"), mode, hint)
	}
	r := ranges[0]
	if r.startIndex >= len(children) || r.endIndex <= r.startIndex {
		return blockRange{}, fmt.Errorf("定位范围无效: [%d, %d)", r.startIndex, r.endIndex)
	}
	return r, nil
}

// ============================================================
// 获取文档顶层子块（Page 的直接子块）
// ============================================================

// getPageChildren 获取文档的 Page 块的直接子块
func getPageChildren(documentID, userAccessToken string) ([]*larkdocx.Block, error) {
	return client.GetAllBlockChildren(documentID, documentID, userAccessToken)
}

// ============================================================
// 本地资源检查（Fail Closed）
// ============================================================

// localResourceRegex 匹配 Markdown 图片与文件链接 ![alt](target)
var localResourceRegex = regexp.MustCompile(`!\[.*?\]\((.*?)\)`)

// containsLocalMarkdownResources 检查 Markdown 中是否包含本地文件/图片资源
func containsLocalMarkdownResources(content string) bool {
	matches := localResourceRegex.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) > 1 {
			target := strings.TrimSpace(m[1])
			if target != "" &&
				!strings.HasPrefix(target, "http://") &&
				!strings.HasPrefix(target, "https://") &&
				!strings.HasPrefix(target, "data:") &&
				!strings.HasPrefix(target, "#") {
				return true
			}
		}
	}
	return false
}

// validateNoColumnWidthDirective 检查列宽自定义诉求；本命令暂不支持，须 fail closed。
//
// 必须同时看 flag 与内容：`<!-- feishu-colwidth: ... -->` 是文档化的等价入口
// （CLAUDE.md 有记载，internal/converter 也实现了它）。只拦 flag 会让注释形态被静默
// 丢弃——表格以默认列宽落地，用户却毫无提示，与显式传 flag 时的明确报错自相矛盾。
func validateNoColumnWidthDirective(flagChanged bool, flagValue, markdown string) error {
	if flagChanged && flagValue != "auto" {
		return clierr.Usagef("doc content-update 现采用官方原子安全更新协议，暂不支持 --table-column-width 自定义列宽；如需保留/自定义表格列宽，请改用 'feishu-cli doc import' 进行文档导入")
	}
	if colWidthCommentInMarkdownRe.MatchString(markdown) {
		return clierr.Usagef("检测到 Markdown 含 <!-- feishu-colwidth: ... --> 列宽指令；doc content-update 现采用官方原子安全更新协议，暂不支持自定义列宽；请移除该注释，或改用 'feishu-cli doc import' 进行文档导入")
	}
	return nil
}

// colWidthCommentInMarkdownRe 匹配任意行上的 <!-- feishu-colwidth: ... --> 指令
// （与 internal/converter 的 colWidthCommentRe 同义，此处按多行扫描整篇内容）。
var colWidthCommentInMarkdownRe = regexp.MustCompile(`(?m)^\s*<!--\s*feishu-colwidth\s*:[^>]*-->\s*$`)

// validateNoLocalResources 检查是否包含本地文件/图片资源；若有则 fail closed 并给出迁移提示
func validateNoLocalResources(uploadImages bool, markdown string) error {
	if uploadImages {
		return clierr.Usagef("doc content-update 现采用官方原子安全更新协议，暂不支持 --upload-images；如需上传本地图片，请改用图床/网络图片 URL，或使用 'feishu-cli doc import' 全量导入")
	}
	if containsLocalMarkdownResources(markdown) {
		return clierr.Usagef("检测到 Markdown 包含本地文件/图片资源；doc content-update 暂不支持本地文件混合上传；请改用网络图片 URL，写入后用 'feishu-cli doc media-insert' 插入本地图片，或使用 'feishu-cli doc import' 导入")
	}
	return nil
}

// ============================================================
// 执行（对齐官方 PUT /open-apis/docs_ai/v1/documents/{id} 原子更新能力）
// ============================================================

// injectRevisionID 集中处理 revision_id 注入：
// 官方规则：默认 -1 发送，正整数发送，显式 0 省略不发送（<-1 之前在参数阶段已校验拦截）
func injectRevisionID(body map[string]any, revisionID int) {
	if revisionID != 0 && revisionID >= -1 {
		body["revision_id"] = revisionID
	}
}

// extractRevisionID 独立检查所有支持的位置，并采纳第一个有效正整数版本号
func extractRevisionID(data map[string]any) int {
	if data == nil {
		return -1
	}
	if docObj, ok := data["document"].(map[string]any); ok {
		if rev := parseRevisionNumber(docObj["revision_id"]); rev > 0 {
			return rev
		}
	}
	if rev := parseRevisionNumber(data["revision_id"]); rev > 0 {
		return rev
	}
	if rev := parseRevisionNumber(data["document_revision_id"]); rev > 0 {
		return rev
	}
	return -1
}

// parseRevisionNumber 解析服务端返回的 revision_id。
// 覆盖 json.Number：当前 UpdateDocContentAtomic 走普通 json.Unmarshal（产出 float64），
// 但本仓多处已切到 UseNumber 解码；漏掉该类型会让 doReplaceAll 在多范围替换中途
// 因取不到新 revision 而 fail-closed 中断，留下部分替换的文档。
func parseRevisionNumber(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return -1
}

// newDocsAIBody 构造 docs_ai 更新请求体。
func (p *contentUpdateParams) newBody(command string) map[string]any {
	return map[string]any{
		"format":  p.format(),
		"command": command,
	}
}

// sendUpdate 发送一次原子更新；返回 data 与错误（含 *client.DocsAIResultError）。
func (p *contentUpdateParams) sendUpdate(body map[string]any, revisionID int) (map[string]any, error) {
	injectRevisionID(body, revisionID)
	return client.UpdateDocContentAtomic(p.documentID, body, p.userToken)
}

// finishUpdate 统一输出一次更新的结果：
//   - 成功：JSON 模式打印 data（含 log_id）；文本模式打印 successMsg，warnings 与 log_id 打到 stderr；
//   - result=partial_success / failed：JSON 模式仍打印 data，随后返回非零错误（含 warnings 与 log_id）。
func (p *contentUpdateParams) finishUpdate(data map[string]any, err error, failPrefix, successMsg string) error {
	// 本地资源：写入成功（或部分成功、占位块已建出）后上传并绑定，失败项清理占位块
	var resErr error
	resData := data
	if rerr, ok := client.AsDocsAIResultError(err); ok && rerr.Result == "partial_success" {
		resData = rerr.Data
	}
	if len(p.resources) > 0 && resData != nil {
		if rerr, ok := client.AsDocsAIResultError(err); err == nil || (ok && rerr.Result == "partial_success") {
			okAll := finalizeLocalDocResources(p, resData, p.resources)
			resData["local_resources"] = p.resources
			if !okAll {
				resErr = p.reportLocalResources(nil, p.resources, false)
			}
		}
	}
	if err != nil {
		if rerr, ok := client.AsDocsAIResultError(err); ok {
			if p.output == "json" && rerr.Data != nil {
				if perr := printJSONTo(p.out(), rerr.Data); perr != nil {
					return perr
				}
			} else if resErr != nil {
				p.printLocalResourceLines()
			}
			return fmt.Errorf("%s: %w", failPrefix, err)
		}
		return fmt.Errorf("%s: %w", failPrefix, err)
	}
	if p.output == "json" {
		if perr := printJSONTo(p.out(), data); perr != nil {
			return perr
		}
		return resErr
	}
	fmt.Fprintln(p.out(), successMsg)
	p.printWarnings(data)
	p.printLocalResourceLines()
	return resErr
}

// printWarnings 文本模式下把成功结果中的 warnings / log_id 打到 stderr（此前被静默吞掉）。
func (p *contentUpdateParams) printWarnings(data map[string]any) {
	warnings := client.DocsAIWarnings(data)
	if len(warnings) == 0 {
		return
	}
	for _, w := range warnings {
		fmt.Fprintf(p.errOut(), "⚠ 服务端警告: %s\n", w)
	}
	if logID, _ := data["log_id"].(string); logID != "" {
		fmt.Fprintf(p.errOut(), "  log_id: %s\n", logID)
	}
}

func printJSONTo(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// executeContentUpdate 按模式分派。
func executeContentUpdate(p *contentUpdateParams) error {
	switch p.mode {
	case "append":
		return p.doAppend()
	case "overwrite":
		return p.doOverwrite()
	case "str_replace":
		return p.doTextReplace(p.pattern, false)
	case "replace_all":
		if isPlainTextSelector(p.selEllipsis) {
			return p.doTextReplace(strings.TrimSpace(p.selEllipsis), true)
		}
		return p.doBlockReplaceAll()
	case "replace_range", "delete_range":
		if isPlainTextSelector(p.selEllipsis) {
			if p.mode == "delete_range" {
				p.content = ""
			}
			return p.doTextReplace(strings.TrimSpace(p.selEllipsis), false)
		}
		return p.doBlockReplaceOrDelete()
	case "insert_before":
		return p.doInsertBefore()
	case "insert_after":
		return p.doInsertAfter()
	case "block_move_after", "block_copy_insert_after":
		body := p.newBody(p.mode)
		body["block_id"] = p.blockID
		body["src_block_ids"] = p.srcBlockIDs
		data, err := p.sendUpdate(body, p.revisionID)
		verb := "移动"
		if p.mode == "block_copy_insert_after" {
			verb = "复制"
		}
		return p.finishUpdate(data, err, verb+"块失败",
			fmt.Sprintf("已将块 %s %s到块 %s 之后", p.srcBlockIDs, verb, p.blockID))
	}
	return clierr.Usagef("不支持的模式: %s", p.mode)
}

// doAppend 追加到文档末尾（对齐官方：docs_ai 将 append 转为 block_insert_after + block_id="-1"）
func (p *contentUpdateParams) doAppend() error {
	body := p.newBody("block_insert_after")
	body["block_id"] = "-1"
	body["content"] = p.content
	data, err := p.sendUpdate(body, p.revisionID)
	return p.finishUpdate(data, err, "追加内容失败", "文档内容追加成功！")
}

// doOverwrite 完全覆盖文档内容（官方单操作原子 overwrite，彻底消除先删后写破坏窗口）
func (p *contentUpdateParams) doOverwrite() error {
	body := p.newBody("overwrite")
	body["content"] = p.content
	data, err := p.sendUpdate(body, p.revisionID)
	return p.finishUpdate(data, err, "覆盖内容失败", "文档内容覆盖成功！")
}

// doBlockReplaceOrDelete 块级 replace_range / delete_range（block_replace / block_delete）。
func (p *contentUpdateParams) doBlockReplaceOrDelete() error {
	command := "block_replace"
	failPrefix := "替换内容失败"
	if p.mode == "delete_range" {
		command = "block_delete"
		failPrefix = "删除内容失败"
	}
	body := p.newBody(command)
	if command == "block_replace" {
		body["content"] = p.content
	}

	var desc string
	switch {
	case p.blockID != "":
		body["block_id"] = p.blockID
		desc = fmt.Sprintf("块 %s", p.blockID)
	case p.startBlockID != "":
		body["start_block_id"] = p.startBlockID
		body["end_block_id"] = p.endBlockID
		desc = fmt.Sprintf("块 %s 到 %s", p.startBlockID, p.endBlockID)
	default:
		children, err := getPageChildren(p.documentID, p.userToken)
		if err != nil {
			return fmt.Errorf("获取文档内容失败: %w", err)
		}
		r, err := findSingleSelection(children, p.mode, p.selByTitle, p.selEllipsis)
		if err != nil {
			return err
		}
		startBlockID := client.StringVal(children[r.startIndex].BlockId)
		endBlockID := client.StringVal(children[r.endIndex-1].BlockId)
		body["start_block_id"] = startBlockID
		body["end_block_id"] = endBlockID
		desc = fmt.Sprintf("索引 %d 到 %d 的内容（块 %s 到 %s）", r.startIndex, r.endIndex, startBlockID, endBlockID)
	}

	data, err := p.sendUpdate(body, p.revisionID)
	if command == "block_delete" {
		return p.finishUpdate(data, err, failPrefix, "已删除"+desc)
	}
	return p.finishUpdate(data, err, failPrefix, "已替换"+desc)
}

// doBlockReplaceAll 块级 replace_all（标题选择器或 "开头...结尾" 范围）：倒序逐个原子 block_replace，
// 部分失败时非零退出并报告已完成项。
func (p *contentUpdateParams) doBlockReplaceAll() error {
	children, err := getPageChildren(p.documentID, p.userToken)
	if err != nil {
		return fmt.Errorf("获取文档内容失败: %w", err)
	}

	ranges, err := findSelection(children, p.selByTitle, p.selEllipsis)
	if err != nil {
		return err
	}

	replaced := 0
	totalRanges := len(ranges)
	currentRevision := p.revisionID
	var lastData map[string]any

	for i := totalRanges - 1; i >= 0; i-- {
		r := ranges[i]
		if r.startIndex >= len(children) || r.endIndex <= r.startIndex {
			continue
		}
		startBlockID := client.StringVal(children[r.startIndex].BlockId)
		endBlockID := client.StringVal(children[r.endIndex-1].BlockId)

		body := p.newBody("block_replace")
		body["content"] = p.content
		body["start_block_id"] = startBlockID
		body["end_block_id"] = endBlockID

		data, err := p.sendUpdate(body, currentRevision)
		if err != nil {
			return fmt.Errorf("全文替换未完全完成：共 %d 处匹配，已成功完成 %d 处，在第 %d 处替换失败（索引 %d 到 %d，块 %s 到 %s）: %w",
				totalRanges, replaced, totalRanges-i, r.startIndex, r.endIndex, startBlockID, endBlockID, err)
		}
		replaced++
		lastData = data
		p.printWarnings(data)

		nextRev := extractRevisionID(data)
		if i > 0 {
			if nextRev <= 0 {
				return fmt.Errorf("全文替换中断：共 %d 处匹配，已成功完成 %d 处，但服务端未返回新的 revision_id；为防止并发数据破坏已停止后续未保护替换",
					totalRanges, replaced)
			}
			currentRevision = nextRev
		}
	}

	if p.output == "json" {
		out := map[string]any{
			"document_id":    p.documentID,
			"replaced_count": replaced,
			"granularity":    "block",
		}
		if rev := extractRevisionID(lastData); rev > 0 {
			out["revision_id"] = rev
		}
		return printJSONTo(p.out(), out)
	}
	fmt.Fprintf(p.out(), "全文替换完成，共替换 %d 处\n", replaced)
	return nil
}

// doInsertBefore 在定位内容前插入（原子 block_insert_after 到前一个兄弟块；文档开头用 "0"）
func (p *contentUpdateParams) doInsertBefore() error {
	anchor := ""
	var desc string
	if p.blockID != "" {
		prev, err := previousSiblingAnchor(p.documentID, p.blockID, p.userToken)
		if err != nil {
			return err
		}
		anchor = prev
		desc = fmt.Sprintf("已在块 %s 前插入内容", p.blockID)
	} else {
		children, err := getPageChildren(p.documentID, p.userToken)
		if err != nil {
			return fmt.Errorf("获取文档内容失败: %w", err)
		}
		r, err := findSingleSelection(children, p.mode, p.selByTitle, p.selEllipsis)
		if err != nil {
			return err
		}
		anchor = "0"
		if r.startIndex > 0 {
			anchor = client.StringVal(children[r.startIndex-1].BlockId)
		}
		desc = fmt.Sprintf("已在索引 %d 前插入内容", r.startIndex)
	}
	body := p.newBody("block_insert_after")
	body["block_id"] = anchor
	body["content"] = p.content
	data, err := p.sendUpdate(body, p.revisionID)
	return p.finishUpdate(data, err, "插入内容失败", desc)
}

// previousSiblingAnchor 计算"在块 X 之前插入"对应的 block_insert_after 锚点。
// X 是其父块的第一个子块时：父块为文档根则用 "0"（文档开头），否则无法表达，报错。
func previousSiblingAnchor(documentID, blockID, userAccessToken string) (string, error) {
	if blockID == "0" || blockID == "-1" {
		return "", clierr.Usagef("insert_before 的 --block-id 必须是真实块 ID；在文档开头插入请用 --mode insert_after --block-id 0")
	}
	block, err := client.GetBlock(documentID, blockID, userAccessToken)
	if err != nil {
		return "", fmt.Errorf("获取锚点块失败: %w", err)
	}
	parentID := client.StringVal(block.ParentId)
	if parentID == "" {
		parentID = documentID
	}
	siblings, err := client.GetAllBlockChildren(documentID, parentID, userAccessToken)
	if err != nil {
		return "", fmt.Errorf("获取锚点块的兄弟块失败: %w", err)
	}
	for i, s := range siblings {
		if client.StringVal(s.BlockId) != blockID {
			continue
		}
		if i > 0 {
			return client.StringVal(siblings[i-1].BlockId), nil
		}
		if parentID == documentID {
			return "0", nil
		}
		return "", fmt.Errorf("块 %s 是父块 %s 的第一个子块，docs_ai 无法在容器首个子块前插入；请改用 --mode insert_after 指定其它锚点", blockID, parentID)
	}
	return "", fmt.Errorf("在父块 %s 下未找到块 %s", parentID, blockID)
}

// doInsertAfter 在定位内容后插入（原子 block_insert_after）
func (p *contentUpdateParams) doInsertAfter() error {
	anchor := p.blockID
	desc := fmt.Sprintf("已在块 %s 后插入内容", p.blockID)
	if anchor == "" {
		children, err := getPageChildren(p.documentID, p.userToken)
		if err != nil {
			return fmt.Errorf("获取文档内容失败: %w", err)
		}
		r, err := findSingleSelection(children, p.mode, p.selByTitle, p.selEllipsis)
		if err != nil {
			return err
		}
		anchor = client.StringVal(children[r.endIndex-1].BlockId)
		desc = fmt.Sprintf("已在索引 %d 后插入内容（块 %s）", r.endIndex-1, anchor)
	}
	body := p.newBody("block_insert_after")
	body["block_id"] = anchor
	body["content"] = p.content
	data, err := p.sendUpdate(body, p.revisionID)
	return p.finishUpdate(data, err, "插入内容失败", desc)
}

// ============================================================
// 兼容旧调用方的包装（单测与内部调用）
// ============================================================

func legacyParams(documentID, mode, markdown, selByTitle, selWithEllipsis, output, userAccessToken string, revisionID int) *contentUpdateParams {
	return &contentUpdateParams{
		documentID: documentID, mode: mode, content: markdown, contentSet: markdown != "",
		docFormat: "markdown", selByTitle: selByTitle, selEllipsis: selWithEllipsis,
		output: output, userToken: userAccessToken, revisionID: revisionID,
	}
}

func doAppend(documentID, markdown string, output, userAccessToken string, revisionID int) error {
	return executeContentUpdate(legacyParams(documentID, "append", markdown, "", "", output, userAccessToken, revisionID))
}

func doOverwrite(documentID, markdown string, output, userAccessToken string, revisionID int) error {
	return executeContentUpdate(legacyParams(documentID, "overwrite", markdown, "", "", output, userAccessToken, revisionID))
}

func doReplaceRange(documentID, markdown, selByTitle, selWithEllipsis string, output, userAccessToken string, revisionID int) error {
	return executeContentUpdate(legacyParams(documentID, "replace_range", markdown, selByTitle, selWithEllipsis, output, userAccessToken, revisionID))
}

func doReplaceAll(documentID, markdown, selByTitle, selWithEllipsis string, output, userAccessToken string, revisionID int) error {
	return executeContentUpdate(legacyParams(documentID, "replace_all", markdown, selByTitle, selWithEllipsis, output, userAccessToken, revisionID))
}

func doInsertBefore(documentID, markdown, selByTitle, selWithEllipsis string, output, userAccessToken string, revisionID int) error {
	return executeContentUpdate(legacyParams(documentID, "insert_before", markdown, selByTitle, selWithEllipsis, output, userAccessToken, revisionID))
}

func doInsertAfter(documentID, markdown, selByTitle, selWithEllipsis string, output, userAccessToken string, revisionID int) error {
	return executeContentUpdate(legacyParams(documentID, "insert_after", markdown, selByTitle, selWithEllipsis, output, userAccessToken, revisionID))
}

func doDeleteRange(documentID, selByTitle, selWithEllipsis string, output, userAccessToken string, revisionID int) error {
	return executeContentUpdate(legacyParams(documentID, "delete_range", "", selByTitle, selWithEllipsis, output, userAccessToken, revisionID))
}
