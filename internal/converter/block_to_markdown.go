package converter

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
)

// 最大递归深度，防止栈溢出
const maxRecursionDepth = 100

// BlockToMarkdown converts Feishu blocks to Markdown
type BlockToMarkdown struct {
	blocks        []*larkdocx.Block
	blockMap      map[string]*larkdocx.Block
	childBlockIDs map[string]bool // 子块 ID 集合，这些块不应独立处理
	options       ConvertOptions
	imageCount    int
	headingSeqs   []string                   // 标题自动编号状态，按深度索引（depth-1）
	userCache     map[string]MentionUserInfo // 用户 ID → 信息缓存
}

// NewBlockToMarkdown creates a new converter
func NewBlockToMarkdown(blocks []*larkdocx.Block, options ConvertOptions) *BlockToMarkdown {
	blockMap := make(map[string]*larkdocx.Block)
	childBlockIDs := make(map[string]bool) // 记录容器块的子块 ID

	// 第一遍：构建 blockMap
	for _, block := range blocks {
		if block.BlockId != nil {
			blockMap[*block.BlockId] = block
		}
	}

	// 递归收集子块 ID
	var collectChildren func(blockID string)
	collectChildren = func(blockID string) {
		block := blockMap[blockID]
		if block == nil {
			return
		}
		if block.Children != nil {
			for _, childID := range block.Children {
				childBlockIDs[childID] = true
				collectChildren(childID)
			}
		}
	}

	// 第二遍：只收集特定容器块的子块（跳过 Page 块）
	for _, block := range blocks {
		if block.BlockType == nil {
			continue
		}
		blockType := BlockType(*block.BlockType)

		// 只处理容器块的子块（不包括 Page）
		switch blockType {
		case BlockTypeTable:
			// Table 的 cells 是子块
			if block.Table != nil && block.Table.Cells != nil {
				for _, cellID := range block.Table.Cells {
					childBlockIDs[cellID] = true
					collectChildren(cellID)
				}
			}
		case BlockTypeCallout, BlockTypeQuoteContainer, BlockTypeGrid:
			// 这些容器块的子块需要跳过
			if block.Children != nil {
				for _, childID := range block.Children {
					childBlockIDs[childID] = true
					collectChildren(childID)
				}
			}
		case BlockTypeBullet, BlockTypeOrdered, BlockTypeTodo:
			// 嵌套列表：子块由父列表递归处理
			if block.Children != nil {
				for _, childID := range block.Children {
					childBlockIDs[childID] = true
					collectChildren(childID)
				}
			}
		case BlockTypeAddOns, BlockTypeSyncSource, BlockTypeSyncReference,
			BlockTypeAgenda, BlockTypeAgendaItem, BlockTypeAgendaItemContent,
			BlockTypeLinkPreview:
			// 容器块：子块由父块递归展开
			if block.Children != nil {
				for _, childID := range block.Children {
					childBlockIDs[childID] = true
					collectChildren(childID)
				}
			}
		}
	}

	return &BlockToMarkdown{
		blocks:        blocks,
		blockMap:      blockMap,
		childBlockIDs: childBlockIDs,
		options:       options,
	}
}

// NewBlockToMarkdownWithResolver 创建支持 @用户 展开的转换器
func NewBlockToMarkdownWithResolver(blocks []*larkdocx.Block, options ConvertOptions, resolver UserResolver) *BlockToMarkdown {
	c := NewBlockToMarkdown(blocks, options)
	if resolver != nil && options.ExpandMentions {
		userIDs := c.collectMentionUserIDs()
		if len(userIDs) > 0 {
			c.userCache = resolver.BatchResolve(userIDs)
		}
	}
	return c
}

// collectMentionUserIDs 扫描所有块的 TextElement，收集去重的 MentionUser ID
func (c *BlockToMarkdown) collectMentionUserIDs() []string {
	seen := make(map[string]bool)

	collectFromElements := func(elements []*larkdocx.TextElement) {
		for _, elem := range elements {
			if elem != nil && elem.MentionUser != nil && elem.MentionUser.UserId != nil {
				seen[*elem.MentionUser.UserId] = true
			}
		}
	}

	for _, block := range c.blocks {
		if block.BlockType == nil {
			continue
		}
		// 检查所有包含 TextElement 的块类型
		if block.Text != nil {
			collectFromElements(block.Text.Elements)
		}
		if block.Heading1 != nil {
			collectFromElements(block.Heading1.Elements)
		}
		if block.Heading2 != nil {
			collectFromElements(block.Heading2.Elements)
		}
		if block.Heading3 != nil {
			collectFromElements(block.Heading3.Elements)
		}
		if block.Heading4 != nil {
			collectFromElements(block.Heading4.Elements)
		}
		if block.Heading5 != nil {
			collectFromElements(block.Heading5.Elements)
		}
		if block.Heading6 != nil {
			collectFromElements(block.Heading6.Elements)
		}
		if block.Heading7 != nil {
			collectFromElements(block.Heading7.Elements)
		}
		if block.Heading8 != nil {
			collectFromElements(block.Heading8.Elements)
		}
		if block.Heading9 != nil {
			collectFromElements(block.Heading9.Elements)
		}
		if block.Bullet != nil {
			collectFromElements(block.Bullet.Elements)
		}
		if block.Ordered != nil {
			collectFromElements(block.Ordered.Elements)
		}
		if block.Quote != nil {
			collectFromElements(block.Quote.Elements)
		}
		if block.Todo != nil {
			collectFromElements(block.Todo.Elements)
		}
		if block.Code != nil {
			collectFromElements(block.Code.Elements)
		}
		if block.Equation != nil {
			collectFromElements(block.Equation.Elements)
		}
	}

	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	return result
}

// isListBlockType 判断是否为列表块类型
func isListBlockType(bt BlockType) bool {
	return bt == BlockTypeBullet || bt == BlockTypeOrdered || bt == BlockTypeTodo
}

