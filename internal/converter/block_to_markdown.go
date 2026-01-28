package converter

import (
	"fmt"
	"os"
	"path/filepath"
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
		}
	}

	return &BlockToMarkdown{
		blocks:        blocks,
		blockMap:      blockMap,
		childBlockIDs: childBlockIDs,
		options:       options,
	}
}

// Convert converts all blocks to Markdown
func (c *BlockToMarkdown) Convert() (string, error) {
	var sb strings.Builder

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

		md, err := c.convertBlock(block, 0)
		if err != nil {
			return "", err
		}
		if md != "" {
			sb.WriteString(md)
			sb.WriteString("\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n") + "\n", nil
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
		// AddOns/SyncedBlock 块暂不支持，跳过
		return "", nil
	case BlockTypeText:
		return c.convertText(block)
	case BlockTypeHeading1, BlockTypeHeading2, BlockTypeHeading3,
		BlockTypeHeading4, BlockTypeHeading5, BlockTypeHeading6,
		BlockTypeHeading7, BlockTypeHeading8, BlockTypeHeading9:
		return c.convertHeading(block, blockType)
	case BlockTypeBullet:
		return c.convertBullet(block, indent)
	case BlockTypeOrdered:
		return c.convertOrdered(block, indent)
	case BlockTypeCode:
		return c.convertCode(block)
	case BlockTypeQuote:
		return c.convertQuote(block)
	case BlockTypeEquation:
		return c.convertEquation(block)
	case BlockTypeTodo:
		return c.convertTodo(block)
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
	default:
		// Unknown block type - output as comment
		return fmt.Sprintf("<!-- Unknown block type: %d -->\n", blockType), nil
	}
}

func (c *BlockToMarkdown) convertText(block *larkdocx.Block) (string, error) {
	if block.Text == nil {
		return "", nil
	}
	return c.convertTextElements(block.Text.Elements) + "\n", nil
}

func (c *BlockToMarkdown) convertHeading(block *larkdocx.Block, blockType BlockType) (string, error) {
	level := int(blockType) - int(BlockTypeHeading1) + 1
	if level > 6 {
		level = 6
	}

	var elements []*larkdocx.TextElement
	switch blockType {
	case BlockTypeHeading1:
		if block.Heading1 != nil {
			elements = block.Heading1.Elements
		}
	case BlockTypeHeading2:
		if block.Heading2 != nil {
			elements = block.Heading2.Elements
		}
	case BlockTypeHeading3:
		if block.Heading3 != nil {
			elements = block.Heading3.Elements
		}
	case BlockTypeHeading4:
		if block.Heading4 != nil {
			elements = block.Heading4.Elements
		}
	case BlockTypeHeading5:
		if block.Heading5 != nil {
			elements = block.Heading5.Elements
		}
	case BlockTypeHeading6:
		if block.Heading6 != nil {
			elements = block.Heading6.Elements
		}
	case BlockTypeHeading7:
		if block.Heading7 != nil {
			elements = block.Heading7.Elements
		}
	case BlockTypeHeading8:
		if block.Heading8 != nil {
			elements = block.Heading8.Elements
		}
	case BlockTypeHeading9:
		if block.Heading9 != nil {
			elements = block.Heading9.Elements
		}
	}

	text := c.convertTextElements(elements)
	return fmt.Sprintf("%s %s\n", strings.Repeat("#", level), text), nil
}

func (c *BlockToMarkdown) convertBullet(block *larkdocx.Block, indent int) (string, error) {
	if block.Bullet == nil {
		return "", nil
	}
	text := c.convertTextElements(block.Bullet.Elements)
	prefix := strings.Repeat("  ", indent)
	return fmt.Sprintf("%s- %s\n", prefix, text), nil
}

func (c *BlockToMarkdown) convertOrdered(block *larkdocx.Block, indent int) (string, error) {
	if block.Ordered == nil {
		return "", nil
	}
	text := c.convertTextElements(block.Ordered.Elements)
	prefix := strings.Repeat("  ", indent)
	return fmt.Sprintf("%s1. %s\n", prefix, text), nil
}

func (c *BlockToMarkdown) convertCode(block *larkdocx.Block) (string, error) {
	if block.Code == nil {
		return "", nil
	}

	language := ""
	if block.Code.Style != nil && block.Code.Style.Language != nil {
		language = languageCodeToName(*block.Code.Style.Language)
	}

	text := c.convertTextElements(block.Code.Elements)
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
	if block.Todo == nil {
		return "", nil
	}

	checkbox := "[ ]"
	if block.Todo.Style != nil && block.Todo.Style.Done != nil && *block.Todo.Style.Done {
		checkbox = "[x]"
	}

	text := c.convertTextElements(block.Todo.Elements)
	return fmt.Sprintf("- %s %s\n", checkbox, text), nil
}

