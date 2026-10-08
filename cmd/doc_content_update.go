package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/converter"
	"github.com/spf13/cobra"
)

var docContentUpdateCmd = &cobra.Command{
	Use:   "content-update <document_id|url>",
	Short: "更新文档内容（支持 7 种模式）",
	Long: `更新飞书文档内容，支持追加、覆盖、定位替换、插入和删除。

模式:
  append         追加到文档末尾
  overwrite      完全覆盖文档内容
  replace_range  按定位替换一段内容
  replace_all    全文查找替换所有匹配
  insert_before  在定位内容前插入
  insert_after   在定位内容后插入
  delete_range   删除定位的内容

文档参数支持 docx token、/docx/ URL 与 /wiki/ URL（wiki 自动解析为底层 docx）。

定位方式（replace_range/replace_all/insert_before/insert_after/delete_range 必需）:
  --selection-by-title "## 标题"        按标题定位
  --selection-with-ellipsis "开头...结尾" 按内容范围定位

示例:
  # 追加内容
  feishu-cli doc content-update DOC_ID --mode append --markdown "## 新章节\n\n内容"

  # 完全覆盖
  feishu-cli doc content-update DOC_ID --mode overwrite --markdown "# 新文档\n\n全新内容"

  # 按标题替换章节
  feishu-cli doc content-update DOC_ID --mode replace_range \
    --selection-by-title "## 旧章节" --markdown "## 新章节\n\n更新后的内容"

  # 全文查找替换
  feishu-cli doc content-update DOC_ID --mode replace_all \
    --selection-with-ellipsis "旧文本" --markdown "新文本"

  # 在指定章节前插入
  feishu-cli doc content-update DOC_ID --mode insert_before \
    --selection-by-title "## 目标章节" --markdown "## 插入的章节\n\n内容"

  # 删除一段内容
  feishu-cli doc content-update DOC_ID --mode delete_range \
    --selection-by-title "## 废弃章节"

  # 从文件读取 markdown
  feishu-cli doc content-update DOC_ID --mode append --markdown-file content.md`,
	Args: cobra.ExactArgs(1),
	RunE: runDocContentUpdate,
}

func init() {
	docCmd.AddCommand(docContentUpdateCmd)
	docContentUpdateCmd.Flags().String("mode", "", "更新模式: append/overwrite/replace_range/replace_all/insert_before/insert_after/delete_range")
	docContentUpdateCmd.Flags().String("markdown", "", "Markdown 内容")
	docContentUpdateCmd.Flags().String("markdown-file", "", "从文件读取 Markdown")
	docContentUpdateCmd.Flags().String("selection-by-title", "", "按标题定位（如 \"## 章节标题\"）")
	docContentUpdateCmd.Flags().String("selection-with-ellipsis", "", "按内容范围定位（如 \"开头内容...结尾内容\"）")
	docContentUpdateCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	docContentUpdateCmd.Flags().String("user-access-token", "", "User Access Token")
	docContentUpdateCmd.Flags().Bool("upload-images", false, "上传 Markdown 中的本地图片")
	docContentUpdateCmd.Flags().String("table-column-width", "auto",
		"Markdown 表格列宽策略：auto | fixed | 像素列表如 80,200,*,120（* 表示该列走 auto）")
	docContentUpdateCmd.Flags().Int("revision-id", -1, "文档版本号（用于并发冲突保护，-1 表示自动基于当前版本）")
	mustMarkFlagRequired(docContentUpdateCmd, "mode")
}