// Convert converts all blocks to Markdown
func (c *BlockToMarkdown) Convert() (string, error) {
	var sb strings.Builder

	var prevBlockType BlockType

	// Process blocks in order
	for _, block := range c.blocks {
		if block.BlockType == nil {
			continue
		}

		// Skip page block
		if *block.BlockType == int(BlockTypePage) {
			continue
		}

		// 跳过子块（它们会通过父块处理）
		if block.BlockId != nil && c.childBlockIDs[*block.BlockId] {
			continue
		}

		currentBlockType := BlockType(*block.BlockType)

		// 列表类型切换时插入额外空行
		if prevBlockType != 0 {
			prevIsList := isListBlockType(prevBlockType)
			currIsList := isListBlockType(currentBlockType)
			if (prevIsList && !currIsList) || (!prevIsList && currIsList) {
				sb.WriteString("\n")
			} else if prevIsList && currIsList && prevBlockType != currentBlockType {
				// Bullet → Ordered 或 Ordered → Bullet 切换
				sb.WriteString("\n")
			}
		}

		md, err := c.convertBlock(block, 0)
		if err != nil {
			return "", err
		}
		if md != "" {
			sb.WriteString(md)
			sb.WriteString("\n")
			prevBlockType = currentBlockType
		}
	}

	output := strings.TrimRight(sb.String(), "\n") + "\n"

	// 规范化连续空行（最多保留一个空行，即两个换行符）
	reBlankLines := regexp.MustCompile(`\n{3,}`)
	output = reBlankLines.ReplaceAllString(output, "\n\n")

	return output, nil
}

func (c *BlockToMarkdown) convertBlock(block *larkdocx.Block, indent int) (string, error) {
	return c.convertBlockWithDepth(block, indent, 0)
}

func (c *BlockToMarkdown) convertBlockWithDepth(block *larkdocx.Block, indent int, depth int) (string, error) {
	// 递归深度检查，防止栈溢出
	if depth > maxRecursionDepth {
		return "<!-- 递归深度超限 -->\n", nil
	}

	if block.BlockType == nil {
		return "", nil
	}

	blockType := BlockType(*block.BlockType)

	switch blockType {
	case BlockTypePage:
		return "", nil
	case BlockTypeTableCell:
		// TableCell 块由 Table 块处理，跳过独立处理
		return "", nil
	case BlockTypeGridColumn:
		// GridColumn 块由 Grid 块处理，跳过独立处理
		return "", nil
	case BlockTypeAddOns:
		// 优先解析小组件内置内容（例如文本绘图的 Mermaid 源码）
		if block.AddOns != nil {
			text, err := c.convertAddOns(block)
			if err != nil {
				// JSON 解析失败时打日志，回退到子块展开
				fmt.Fprintf(os.Stderr, "警告: %v，回退到子块展开\n", err)
			} else if text != "" {
				return text, nil
			}
		}

		// 回退逻辑：如无可解析小组件内容或解析失败，按历史行为递归展开子块
		if block.Children != nil {
			var sb strings.Builder
			for _, childID := range block.Children {
				childBlock := c.blockMap[childID]
				if childBlock != nil {
					text, _ := c.convertBlockWithDepth(childBlock, indent, depth+1)
					sb.WriteString(text)
				}
			}
			return sb.String(), nil
		}
		return "", nil
	case BlockTypeText:
		return c.convertText(block)
	case BlockTypeHeading1, BlockTypeHeading2, BlockTypeHeading3,
		BlockTypeHeading4, BlockTypeHeading5, BlockTypeHeading6,
		BlockTypeHeading7, BlockTypeHeading8, BlockTypeHeading9:
		return c.convertHeading(block, blockType)
	case BlockTypeBullet:
		return c.convertBullet(block, indent, depth)
	case BlockTypeOrdered:
		return c.convertOrdered(block, indent, depth)
	case BlockTypeCode:
		return c.convertCode(block)
	case BlockTypeQuote:
		return c.convertQuote(block)
	case BlockTypeEquation:
		return c.convertEquation(block)
	case BlockTypeTodo:
		return c.convertTodoWithDepth(block, indent, depth)
	case BlockTypeDivider:
		return "---\n", nil
	case BlockTypeImage:
		return c.convertImage(block)
	case BlockTypeTable:
		return c.convertTable(block)
	case BlockTypeCallout:
		return c.convertCallout(block)
	case BlockTypeFile:
		return c.convertFile(block)
	case BlockTypeBitable:
		return c.convertBitable(block)
	case BlockTypeSheet:
		return c.convertSheet(block)
	case BlockTypeChatCard:
		return c.convertChatCard(block)
	case BlockTypeDiagram:
		return c.convertDiagram(block)
	case BlockTypeGrid:
		return c.convertGrid(block)
	case BlockTypeQuoteContainer:
		return c.convertQuoteContainer(block)
	case BlockTypeBoard:
		return c.convertBoard(block)
	case BlockTypeIframe:
		return c.convertIframe(block)
	case BlockTypeMindNote:
		return c.convertMindNote(block)
	case BlockTypeWikiCatalog:
		return c.convertWikiCatalog(block)
	case BlockTypeISV:
		return c.convertISV(block)
	case BlockTypeAgenda:
		// 议程块：分隔线 + 递归展开子块
		var sb strings.Builder
		sb.WriteString("---\n")
		if block.Children != nil {
			for _, childID := range block.Children {
				childBlock := c.blockMap[childID]
				if childBlock != nil {
					text, _ := c.convertBlockWithDepth(childBlock, indent, depth+1)
					sb.WriteString(text)
				}
			}
		}
		return sb.String(), nil
	case BlockTypeAgendaItem:
		// 议程项：容器块，递归展开子块
		if block.Children != nil {
			var sb strings.Builder
			for _, childID := range block.Children {
				childBlock := c.blockMap[childID]
				if childBlock != nil {
					text, _ := c.convertBlockWithDepth(childBlock, indent, depth+1)
					sb.WriteString(text)
				}
			}
			return sb.String(), nil
		}
		return "", nil
	case BlockTypeAgendaItemTitle:
		// 议程项标题：提取文本并加粗
		if block.Text != nil {
			text := c.convertTextElements(block.Text.Elements)
			return fmt.Sprintf("**%s**\n", text), nil
		}
		return "", nil
	case BlockTypeAgendaItemContent:
		// 议程项内容：容器块，递归展开子块
		if block.Children != nil {
			var sb strings.Builder
			for _, childID := range block.Children {
				childBlock := c.blockMap[childID]
				if childBlock != nil {
					text, _ := c.convertBlockWithDepth(childBlock, indent, depth+1)
					sb.WriteString(text)
				}
			}
			return sb.String(), nil
		}
		return "", nil
	case BlockTypeLinkPreview:
		// 链接预览：尝试展开子块，否则输出占位符
		if block.Children != nil && len(block.Children) > 0 {
			var sb strings.Builder
			for _, childID := range block.Children {
				childBlock := c.blockMap[childID]
				if childBlock != nil {
					text, _ := c.convertBlockWithDepth(childBlock, indent, depth+1)
					sb.WriteString(text)
				}
			}
			if sb.Len() > 0 {
				return sb.String(), nil
			}
		}
		return "[链接预览]\n", nil
	case BlockTypeSyncSource, BlockTypeSyncReference:
		// 同步块：容器块，递归展开子块（类似 AddOns）
		if block.Children != nil {
			var sb strings.Builder
			for _, childID := range block.Children {
				childBlock := c.blockMap[childID]
				if childBlock != nil {
					text, _ := c.convertBlockWithDepth(childBlock, indent, depth+1)
					sb.WriteString(text)
				}
			}
			return sb.String(), nil
		}
		return "", nil
	case BlockTypeWikiCatalogV2:
		return "[知识库目录 V2]\n", nil
	case BlockTypeAITemplate:
		return "<!-- AI 模板块 -->\n", nil
	default:
		typeName := BlockTypeName(blockType)
		return fmt.Sprintf("<!-- 不支持的块类型: %s (type=%d) -->\n", typeName, int(blockType)), nil
	}
}