func (c *BlockToMarkdown) convertImage(block *larkdocx.Block) (string, error) {
	if block.Image == nil {
		return "", nil
	}

	token := ""
	if block.Image.Token != nil {
		token = *block.Image.Token
	}

	if token == "" {
		return "![image]()\n", nil
	}

	if c.options.DownloadImages {
		// Download image to local assets directory
		c.imageCount++
		filename := fmt.Sprintf("image_%d.png", c.imageCount)

		if err := os.MkdirAll(c.options.AssetsDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create assets directory: %w", err)
		}

		localPath := filepath.Join(c.options.AssetsDir, filename)

		// Try to get temp URL and download
		url, err := client.GetMediaTempURL(token)
		if err == nil {
			if err := client.DownloadFromURL(url, localPath); err == nil {
				return fmt.Sprintf("![image](%s)\n", localPath), nil
			}
		}

		// Fallback: try direct download
		if err := client.DownloadMedia(token, localPath); err == nil {
			return fmt.Sprintf("![image](%s)\n", localPath), nil
		}

		// If all fails, use token reference
		return fmt.Sprintf("![image](feishu://media/%s)\n", token), nil
	}

	// Just use token reference
	return fmt.Sprintf("![image](feishu://media/%s)\n", token), nil
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
	// For now, we'll handle simple text content
	if block.Children != nil {
		var texts []string
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil {
				text, _ := c.convertBlockWithDepth(childBlock, 0, depth+1)
				texts = append(texts, strings.TrimSpace(text))
			}
		}
		return strings.Join(texts, " ")
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
			calloutType = "INFO"
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("> [!%s]\n", calloutType))

	// Process child blocks
	if block.Children != nil {
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil {
				text, _ := c.convertBlockWithDepth(childBlock, 0, depth+1)
				for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
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

	name := "file"
	if block.File.Name != nil {
		name = *block.File.Name
	}

	token := ""
	if block.File.Token != nil {
		token = *block.File.Token
	}

	return fmt.Sprintf("[%s](feishu://file/%s)\n", name, token), nil
}

func (c *BlockToMarkdown) convertBitable(block *larkdocx.Block) (string, error) {
	if block.Bitable == nil {
		return "", nil
	}

	token := ""
	if block.Bitable.Token != nil {
		token = *block.Bitable.Token
	}

	return fmt.Sprintf("[Bitable: %s](https://feishu.cn/base/%s)\n", token, token), nil
}

func (c *BlockToMarkdown) convertSheet(block *larkdocx.Block) (string, error) {
	if block.Sheet == nil {
		return "", nil
	}

	token := ""
	if block.Sheet.Token != nil {
		token = *block.Sheet.Token
	}

	return fmt.Sprintf("[Sheet: %s](https://feishu.cn/sheets/%s)\n", token, token), nil
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

	// Board (画板) can contain PlantUML or other diagrams
	// The content is accessed via a separate Board API
	return fmt.Sprintf("[画板/Whiteboard](feishu://board/%s)\n", token), nil
}

func (c *BlockToMarkdown) convertIframe(block *larkdocx.Block) (string, error) {
	if block.Iframe == nil {
		return "", nil
	}

	// Iframe blocks embed external content
	url := ""
	if block.Iframe.Component != nil && block.Iframe.Component.Url != nil {
		url = *block.Iframe.Component.Url
	}

	return fmt.Sprintf("[Embedded Content](%s)\n", url), nil
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

	var sb strings.Builder

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

	return sb.String(), nil
}

func (c *BlockToMarkdown) convertGridColumnWithDepth(block *larkdocx.Block, depth int) (string, error) {
	if block.GridColumn == nil {
		return "", nil
	}

	// 递归深度检查
	if depth > maxRecursionDepth {
		return "<!-- GridColumn 递归深度超限 -->\n", nil
	}

	var sb strings.Builder

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

	// Process child blocks
	if block.Children != nil {
		for _, childID := range block.Children {
			childBlock := c.blockMap[childID]
			if childBlock != nil {
				text, _ := c.convertBlockWithDepth(childBlock, 0, depth+1)
				for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
					sb.WriteString("> " + line + "\n")
				}
			}
		}
	}

	return sb.String(), nil
}

func (c *BlockToMarkdown) convertTextElements(elements []*larkdocx.TextElement) string {
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

				// Handle inline code first (innermost)
				if style.InlineCode != nil && *style.InlineCode {
					text = "`" + text + "`"
				} else {
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
					text = fmt.Sprintf("[%s](%s)", text, *style.Link.Url)
				}
			}

			result.WriteString(text)
		}

		if elem.MentionUser != nil {
			userID := ""
			if elem.MentionUser.UserId != nil {
				userID = *elem.MentionUser.UserId
			}
			result.WriteString(fmt.Sprintf("@[user:%s]", userID))
		}

		if elem.MentionDoc != nil {
			token := ""
			if elem.MentionDoc.Token != nil {
				token = *elem.MentionDoc.Token
			}
			title := ""
			if elem.MentionDoc.Title != nil {
				title = *elem.MentionDoc.Title
			}
			result.WriteString(fmt.Sprintf("[%s](feishu://doc/%s)", title, token))
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