// runDocContentUpdate 是 content-update 命令的主入口
func runDocContentUpdate(cmd *cobra.Command, args []string) error {
	if err := config.Validate(); err != nil {
		return err
	}

	mode, _ := cmd.Flags().GetString("mode")
	markdownStr, _ := cmd.Flags().GetString("markdown")
	markdownFile, _ := cmd.Flags().GetString("markdown-file")
	selByTitle, _ := cmd.Flags().GetString("selection-by-title")
	selWithEllipsis, _ := cmd.Flags().GetString("selection-with-ellipsis")
	output, _ := cmd.Flags().GetString("output")
	output = strings.ToLower(strings.TrimSpace(output))
	if output != "" && output != "json" {
		return fmt.Errorf("不支持的 --output %q，仅支持 json（或留空使用默认格式）", output)
	}

	uploadImages, _ := cmd.Flags().GetBool("upload-images")
	colWidthRaw, _ := cmd.Flags().GetString("table-column-width")
	if _, _, errFlag := parseTableColumnWidthFlag(colWidthRaw); errFlag != nil {
		return errFlag
	}
	userAccessToken := resolveOptionalUserToken(cmd)
	revisionID, _ := cmd.Flags().GetInt("revision-id")
	if revisionID < -1 {
		return fmt.Errorf("--revision-id 必须 >= -1（-1 表示忽略校验或自动最新，当前输入: %d）", revisionID)
	}

	// 解析 Markdown 内容
	markdownContent, err := resolveMarkdownContent(markdownStr, markdownFile)
	if err != nil && mode != "delete_range" {
		return err
	}

	// 验证参数
	if err := validateContentUpdateParams(mode, markdownContent, selByTitle, selWithEllipsis); err != nil {
		return err
	}

	// 验证本地资源与列宽指令：flag 与内容注释两条入口都须 fail closed 并给出迁移提示
	if mode != "delete_range" {
		if err := validateNoLocalResources(uploadImages, markdownContent); err != nil {
			return err
		}
		if err := validateNoColumnWidthDirective(cmd.Flags().Changed("table-column-width"), colWidthRaw, markdownContent); err != nil {
			return err
		}
	} else if err := validateNoColumnWidthDirective(cmd.Flags().Changed("table-column-width"), colWidthRaw, ""); err != nil {
		return err
	}

	// 参数校验通过后再解析文档（wiki URL 需要一次 node_by_token 请求）
	documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
	if err != nil {
		return err
	}

	switch mode {
	case "append":
		return doAppend(documentID, markdownContent, output, userAccessToken, revisionID)
	case "overwrite":
		return doOverwrite(documentID, markdownContent, output, userAccessToken, revisionID)
	case "replace_range":
		return doReplaceRange(documentID, markdownContent, selByTitle, selWithEllipsis, output, userAccessToken, revisionID)
	case "replace_all":
		return doReplaceAll(documentID, markdownContent, selByTitle, selWithEllipsis, output, userAccessToken, revisionID)
	case "insert_before":
		return doInsertBefore(documentID, markdownContent, selByTitle, selWithEllipsis, output, userAccessToken, revisionID)
	case "insert_after":
		return doInsertAfter(documentID, markdownContent, selByTitle, selWithEllipsis, output, userAccessToken, revisionID)
	case "delete_range":
		return doDeleteRange(documentID, selByTitle, selWithEllipsis, output, userAccessToken, revisionID)
	}
	return nil // validateContentUpdateParams 已确保 mode 合法
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

// validateContentUpdateParams 验证参数组合是否合法
func validateContentUpdateParams(mode, markdown, selByTitle, selWithEllipsis string) error {
	validModes := map[string]bool{
		"append": true, "overwrite": true, "replace_range": true,
		"replace_all": true, "insert_before": true, "insert_after": true,
		"delete_range": true,
	}
	if !validModes[mode] {
		return fmt.Errorf("不支持的模式: %s", mode)
	}

	// 需要 markdown 内容的模式
	needsMarkdown := mode != "delete_range"
	if needsMarkdown && markdown == "" {
		return fmt.Errorf("模式 %s 需要 --markdown 或 --markdown-file", mode)
	}

	// 需要定位的模式
	needsSelection := mode != "append" && mode != "overwrite"
	if needsSelection {
		if selByTitle == "" && selWithEllipsis == "" {
			return fmt.Errorf("模式 %s 需要 --selection-by-title 或 --selection-with-ellipsis", mode)
		}
		if selByTitle != "" && selWithEllipsis != "" {
			return fmt.Errorf("--selection-by-title 和 --selection-with-ellipsis 不能同时使用")
		}
		if selWithEllipsis != "" {
			trimmed := strings.TrimSpace(selWithEllipsis)
			if trimmed == "..." {
				return fmt.Errorf("--selection-with-ellipsis 不能仅为省略号 '...'")
			}
			if strings.Contains(trimmed, "...") {
				parts := strings.SplitN(trimmed, "...", 2)
				startText := strings.TrimSpace(parts[0])
				endText := strings.TrimSpace(parts[1])
				if startText == "" || endText == "" {
					return fmt.Errorf("--selection-with-ellipsis 的起始和结束端点均不能为空，当前输入: %q", selWithEllipsis)
				}
			}
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
// 定位逻辑
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
// 格式："开头内容...结尾内容" 或不含 "..." 的精确匹配
func findByEllipsis(children []*larkdocx.Block, selector string) ([]blockRange, error) {
	// 检查是否包含非转义的 "..."
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

// findByStartEnd 查找从包含 startText 的块到包含 endText 的块的范围
func findByStartEnd(children []*larkdocx.Block, startText, endText string) ([]blockRange, error) {
	// 查找起始块
	startIdx := -1
	for i, block := range children {
		if strings.Contains(getBlockText(block), startText) {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return nil, fmt.Errorf("未找到包含起始文本的块: %q", startText)
	}

	// 从起始块之后查找结束块
	endIdx := -1
	for i := startIdx; i < len(children); i++ {
		if strings.Contains(getBlockText(children[i]), endText) {
			endIdx = i + 1 // 左闭右开
			// 不 break，取最后一个匹配
		}
	}
	if endIdx < 0 {
		return nil, fmt.Errorf("未找到包含结束文本的块: %q", endText)
	}

	return []blockRange{{startIndex: startIdx, endIndex: endIdx}}, nil
}

// findSelection 统一定位入口
func findSelection(children []*larkdocx.Block, selByTitle, selWithEllipsis string) ([]blockRange, error) {
	if selByTitle != "" {
		return findByTitle(children, selByTitle)
	}
	return findByEllipsis(children, selWithEllipsis)
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
		return fmt.Errorf("doc content-update 现采用官方原子安全更新协议，暂不支持 --table-column-width 自定义列宽；如需保留/自定义表格列宽，请改用 'feishu-cli doc import' 进行文档导入")
	}
	if colWidthCommentInMarkdownRe.MatchString(markdown) {
		return fmt.Errorf("检测到 Markdown 含 <!-- feishu-colwidth: ... --> 列宽指令；doc content-update 现采用官方原子安全更新协议，暂不支持自定义列宽；请移除该注释，或改用 'feishu-cli doc import' 进行文档导入")
	}
	return nil
}

// colWidthCommentInMarkdownRe 匹配任意行上的 <!-- feishu-colwidth: ... --> 指令
// （与 internal/converter 的 colWidthCommentRe 同义，此处按多行扫描整篇内容）。
var colWidthCommentInMarkdownRe = regexp.MustCompile(`(?m)^\s*<!--\s*feishu-colwidth\s*:[^>]*-->\s*$`)

// validateNoLocalResources 检查是否包含本地文件/图片资源；若有则 fail closed 并给出迁移提示
func validateNoLocalResources(uploadImages bool, markdown string) error {
	if uploadImages {
		return fmt.Errorf("doc content-update 现采用官方原子安全更新协议，暂不支持 --upload-images；如需上传本地图片，请改用图床/网络图片 URL，或使用 'feishu-cli doc import' 全量导入")
	}
	if containsLocalMarkdownResources(markdown) {
		return fmt.Errorf("检测到 Markdown 包含本地文件/图片资源；doc content-update 为保证数据安全不产生先删后写破坏窗口，暂不支持本地文件混合上传；请改用网络图片 URL，或使用 'feishu-cli doc import' 导入")
	}
	return nil
}

// ============================================================
// 7 种模式实现（对齐官方 PUT /open-apis/docs_ai/v1/documents/{id} 原子更新能力）
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

// doAppend 追加到文档末尾（对齐官方：docs_ai 将 append 转为 block_insert_after + block_id="-1"）
func doAppend(documentID, markdown string, output, userAccessToken string, revisionID int) error {
	body := map[string]any{
		"format":   "markdown",
		"command":  "block_insert_after",
		"block_id": "-1",
		"content":  markdown,
	}
	injectRevisionID(body, revisionID)
	data, err := client.UpdateDocContentAtomic(documentID, body, userAccessToken)
	if err != nil {
		return fmt.Errorf("追加内容失败: %w", err)
	}
	if output == "json" {
		return printJSON(data)
	}
	fmt.Println("文档内容追加成功！")
	return nil
}

// doOverwrite 完全覆盖文档内容（官方单操作原子 overwrite，彻底消除先删后写破坏窗口）
func doOverwrite(documentID, markdown string, output, userAccessToken string, revisionID int) error {
	body := map[string]any{
		"format":  "markdown",
		"command": "overwrite",
		"content": markdown,
	}
	injectRevisionID(body, revisionID)
	data, err := client.UpdateDocContentAtomic(documentID, body, userAccessToken)
	if err != nil {
		return fmt.Errorf("覆盖内容失败: %w", err)
	}
	if output == "json" {
		return printJSON(data)
	}
	fmt.Println("文档内容覆盖成功！")
	return nil
}

// doReplaceRange 按定位替换一段内容（映射为单操作原子 block_replace + start_block_id/end_block_id）
func doReplaceRange(documentID, markdown, selByTitle, selWithEllipsis string, output, userAccessToken string, revisionID int) error {
	children, err := getPageChildren(documentID, userAccessToken)
	if err != nil {
		return fmt.Errorf("获取文档内容失败: %w", err)
	}

	ranges, err := findSelection(children, selByTitle, selWithEllipsis)
	if err != nil {
		return err
	}

	r := ranges[0]
	if r.startIndex >= len(children) || r.endIndex <= r.startIndex {
		return fmt.Errorf("定位范围无效: [%d, %d)", r.startIndex, r.endIndex)
	}
	startBlockID := client.StringVal(children[r.startIndex].BlockId)
	endBlockID := client.StringVal(children[r.endIndex-1].BlockId)

	body := map[string]any{
		"format":         "markdown",
		"command":        "block_replace",
		"content":        markdown,
		"start_block_id": startBlockID,
		"end_block_id":   endBlockID,
	}
	injectRevisionID(body, revisionID)

	data, err := client.UpdateDocContentAtomic(documentID, body, userAccessToken)
	if err != nil {
		return fmt.Errorf("替换内容失败: %w", err)
	}
	if output == "json" {
		return printJSON(data)
	}
	fmt.Printf("已替换索引 %d 到 %d 的内容（块 %s 到 %s）\n", r.startIndex, r.endIndex, startBlockID, endBlockID)
	return nil
}

// doReplaceAll 全文查找替换所有匹配（倒序逐个原子 block_replace，部分失败时非零退出并报告已完成项）
func doReplaceAll(documentID, markdown, selByTitle, selWithEllipsis string, output, userAccessToken string, revisionID int) error {
	children, err := getPageChildren(documentID, userAccessToken)
	if err != nil {
		return fmt.Errorf("获取文档内容失败: %w", err)
	}

	ranges, err := findSelection(children, selByTitle, selWithEllipsis)
	if err != nil {
		return err
	}

	replaced := 0
	totalRanges := len(ranges)
	currentRevision := revisionID

	for i := totalRanges - 1; i >= 0; i-- {
		r := ranges[i]
		if r.startIndex >= len(children) || r.endIndex <= r.startIndex {
			continue
		}
		startBlockID := client.StringVal(children[r.startIndex].BlockId)
		endBlockID := client.StringVal(children[r.endIndex-1].BlockId)

		body := map[string]any{
			"format":         "markdown",
			"command":        "block_replace",
			"content":        markdown,
			"start_block_id": startBlockID,
			"end_block_id":   endBlockID,
		}
		injectRevisionID(body, currentRevision)

		data, err := client.UpdateDocContentAtomic(documentID, body, userAccessToken)
		if err != nil {
			return fmt.Errorf("全文替换未完全完成：共 %d 处匹配，已成功完成 %d 处，在第 %d 处替换失败（索引 %d 到 %d，块 %s 到 %s）: %w",
				totalRanges, replaced, totalRanges-i, r.startIndex, r.endIndex, startBlockID, endBlockID, err)
		}
		replaced++

		nextRev := extractRevisionID(data)
		if i > 0 {
			if nextRev <= 0 {
				return fmt.Errorf("全文替换中断：共 %d 处匹配，已成功完成 %d 处，但服务端未返回新的 revision_id；为防止并发数据破坏已停止后续未保护替换",
					totalRanges, replaced)
			}
			currentRevision = nextRev
		}
	}

	if output == "json" {
		return printJSON(map[string]any{
			"document_id":    documentID,
			"replaced_count": replaced,
		})
	}
	fmt.Printf("全文替换完成，共替换 %d 处\n", replaced)
	return nil
}

// doInsertBefore 在定位内容前插入（原子 block_insert_after）
func doInsertBefore(documentID, markdown, selByTitle, selWithEllipsis string, output, userAccessToken string, revisionID int) error {
	children, err := getPageChildren(documentID, userAccessToken)
	if err != nil {
		return fmt.Errorf("获取文档内容失败: %w", err)
	}

	ranges, err := findSelection(children, selByTitle, selWithEllipsis)
	if err != nil {
		return err
	}

	r := ranges[0]
	blockID := "0"
	if r.startIndex > 0 {
		blockID = client.StringVal(children[r.startIndex-1].BlockId)
	}
	body := map[string]any{
		"format":   "markdown",
		"command":  "block_insert_after",
		"block_id": blockID,
		"content":  markdown,
	}
	injectRevisionID(body, revisionID)
	data, err := client.UpdateDocContentAtomic(documentID, body, userAccessToken)
	if err != nil {
		return fmt.Errorf("插入内容失败: %w", err)
	}
	if output == "json" {
		return printJSON(data)
	}
	fmt.Printf("已在索引 %d 前插入内容\n", r.startIndex)
	return nil
}

// doInsertAfter 在定位内容后插入（原子 block_insert_after）
func doInsertAfter(documentID, markdown, selByTitle, selWithEllipsis string, output, userAccessToken string, revisionID int) error {
	children, err := getPageChildren(documentID, userAccessToken)
	if err != nil {
		return fmt.Errorf("获取文档内容失败: %w", err)
	}

	ranges, err := findSelection(children, selByTitle, selWithEllipsis)
	if err != nil {
		return err
	}

	r := ranges[0]
	targetBlockID := client.StringVal(children[r.endIndex-1].BlockId)
	body := map[string]any{
		"format":   "markdown",
		"command":  "block_insert_after",
		"block_id": targetBlockID,
		"content":  markdown,
	}
	injectRevisionID(body, revisionID)
	data, err := client.UpdateDocContentAtomic(documentID, body, userAccessToken)
	if err != nil {
		return fmt.Errorf("插入内容失败: %w", err)
	}
	if output == "json" {
		return printJSON(data)
	}
	fmt.Printf("已在索引 %d 后插入内容（块 %s）\n", r.endIndex-1, targetBlockID)
	return nil
}

// doDeleteRange 删除定位的内容（官方单操作原子 block_delete + start_block_id/end_block_id）
func doDeleteRange(documentID, selByTitle, selWithEllipsis string, output, userAccessToken string, revisionID int) error {
	children, err := getPageChildren(documentID, userAccessToken)
	if err != nil {
		return fmt.Errorf("获取文档内容失败: %w", err)
	}

	ranges, err := findSelection(children, selByTitle, selWithEllipsis)
	if err != nil {
		return err
	}

	r := ranges[0]
	if r.startIndex >= len(children) || r.endIndex <= r.startIndex {
		return fmt.Errorf("定位范围无效: [%d, %d)", r.startIndex, r.endIndex)
	}
	startBlockID := client.StringVal(children[r.startIndex].BlockId)
	endBlockID := client.StringVal(children[r.endIndex-1].BlockId)

	body := map[string]any{
		"command":        "block_delete",
		"start_block_id": startBlockID,
		"end_block_id":   endBlockID,
	}
	injectRevisionID(body, revisionID)

	data, err := client.UpdateDocContentAtomic(documentID, body, userAccessToken)
	if err != nil {
		return fmt.Errorf("删除内容失败: %w", err)
	}
	if output == "json" {
		return printJSON(data)
	}
	fmt.Printf("已删除索引 %d 到 %d 的块（块 %s 到 %s）\n", r.startIndex, r.endIndex, startBlockID, endBlockID)
	return nil
}