func (c *BlockToMarkdown) convertText(block *larkdocx.Block) (string, error) {
	if block.Text == nil {
		return "", nil
	}
	return c.convertTextElements(block.Text.Elements) + "\n", nil
}

func (c *BlockToMarkdown) convertAddOns(block *larkdocx.Block) (string, error) {
	if block.AddOns == nil {
		return "", nil
	}

	typeID := ""
	if block.AddOns.ComponentTypeId != nil {
		typeID = *block.AddOns.ComponentTypeId
	}

	componentID := ""
	if block.AddOns.ComponentId != nil {
		componentID = *block.AddOns.ComponentId
	}

	if block.AddOns.Record != nil {
		var record struct {
			Data  string `json:"data"`
			View  string `json:"view"`
			Theme string `json:"theme"`
		}

		raw := strings.TrimSpace(*block.AddOns.Record)
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &record); err != nil {
				return fmt.Sprintf("[文本绘图组件 (%s) 源码解析失败]\n", componentID), fmt.Errorf("AddOns Record JSON 解析失败 (component: %s): %w", componentID, err)
			}
			source := strings.TrimSpace(record.Data)
			if source != "" {
				lang := detectAddonDiagramLanguage(record.View, source)
				return fmt.Sprintf("```%s\n%s\n```\n", lang, source), nil
			}
		}
	}

	if typeID == ISVTypeTextDrawing {
		return fmt.Sprintf("[文本绘图组件 (component: %s)]\n", componentID), nil
	}

	if componentID != "" {
		return fmt.Sprintf("[小组件 (type: %s, component: %s)]\n", typeID, componentID), nil
	}

	if typeID != "" {
		return fmt.Sprintf("[小组件 (type: %s)]\n", typeID), nil
	}

	return "", nil
}

func detectAddonDiagramLanguage(view string, source string) string {
	normalized := strings.ToLower(strings.TrimSpace(view))
	src := strings.TrimSpace(source)

	if normalized == "" {
		// 源码中 @startuml 可能出现在任意位置，用 Contains 匹配
		if strings.Contains(src, "@startuml") {
			return "plantuml"
		}
		return "mermaid"
	}

	if strings.Contains(normalized, "plantuml") {
		return "plantuml"
	}

	// 典型飞书绘图 view 多为 mermaid 风格语法，统一归一为 mermaid
	return "mermaid"
}

// getHeadingTextAndStyle 从 heading 块中提取 elements 和 TextStyle
func getHeadingTextAndStyle(block *larkdocx.Block, blockType BlockType) ([]*larkdocx.TextElement, *larkdocx.TextStyle) {
	switch blockType {
	case BlockTypeHeading1:
		if block.Heading1 != nil {
			return block.Heading1.Elements, block.Heading1.Style
		}
	case BlockTypeHeading2:
		if block.Heading2 != nil {
			return block.Heading2.Elements, block.Heading2.Style
		}
	case BlockTypeHeading3:
		if block.Heading3 != nil {
			return block.Heading3.Elements, block.Heading3.Style
		}
	case BlockTypeHeading4:
		if block.Heading4 != nil {
			return block.Heading4.Elements, block.Heading4.Style
		}
	case BlockTypeHeading5:
		if block.Heading5 != nil {
			return block.Heading5.Elements, block.Heading5.Style
		}
	case BlockTypeHeading6:
		if block.Heading6 != nil {
			return block.Heading6.Elements, block.Heading6.Style
		}
	case BlockTypeHeading7:
		if block.Heading7 != nil {
			return block.Heading7.Elements, block.Heading7.Style
		}
	case BlockTypeHeading8:
		if block.Heading8 != nil {
			return block.Heading8.Elements, block.Heading8.Style
		}
	case BlockTypeHeading9:
		if block.Heading9 != nil {
			return block.Heading9.Elements, block.Heading9.Style
		}
	}
	return nil, nil
}

func (c *BlockToMarkdown) convertHeading(block *larkdocx.Block, blockType BlockType) (string, error) {
	level := int(blockType) - int(BlockTypeHeading1) + 1
	elements, style := getHeadingTextAndStyle(block, blockType)

	// Heading 7-9 可选降级为粗体段落
	if level > 6 && c.options.DegradeDeepHeadings {
		text := c.convertTextElements(elements)
		return fmt.Sprintf("**%s**\n", text), nil
	}

	if level > 6 {
		level = 6
	}

	text := c.convertTextElements(elements)

	// 标题自动编号：根据 TextStyle.Sequence 字段
	seqPrefix := c.computeHeadingSeq(level, style)
	if seqPrefix != "" {
		text = seqPrefix + text
	}

	return fmt.Sprintf("%s %s\n", strings.Repeat("#", level), text), nil
}

// computeHeadingSeq 计算标题编号前缀
func (c *BlockToMarkdown) computeHeadingSeq(level int, style *larkdocx.TextStyle) string {
	if style == nil || style.Sequence == nil {
		return ""
	}
	seq := *style.Sequence
	if seq == "" {
		return ""
	}

	// 确保 headingSeqs 长度足够
	for len(c.headingSeqs) < level {
		c.headingSeqs = append(c.headingSeqs, "")
	}
	// 截断更深层级的编号（当遇到较浅标题时重置子标题编号）
	c.headingSeqs = c.headingSeqs[:level]

	if seq == "auto" {
		// 自动递增：取当前层级的上一个编号 +1
		prev := c.headingSeqs[level-1]
		if prev == "" {
			c.headingSeqs[level-1] = "1"
		} else {
			n := 0
			fmt.Sscanf(prev, "%d", &n)
			c.headingSeqs[level-1] = fmt.Sprintf("%d", n+1)
		}
	} else {
		// 手动指定编号
		c.headingSeqs[level-1] = seq
	}

	return c.headingSeqs[level-1] + ". "
}

func (c *BlockToMarkdown) convertBullet(block *larkdocx.Block, indent, depth int) (string, error) {
	if block.Bullet == nil {
		return "", nil
	}
	text := c.convertTextElements(block.Bullet.Elements)
	prefix := strings.Repeat("  ", indent)
	result := fmt.Sprintf("%s- %s\n", prefix, text)

	// 递归处理嵌套子列表
	if block.Children != nil {
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil {
				childMd, _ := c.convertBlockWithDepth(childBlock, indent+1, depth+1)
				result += childMd
			}
		}
	}
	return result, nil
}

func (c *BlockToMarkdown) convertOrdered(block *larkdocx.Block, indent, depth int) (string, error) {
	if block.Ordered == nil {
		return "", nil
	}
	text := c.convertTextElements(block.Ordered.Elements)
	prefix := strings.Repeat("  ", indent)

	seq := "1"
	if block.Ordered.Style != nil && block.Ordered.Style.Sequence != nil {
		s := *block.Ordered.Style.Sequence
		if s != "auto" && s != "" {
			seq = s
		}
	}
	result := fmt.Sprintf("%s%s. %s\n", prefix, seq, text)

	// 递归处理嵌套子列表
	if block.Children != nil {
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil {
				childMd, _ := c.convertBlockWithDepth(childBlock, indent+1, depth+1)
				result += childMd
			}
		}
	}
	return result, nil
}

func (c *BlockToMarkdown) convertCode(block *larkdocx.Block) (string, error) {
	if block.Code == nil {
		return "", nil
	}

	language := ""
	if block.Code.Style != nil && block.Code.Style.Language != nil {
		language = languageCodeToName(*block.Code.Style.Language)
	}

	// 代码块使用纯文本提取，不添加 Markdown 格式标记
	text := c.convertTextElementsRaw(block.Code.Elements)
	return fmt.Sprintf("```%s\n%s\n```\n", language, text), nil
}

func (c *BlockToMarkdown) convertQuote(block *larkdocx.Block) (string, error) {
	if block.Quote == nil {
		return "", nil
	}
	text := c.convertTextElements(block.Quote.Elements)
	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		result = append(result, "> "+line)
	}
	return strings.Join(result, "\n") + "\n", nil
}

func (c *BlockToMarkdown) convertEquation(block *larkdocx.Block) (string, error) {
	if block.Equation == nil {
		return "", nil
	}
	text := c.convertTextElements(block.Equation.Elements)
	return fmt.Sprintf("$$\n%s\n$$\n", text), nil
}

func (c *BlockToMarkdown) convertTodo(block *larkdocx.Block) (string, error) {
	return c.convertTodoWithDepth(block, 0, 0)
}

func (c *BlockToMarkdown) convertTodoWithDepth(block *larkdocx.Block, indent, depth int) (string, error) {
	if block.Todo == nil {
		return "", nil
	}

	checkbox := "[ ]"
	if block.Todo.Style != nil && block.Todo.Style.Done != nil && *block.Todo.Style.Done {
		checkbox = "[x]"
	}

	text := c.convertTextElements(block.Todo.Elements)
	prefix := strings.Repeat("  ", indent)
	result := fmt.Sprintf("%s- %s %s\n", prefix, checkbox, text)

	// 递归处理嵌套子项
	if block.Children != nil {
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil {
				childMd, _ := c.convertBlockWithDepth(childBlock, indent+1, depth+1)
				result += childMd
			}
		}
	}
	return result, nil
}

func (c *BlockToMarkdown) convertImage(block *larkdocx.Block) (string, error) {
	if block.Image == nil {
		return "", nil
	}

	token := ""
	if block.Image.Token != nil {
		token = *block.Image.Token
	}

	// 提取 alt 文本（从子块中获取）
	alt := "image"
	if block.Children != nil {
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil && childBlock.Text != nil {
				extracted := c.convertTextElementsRaw(childBlock.Text.Elements)
				if extracted != "" {
					alt = extracted
				}
				break
			}
		}
	}

	if token == "" {
		return fmt.Sprintf("![%s]()\n", alt), nil
	}

	if c.options.DownloadImages {
		// Download image to local assets directory
		c.imageCount++
		filename := fmt.Sprintf("image_%d.png", c.imageCount)

		if err := os.MkdirAll(c.options.AssetsDir, 0755); err != nil {
			return "", fmt.Errorf("创建资源目录失败: %w", err)
		}

		localPath := filepath.Join(c.options.AssetsDir, filename)

		dlOpts := client.DownloadMediaOptions{
			UserAccessToken: c.options.UserAccessToken,
			DocToken:        c.options.DocumentID,
		}

		// 方式一：获取临时 URL 后下载
		tmpURL, urlErr := client.GetMediaTempURL(token, dlOpts)
		if urlErr == nil {
			if dlErr := client.DownloadFromURL(tmpURL, localPath); dlErr == nil {
				return fmt.Sprintf("![%s](%s)\n", alt, localPath), nil
			} else if c.options.Debug {
				fmt.Fprintf(os.Stderr, "[Debug] 图片下载失败 (URL方式): %v\n", dlErr)
			}
		} else if c.options.Debug {
			fmt.Fprintf(os.Stderr, "[Debug] 获取图片临时URL失败: %v\n", urlErr)
		}

		// 方式二：SDK 直接下载
		if sdkErr := client.DownloadMedia(token, localPath, dlOpts); sdkErr == nil {
			return fmt.Sprintf("![%s](%s)\n", alt, localPath), nil
		} else if c.options.Debug {
			fmt.Fprintf(os.Stderr, "[Debug] 图片SDK下载失败: %v\n", sdkErr)
		}

		// 全部失败，输出 <image> 标签（可 roundtrip）
		return c.formatImageTag(block.Image), nil
	}

	// 不下载时输出 <image> 标签，包含完整属性（可 roundtrip）
	return c.formatImageTag(block.Image), nil
}

// formatImageTag 将 Image 块格式化为 <image .../> HTML 标签
func (c *BlockToMarkdown) formatImageTag(img *larkdocx.Image) string {
	token := ""
	if img.Token != nil {
		token = *img.Token
	}
	var attrs []string
	attrs = append(attrs, fmt.Sprintf("token=\"%s\"", token))
	if img.Width != nil && *img.Width > 0 {
		attrs = append(attrs, fmt.Sprintf("width=\"%d\"", *img.Width))
	}
	if img.Height != nil && *img.Height > 0 {
		attrs = append(attrs, fmt.Sprintf("height=\"%d\"", *img.Height))
	}
	if img.Align != nil && *img.Align > 0 {
		alignStr := ""
		switch *img.Align {
		case 1:
			alignStr = "left"
		case 2:
			alignStr = "center"
		case 3:
			alignStr = "right"
		}
		if alignStr != "" {
			attrs = append(attrs, fmt.Sprintf("align=\"%s\"", alignStr))
		}
	}
	return fmt.Sprintf("<image %s/>\n", strings.Join(attrs, " "))
}

func (c *BlockToMarkdown) convertTable(block *larkdocx.Block) (string, error) {
	if block.Table == nil || block.Table.Cells == nil {
		return "", nil
	}

	rows := 0
	cols := 0
	if block.Table.Property != nil {
		if block.Table.Property.RowSize != nil {
			rows = *block.Table.Property.RowSize
		}
		if block.Table.Property.ColumnSize != nil {
			cols = *block.Table.Property.ColumnSize
		}
	}

	if rows == 0 || cols == 0 {
		return "", nil
	}

	// Build table content
	cells := block.Table.Cells

	// 边界检查：验证 cells 数组长度是否匹配
	expectedCells := rows * cols
	if len(cells) < expectedCells {
		// cells 数量不足，返回空表格或部分表格
		rows = len(cells) / cols
		if rows == 0 {
			return "", nil
		}
	}

	var table [][]string

	for i := 0; i < rows; i++ {
		var row []string
		for j := 0; j < cols; j++ {
			idx := i*cols + j
			// 再次进行边界检查
			if idx >= len(cells) {
				row = append(row, "")
				continue
			}
			if cells[idx] != "" {
				// Get cell content from blockMap
				cellBlock := c.blockMap[cells[idx]]
				if cellBlock != nil && cellBlock.TableCell != nil {
					// Table cells contain child blocks
					// For simplicity, we just get text content
					row = append(row, c.getCellTextWithDepth(cellBlock, 0))
				} else {
					row = append(row, "")
				}
			} else {
				row = append(row, "")
			}
		}
		table = append(table, row)
	}

	// Build markdown table
	var sb strings.Builder

	// Header row
	if len(table) > 0 {
		sb.WriteString("| ")
		sb.WriteString(strings.Join(table[0], " | "))
		sb.WriteString(" |\n")

		// Separator
		sb.WriteString("|")
		for range table[0] {
			sb.WriteString(" --- |")
		}
		sb.WriteString("\n")

		// Data rows
		for i := 1; i < len(table); i++ {
			sb.WriteString("| ")
			sb.WriteString(strings.Join(table[i], " | "))
			sb.WriteString(" |\n")
		}
	}

	return sb.String(), nil
}

func (c *BlockToMarkdown) getCellTextWithDepth(block *larkdocx.Block, depth int) string {
	// 递归深度检查
	if depth > maxRecursionDepth {
		return "[递归深度超限]"
	}

	// Table cells may contain nested blocks
	if block.Children != nil {
		var texts []string
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil {
				text, _ := c.convertBlockWithDepth(childBlock, 0, depth+1)
				trimmed := strings.TrimSpace(text)
				if trimmed != "" {
					texts = append(texts, trimmed)
				}
			}
		}
		// 使用 <br> 连接多个块，保留单元格内的结构（标题、列表等）
		result := strings.Join(texts, "<br>")
		// 替换残留的换行符为 <br>，避免破坏 markdown 表格结构
		result = strings.ReplaceAll(result, "\n", "<br>")
		result = strings.ReplaceAll(result, "\r", "")
		// 转义管道符，避免破坏 Markdown 表格结构
		result = strings.ReplaceAll(result, `|`, `\|`)
		return result
	}
	return ""
}

func (c *BlockToMarkdown) convertCallout(block *larkdocx.Block) (string, error) {
	return c.convertCalloutWithDepth(block, 0)
}

func (c *BlockToMarkdown) convertCalloutWithDepth(block *larkdocx.Block, depth int) (string, error) {
	if block.Callout == nil {
		return "", nil
	}

	// 递归深度检查
	if depth > maxRecursionDepth {
		return "> [!NOTE]\n> [递归深度超限]\n", nil
	}

	// Determine callout type based on background color or emoji
	calloutType := "NOTE"
	if block.Callout.BackgroundColor != nil {
		switch *block.Callout.BackgroundColor {
		case 2: // Red
			calloutType = "WARNING"
		case 3: // Orange
			calloutType = "CAUTION"
		case 4: // Yellow
			calloutType = "TIP"
		case 5: // Green
			calloutType = "SUCCESS"
		case 6: // Blue
			calloutType = "NOTE"
		case 7: // Purple
			calloutType = "IMPORTANT"
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("> [!%s]\n", calloutType))

	// Process child blocks（跳过空文本子块）
	if block.Children != nil {
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil {
				text, _ := c.convertBlockWithDepth(childBlock, 0, depth+1)
				text = strings.TrimRight(text, "\n")
				if text == "" {
					continue
				}
				for _, line := range strings.Split(text, "\n") {
					sb.WriteString("> " + line + "\n")
				}
			}
		}
	}

	return sb.String(), nil
}

func (c *BlockToMarkdown) convertFile(block *larkdocx.Block) (string, error) {
	if block.File == nil {
		return "", nil
	}

	var attrs []string
	if block.File.Token != nil && *block.File.Token != "" {
		attrs = append(attrs, fmt.Sprintf("token=\"%s\"", *block.File.Token))
	}
	if block.File.Name != nil && *block.File.Name != "" {
		attrs = append(attrs, fmt.Sprintf("name=\"%s\"", *block.File.Name))
	}
	if block.File.ViewType != nil && *block.File.ViewType > 0 {
		attrs = append(attrs, fmt.Sprintf("view-type=\"%d\"", *block.File.ViewType))
	}

	if len(attrs) == 0 {
		return "", nil
	}

	return fmt.Sprintf("<file %s/>\n", strings.Join(attrs, " ")), nil
}

func (c *BlockToMarkdown) convertBitable(block *larkdocx.Block) (string, error) {
	if block.Bitable == nil {
		return "", nil
	}

	var attrs []string
	if block.Bitable.Token != nil && *block.Bitable.Token != "" {
		attrs = append(attrs, fmt.Sprintf("token=\"%s\"", *block.Bitable.Token))
	}

	viewStr := "table"
	if block.Bitable.ViewType != nil {
		switch *block.Bitable.ViewType {
		case 2:
			viewStr = "kanban"
		case 3:
			viewStr = "calendar"
		case 4:
			viewStr = "gallery"
		case 5:
			viewStr = "gantt"
		case 6:
			viewStr = "form"
		}
	}
	attrs = append(attrs, fmt.Sprintf("view=\"%s\"", viewStr))

	return fmt.Sprintf("<bitable %s/>\n", strings.Join(attrs, " ")), nil
}

func (c *BlockToMarkdown) convertSheet(block *larkdocx.Block) (string, error) {
	if block.Sheet == nil {
		return "", nil
	}

	var attrs []string
	if block.Sheet.Token != nil && *block.Sheet.Token != "" {
		token := *block.Sheet.Token
		if idx := strings.Index(token, "_"); idx != -1 {
			attrs = append(attrs, fmt.Sprintf("token=\"%s\"", token[:idx]))
			attrs = append(attrs, fmt.Sprintf("id=\"%s\"", token[idx+1:]))
		} else {
			attrs = append(attrs, fmt.Sprintf("token=\"%s\"", token))
		}
	}
	if block.Sheet.RowSize != nil && *block.Sheet.RowSize > 0 {
		attrs = append(attrs, fmt.Sprintf("rows=\"%d\"", *block.Sheet.RowSize))
	}
	if block.Sheet.ColumnSize != nil && *block.Sheet.ColumnSize > 0 {
		attrs = append(attrs, fmt.Sprintf("cols=\"%d\"", *block.Sheet.ColumnSize))
	}

	return fmt.Sprintf("<sheet %s/>\n", strings.Join(attrs, " ")), nil
}

func (c *BlockToMarkdown) convertChatCard(block *larkdocx.Block) (string, error) {
	if block.ChatCard == nil {
		return "", nil
	}

	chatID := ""
	if block.ChatCard.ChatId != nil {
		chatID = *block.ChatCard.ChatId
	}

	return fmt.Sprintf("[ChatCard: %s]\n", chatID), nil
}

func (c *BlockToMarkdown) convertDiagram(block *larkdocx.Block) (string, error) {
	if block.Diagram == nil {
		return "", nil
	}

	diagramType := 0
	if block.Diagram.DiagramType != nil {
		diagramType = *block.Diagram.DiagramType
	}

	// Map diagram type to name
	typeName := "Unknown"
	switch diagramType {
	case 1:
		typeName = "Flowchart"
	case 2:
		typeName = "UML"
	}

	// Note: Feishu API doesn't expose the actual Mermaid code in the block structure.
	// The Mermaid content is stored internally and rendered as an image.
	return fmt.Sprintf("```mermaid\n%% Feishu %s Diagram (type: %d)\n%% Note: Mermaid source code is not accessible via API\n```\n", typeName, diagramType), nil
}

func (c *BlockToMarkdown) convertBoard(block *larkdocx.Block) (string, error) {
	if block.Board == nil {
		return "", nil
	}

	token := ""
	if block.Board.Token != nil {
		token = *block.Board.Token
	}

	if token != "" && c.options.DownloadImages {
		c.imageCount++
		filename := fmt.Sprintf("board_%d.png", c.imageCount)

		if err := os.MkdirAll(c.options.AssetsDir, 0755); err != nil {
			return "", fmt.Errorf("创建资源目录失败: %w", err)
		}

		localPath := filepath.Join(c.options.AssetsDir, filename)

		if err := client.GetBoardImage(token, localPath, c.options.UserAccessToken); err == nil {
			return fmt.Sprintf("![画板](%s)\n", localPath), nil
		} else if c.options.Debug {
			fmt.Fprintf(os.Stderr, "[Debug] 画板下载失败: %v\n", err)
		}
	}

	// 下载未启用或下载失败，输出 <whiteboard> 标签（可 roundtrip）
	var attrs []string
	if token != "" {
		attrs = append(attrs, fmt.Sprintf("token=\"%s\"", token))
	}
	attrs = append(attrs, "type=\"blank\"")
	return fmt.Sprintf("<whiteboard %s/>\n", strings.Join(attrs, " ")), nil
}

func (c *BlockToMarkdown) convertIframe(block *larkdocx.Block) (string, error) {
	if block.Iframe == nil {
		return "", nil
	}

	iframeURL := ""
	if block.Iframe.Component != nil && block.Iframe.Component.Url != nil {
		iframeURL = *block.Iframe.Component.Url
	}
	if iframeURL == "" {
		return "", nil
	}

	return fmt.Sprintf(`<iframe src="%s" sandbox="allow-scripts allow-same-origin allow-presentation allow-forms allow-popups" allowfullscreen frameborder="0" style="width:100%%; min-height:400px;"></iframe>`+"\n", iframeURL), nil
}

func (c *BlockToMarkdown) convertMindNote(block *larkdocx.Block) (string, error) {
	if block.Mindnote == nil {
		return "", nil
	}

	token := ""
	if block.Mindnote.Token != nil {
		token = *block.Mindnote.Token
	}

	return fmt.Sprintf("[思维导图/MindNote](feishu://mindnote/%s)\n", token), nil
}

func (c *BlockToMarkdown) convertWikiCatalog(block *larkdocx.Block) (string, error) {
	// WikiCatalog (block_type=42) 是知识库目录块
	// 它本身不包含实际内容，子节点信息需要通过 wiki nodes API 获取
	return "[Wiki 目录 - 使用 'wiki nodes <space_id> --parent <node_token>' 获取子节点列表]\n", nil
}

func (c *BlockToMarkdown) convertISV(block *larkdocx.Block) (string, error) {
	if block.Isv == nil {
		return "", nil
	}

	typeID := ""
	if block.Isv.ComponentTypeId != nil {
		typeID = *block.Isv.ComponentTypeId
	}

	componentID := ""
	if block.Isv.ComponentId != nil {
		componentID = *block.Isv.ComponentId
	}

	switch typeID {
	case ISVTypeTextDrawing:
		// ISV (block_type=28) 的 TextDrawing 不含 Record 字段，Open API 不暴露源码。
		// 与 AddOns (block_type=40) 不同：AddOns 的 Record 字段包含完整的 Mermaid/PlantUML
		// 源码（JSON 格式 {"data":"...","view":"..."}），可直接提取还原为代码块。
		return fmt.Sprintf("```mermaid\n%%%% Feishu TextDrawing (component: %s)\n%%%% Mermaid source code is not accessible via Open API\n```\n", componentID), nil
	case ISVTypeTimeline:
		// Timeline 是时间线块，Open API 不暴露源数据
		return fmt.Sprintf("```mermaid\n%%%% Feishu Timeline (component: %s)\n%%%% Timeline data is not accessible via Open API\ntimeline\n    title Timeline\n```\n", componentID), nil
	default:
		// 其他 ISV 块类型
		return fmt.Sprintf("[ISV 应用块 (type: %s, id: %s)]\n", typeID, componentID), nil
	}
}

func (c *BlockToMarkdown) convertGrid(block *larkdocx.Block) (string, error) {
	return c.convertGridWithDepth(block, 0)
}

func (c *BlockToMarkdown) convertGridWithDepth(block *larkdocx.Block, depth int) (string, error) {
	if block.Grid == nil {
		return "", nil
	}

	// 递归深度检查
	if depth > maxRecursionDepth {
		return "<!-- Grid 递归深度超限 -->\n", nil
	}

	cols := 0
	if block.Grid.ColumnSize != nil {
		cols = *block.Grid.ColumnSize
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<grid cols=\"%d\">\n", cols))

	// Process grid columns
	if block.Children != nil {
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil && childBlock.BlockType != nil && *childBlock.BlockType == int(BlockTypeGridColumn) {
				text, _ := c.convertGridColumnWithDepth(childBlock, depth+1)
				sb.WriteString(text)
			}
		}
	}

	sb.WriteString("</grid>\n")
	return sb.String(), nil
}

func (c *BlockToMarkdown) convertGridColumnWithDepth(block *larkdocx.Block, depth int) (string, error) {
	if block.GridColumn == nil {
		return "", nil
	}

	// 递归深度检查
	if depth > maxRecursionDepth {
		return "<column>\n<!-- GridColumn 递归深度超限 -->\n</column>\n", nil
	}

	var sb strings.Builder
	sb.WriteString("<column>\n")

	// Process child blocks in the column
	if block.Children != nil {
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil {
				text, _ := c.convertBlockWithDepth(childBlock, 0, depth+1)
				sb.WriteString(text)
			}
		}
	}

	sb.WriteString("</column>\n")
	return sb.String(), nil
}

func (c *BlockToMarkdown) convertQuoteContainer(block *larkdocx.Block) (string, error) {
	return c.convertQuoteContainerWithDepth(block, 0)
}

func (c *BlockToMarkdown) convertQuoteContainerWithDepth(block *larkdocx.Block, depth int) (string, error) {
	if block.QuoteContainer == nil {
		return "", nil
	}

	// 递归深度检查
	if depth > maxRecursionDepth {
		return "> [递归深度超限]\n", nil
	}

	var sb strings.Builder

	// Process child blocks（保留空行，确保引用块内空行正确导出）
	if block.Children != nil {
		hasContent := false
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil {
				text, _ := c.convertBlockWithDepth(childBlock, 0, depth+1)
				text = strings.TrimRight(text, "\n")
				if text == "" {
					// 跳过开头的空行；已有内容后保留空行
					if hasContent {
						sb.WriteString(">\n")
					}
					continue
				}
				hasContent = true
				for _, line := range strings.Split(text, "\n") {
					sb.WriteString("> " + line + "\n")
				}
			}
		}
	}

	return sb.String(), nil
}

func (c *BlockToMarkdown) convertTextElements(elements []*larkdocx.TextElement) string {
	// 先合并相邻同样式元素
	elements = mergeAdjacentElements(elements)

	var result strings.Builder

	for _, elem := range elements {
		if elem == nil {
			continue
		}

		if elem.TextRun != nil {
			text := ""
			if elem.TextRun.Content != nil {
				text = *elem.TextRun.Content
			}

			// Apply styles in correct order (innermost to outermost)
			if elem.TextRun.TextElementStyle != nil {
				style := elem.TextRun.TextElementStyle

				// Handle inline code first (innermost) — 不转义内部文本
				if style.InlineCode != nil && *style.InlineCode {
					text = "`" + text + "`"
				} else {
					hasLink := style.Link != nil && style.Link.Url != nil

					// 对非链接、非行内代码的纯文本转义特殊字符
					if !hasLink {
						text = escapeMarkdown(text)
					}

					// Apply text formatting styles (not applicable to inline code)
					if style.Bold != nil && *style.Bold {
						text = "**" + text + "**"
					}
					if style.Italic != nil && *style.Italic {
						text = "*" + text + "*"
					}
					if style.Strikethrough != nil && *style.Strikethrough {
						text = "~~" + text + "~~"
					}
					if style.Underline != nil && *style.Underline {
						text = "<u>" + text + "</u>"
					}
				}

				// Handle link last (outermost)
				if style.Link != nil && style.Link.Url != nil {
					linkURL := *style.Link.Url
					// 解码完全 URL 编码的链接（如 https%3A%2F%2F...），提升可读性
					if decoded, err := url.QueryUnescape(linkURL); err == nil && decoded != linkURL {
						linkURL = decoded
					}
					// URL 中的括号编码，避免破坏 Markdown 链接语法
					linkURL = strings.ReplaceAll(linkURL, "(", "%28")
					linkURL = strings.ReplaceAll(linkURL, ")", "%29")
					text = fmt.Sprintf("[%s](%s)", text, linkURL)
				}

				// 高亮颜色导出（最外层包装）
				if c.options.Highlight {
					text = c.wrapHighlightSpan(style, text)
				}
			} else {
				// 无样式的纯文本也需要转义
				text = escapeMarkdown(text)
			}

			result.WriteString(text)
		}

		if elem.MentionUser != nil {
			userID := ""
			if elem.MentionUser.UserId != nil {
				userID = *elem.MentionUser.UserId
			}
			if c.options.ExpandMentions {
				// ExpandMentions=true: 展开为友好格式
				if info, ok := c.userCache[userID]; ok && info.Name != "" {
					if info.Email != "" {
						result.WriteString(fmt.Sprintf("[@%s](mailto:%s)", info.Name, info.Email))
					} else {
						result.WriteString(fmt.Sprintf("@%s", info.Name))
					}
				} else {
					// 用户不在缓存中时，输出 HTML 标签格式（可 roundtrip）
					result.WriteString(fmt.Sprintf("<mention-user id=\"%s\"/>", userID))
				}
			} else {
				// 默认输出 HTML 标签格式（可被导入端解析还原）
				result.WriteString(fmt.Sprintf("<mention-user id=\"%s\"/>", userID))
			}
		}

		if elem.MentionDoc != nil {
			title := ""
			if elem.MentionDoc.Title != nil {
				title = *elem.MentionDoc.Title
			}
			token := ""
			if elem.MentionDoc.Token != nil {
				token = *elem.MentionDoc.Token
			}
			docType := mapObjTypeToDocType(elem.MentionDoc.ObjType)
			// 输出 HTML 标签格式（可被导入端解析还原）
			result.WriteString(fmt.Sprintf("<mention-doc token=\"%s\" type=\"%s\">%s</mention-doc>", token, docType, title))
		}

		if elem.Equation != nil {
			content := ""
			if elem.Equation.Content != nil {
				content = *elem.Equation.Content
			}
			result.WriteString("$" + content + "$")
		}
	}

	return result.String()
}

// wrapHighlightSpan 将带颜色的文本包装为 HTML span 标签
func (c *BlockToMarkdown) wrapHighlightSpan(style *larkdocx.TextElementStyle, text string) string {
	if style == nil {
		return text
	}
	textColor := ""
	bgColor := ""
	if style.TextColor != nil && *style.TextColor != 0 {
		if c, ok := fontColorMap[*style.TextColor]; ok {
			textColor = c
		}
	}
	if style.BackgroundColor != nil && *style.BackgroundColor != 0 {
		if c, ok := fontBgColorMap[*style.BackgroundColor]; ok {
			bgColor = c
		}
	}
	if textColor == "" && bgColor == "" {
		return text
	}
	var styles []string
	if textColor != "" {
		styles = append(styles, "color: "+textColor)
	}
	if bgColor != "" {
		styles = append(styles, "background-color: "+bgColor)
	}
	return fmt.Sprintf(`<span style="%s">%s</span>`, strings.Join(styles, "; "), text)
}

// escapeMarkdown 转义 Markdown 特殊字符，避免纯文本被误解析
func escapeMarkdown(text string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`*`, `\*`,
		`_`, `\_`,
		`[`, `\[`,
		`]`, `\]`,
		`#`, `\#`,
		`~`, `\~`,
		"`", "\\`",
		`$`, `\$`,
		`|`, `\|`,
		`>`, `\>`,
	)
	return replacer.Replace(text)
}

// convertTextElementsRaw 将文本元素提取为纯文本，不添加任何 Markdown 标记
// 用于代码块等不应包含格式化标记的场景
func (c *BlockToMarkdown) convertTextElementsRaw(elements []*larkdocx.TextElement) string {
	var result strings.Builder

	for _, elem := range elements {
		if elem == nil {
			continue
		}
		if elem.TextRun != nil && elem.TextRun.Content != nil {
			result.WriteString(*elem.TextRun.Content)
		}
		if elem.MentionUser != nil && elem.MentionUser.UserId != nil {
			userID := *elem.MentionUser.UserId
			if c.options.ExpandMentions {
				if info, ok := c.userCache[userID]; ok && info.Name != "" {
					result.WriteString(info.Name)
				} else {
					result.WriteString(userID)
				}
			} else {
				result.WriteString(userID)
			}
		}
		if elem.MentionDoc != nil && elem.MentionDoc.Title != nil {
			result.WriteString(*elem.MentionDoc.Title)
		}
		if elem.Equation != nil && elem.Equation.Content != nil {
			result.WriteString(*elem.Equation.Content)
		}
	}

	return result.String()
}

// mergeAdjacentElements 合并相邻的同样式 TextElement，减少冗余标记
func mergeAdjacentElements(elements []*larkdocx.TextElement) []*larkdocx.TextElement {
	if len(elements) <= 1 {
		return elements
	}

	var merged []*larkdocx.TextElement
	for _, elem := range elements {
		if elem == nil || elem.TextRun == nil || elem.TextRun.Content == nil {
			merged = append(merged, elem)
			continue
		}

		if len(merged) > 0 {
			last := merged[len(merged)-1]
			if last != nil && last.TextRun != nil && last.TextRun.Content != nil && textStyleEqual(last.TextRun.TextElementStyle, elem.TextRun.TextElementStyle) {
				// 合并内容
				combined := *last.TextRun.Content + *elem.TextRun.Content
				last.TextRun.Content = &combined
				continue
			}
		}
		merged = append(merged, elem)
	}
	return merged
}

// textStyleEqual 判断两个 TextElementStyle 是否完全相同
func textStyleEqual(a, b *larkdocx.TextElementStyle) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return ptrBoolEq(a.Bold, b.Bold) &&
		ptrBoolEq(a.Italic, b.Italic) &&
		ptrBoolEq(a.Strikethrough, b.Strikethrough) &&
		ptrBoolEq(a.Underline, b.Underline) &&
		ptrBoolEq(a.InlineCode, b.InlineCode) &&
		linkEqual(a.Link, b.Link) &&
		ptrIntEq(a.TextColor, b.TextColor) &&
		ptrIntEq(a.BackgroundColor, b.BackgroundColor)
}

func ptrBoolEq(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrIntEq(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func linkEqual(a, b *larkdocx.Link) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Url == nil && b.Url == nil {
		return true
	}
	if a.Url == nil || b.Url == nil {
		return false
	}
	return *a.Url == *b.Url
}

// languageCodeToName converts Feishu language code to language name
func languageCodeToName(code int) string {
	languages := map[int]string{
		1:  "plaintext",
		2:  "abap",
		3:  "ada",
		4:  "apache",
		5:  "apex",
		6:  "assembly",
		7:  "bash",
		8:  "csharp",
		9:  "cpp",
		10: "c",
		11: "cobol",
		12: "css",
		13: "coffeescript",
		14: "d",
		15: "dart",
		16: "delphi",
		17: "django",
		18: "dockerfile",
		19: "erlang",
		20: "fortran",
		21: "foxpro",
		22: "go",
		23: "groovy",
		24: "html",
		25: "htmlbars",
		26: "http",
		27: "haskell",
		28: "json",
		29: "java",
		30: "javascript",
		31: "julia",
		32: "kotlin",
		33: "latex",
		34: "lisp",
		35: "lua",
		36: "matlab",
		37: "makefile",
		38: "markdown",
		39: "nginx",
		40: "objectivec",
		41: "openedgeabl",
		42: "php",
		43: "perl",
		44: "powershell",
		45: "prolog",
		46: "protobuf",
		47: "python",
		48: "r",
		49: "rpm",
		50: "ruby",
		51: "rust",
		52: "sas",
		53: "scss",
		54: "sql",
		55: "scala",
		56: "scheme",
		57: "shell",
		58: "swift",
		59: "thrift",
		60: "typescript",
		61: "vbscript",
		62: "verilog",
		63: "vhdl",
		64: "visualbasic",
		65: "xml",
		66: "yaml",
	}

	if name, ok := languages[code]; ok {
		return name
	}
	return "plaintext"
}
