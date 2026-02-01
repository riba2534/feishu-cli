package converter

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// 最大递归深度常量（在 block_to_markdown.go 中定义）
// const maxRecursionDepth = 100

// 飞书 API 限制单个表格最多 9 行（包括表头）
const maxTableRows = 9

// 表格列宽配置（单位：像素）
const (
	minColumnWidth   = 80  // 最小列宽
	maxColumnWidth   = 400 // 最大列宽
	defaultDocWidth  = 700 // 飞书文档默认可用宽度
	charWidthChinese = 14  // 中文字符宽度
	charWidthEnglish = 8   // 英文/数字字符宽度
	columnPadding    = 16  // 列内边距
)

// calculateColumnWidths 根据单元格内容计算每列的宽度
func calculateColumnWidths(headerContents []string, dataRows [][]string, cols int) []int {
	if cols == 0 {
		return nil
	}

	// 计算每列的最大内容宽度
	maxWidths := make([]int, cols)

	// 计算单个字符串的显示宽度
	calcTextWidth := func(s string) int {
		width := 0
		for _, r := range s {
			if r > 127 { // 非 ASCII 字符（中文等）
				width += charWidthChinese
			} else {
				width += charWidthEnglish
			}
		}
		return width + columnPadding
	}

	// 处理表头
	for i, content := range headerContents {
		if i < cols {
			w := calcTextWidth(content)
			if w > maxWidths[i] {
				maxWidths[i] = w
			}
		}
	}

	// 处理数据行
	for _, row := range dataRows {
		for i, content := range row {
			if i < cols {
				w := calcTextWidth(content)
				if w > maxWidths[i] {
					maxWidths[i] = w
				}
			}
		}
	}

	// 应用最小/最大限制
	totalWidth := 0
	for i := range maxWidths {
		if maxWidths[i] < minColumnWidth {
			maxWidths[i] = minColumnWidth
		}
		if maxWidths[i] > maxColumnWidth {
			maxWidths[i] = maxColumnWidth
		}
		totalWidth += maxWidths[i]
	}

	// 如果总宽度小于文档宽度，按比例扩展
	if totalWidth < defaultDocWidth && cols > 0 {
		extra := (defaultDocWidth - totalWidth) / cols
		for i := range maxWidths {
			maxWidths[i] += extra
			if maxWidths[i] > maxColumnWidth {
				maxWidths[i] = maxColumnWidth
			}
		}
	}

	return maxWidths
}

// MarkdownToBlock converts Markdown to Feishu blocks
type MarkdownToBlock struct {
	source     []byte
	options    ConvertOptions
	basePath   string // base path for resolving relative image paths
	imageStats ImageStats
}

// NewMarkdownToBlock creates a new converter
func NewMarkdownToBlock(source []byte, options ConvertOptions, basePath string) *MarkdownToBlock {
	return &MarkdownToBlock{
		source:   source,
		options:  options,
		basePath: basePath,
	}
}

// BlockNode represents a block that may contain nested child blocks.
// Used to support hierarchical structures like nested lists in Feishu.
type BlockNode struct {
	Block    *larkdocx.Block
	Children []*BlockNode
}

// FlattenBlockNodes flattens a tree of BlockNodes into a flat list of blocks (depth-first)
func FlattenBlockNodes(nodes []*BlockNode) []*larkdocx.Block {
	var result []*larkdocx.Block
	for _, n := range nodes {
		if n == nil || n.Block == nil {
			continue
		}
		result = append(result, n.Block)
		if len(n.Children) > 0 {
			result = append(result, FlattenBlockNodes(n.Children)...)
		}
	}
	return result
}

// ConvertResult contains converted blocks and table data
type ConvertResult struct {
	BlockNodes []*BlockNode // 支持嵌套层级的块树
	TableDatas []*TableData // Table data in order of appearance, used for filling content
	ImageStats ImageStats   // 图片处理统计
}

// ConvertWithTableData converts Markdown to Feishu blocks and returns table data for content filling
func (c *MarkdownToBlock) ConvertWithTableData() (*ConvertResult, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	reader := text.NewReader(c.source)
	doc := md.Parser().Parse(reader)

	result := &ConvertResult{}
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			block, err := c.convertHeading(node)
			if err != nil {
				return ast.WalkStop, err
			}
			if block != nil {
				result.BlockNodes = append(result.BlockNodes, &BlockNode{Block: block})
			}
			return ast.WalkSkipChildren, nil

		case *ast.Paragraph:
			block, err := c.convertParagraph(node)
			if err != nil {
				return ast.WalkStop, err
			}
			if block != nil {
				result.BlockNodes = append(result.BlockNodes, &BlockNode{Block: block})
			}
			return ast.WalkSkipChildren, nil

		case *ast.FencedCodeBlock:
			block, err := c.convertCodeBlock(node)
			if err != nil {
				return ast.WalkStop, err
			}
			if block != nil {
				result.BlockNodes = append(result.BlockNodes, &BlockNode{Block: block})
			}
			return ast.WalkSkipChildren, nil

		case *ast.List:
			listNodes, err := c.convertList(node)
			if err != nil {
				return ast.WalkStop, err
			}
			result.BlockNodes = append(result.BlockNodes, listNodes...)
			return ast.WalkSkipChildren, nil

		case *ast.Blockquote:
			quoteNodes, err := c.convertBlockquote(node)
			if err != nil {
				return ast.WalkStop, err
			}
			result.BlockNodes = append(result.BlockNodes, quoteNodes...)
			return ast.WalkSkipChildren, nil

		case *east.Table:
			// 使用支持大表格拆分的方法
			tableResults := c.convertTableWithDataMultiple(node)
			for _, tableResult := range tableResults {
				if tableResult != nil {
					result.BlockNodes = append(result.BlockNodes, &BlockNode{Block: tableResult.Block})
					result.TableDatas = append(result.TableDatas, tableResult.TableData)
				}
			}
			return ast.WalkSkipChildren, nil

		case *ast.ThematicBreak:
			result.BlockNodes = append(result.BlockNodes, &BlockNode{Block: c.createDividerBlock()})
			return ast.WalkContinue, nil
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		return nil, err
	}

	result.ImageStats = c.imageStats
	return result, nil
}

// Convert converts Markdown to Feishu blocks (flat list, nesting info is lost).
// For nested list support, use ConvertWithTableData() which preserves BlockNode hierarchy.
func (c *MarkdownToBlock) Convert() ([]*larkdocx.Block, error) {
	result, err := c.ConvertWithTableData()
	if err != nil {
		return nil, err
	}
	return FlattenBlockNodes(result.BlockNodes), nil
}

func (c *MarkdownToBlock) convertHeading(node *ast.Heading) (*larkdocx.Block, error) {
	elements := c.extractTextElements(node)

	level := node.Level
	if level > 9 {
		level = 9
	}

	blockType := int(BlockTypeHeading1) + level - 1

	block := &larkdocx.Block{
		BlockType: &blockType,
	}

	headingText := &larkdocx.Text{Elements: elements}
	switch level {
	case 1:
		block.Heading1 = headingText
	case 2:
		block.Heading2 = headingText
	case 3:
		block.Heading3 = headingText
	case 4:
		block.Heading4 = headingText
	case 5:
		block.Heading5 = headingText
	case 6:
		block.Heading6 = headingText
	case 7:
		block.Heading7 = headingText
	case 8:
		block.Heading8 = headingText
	case 9:
		block.Heading9 = headingText
	}

	return block, nil
}

func (c *MarkdownToBlock) convertParagraph(node *ast.Paragraph) (*larkdocx.Block, error) {
	// Check if paragraph contains only an image
	if node.ChildCount() == 1 {
		if img, ok := node.FirstChild().(*ast.Image); ok {
			return c.convertImage(img)
		}
	}

	elements := c.extractTextElements(node)
	if len(elements) == 0 {
		return nil, nil
	}

	blockType := int(BlockTypeText)
	return &larkdocx.Block{
		BlockType: &blockType,
		Text:      &larkdocx.Text{Elements: elements},
	}, nil
}

func (c *MarkdownToBlock) convertCodeBlock(node *ast.FencedCodeBlock) (*larkdocx.Block, error) {
	lang := string(node.Language(c.source))
	langCode := languageNameToCode(lang)

	var content bytes.Buffer
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		content.Write(line.Value(c.source))
	}

	text := strings.TrimRight(content.String(), "\n")
	textContent := text

	blockType := int(BlockTypeCode)
	return &larkdocx.Block{
		BlockType: &blockType,
		Code: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: &textContent,
					},
				},
			},
			Style: &larkdocx.TextStyle{
				Language: &langCode,
			},
		},
	}, nil
}

func (c *MarkdownToBlock) convertList(node *ast.List) ([]*BlockNode, error) {
	var nodes []*BlockNode
	isOrdered := node.IsOrdered()

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if listItem, ok := child.(*ast.ListItem); ok {
			bn, err := c.convertListItem(listItem, isOrdered)
			if err != nil {
				return nil, err
			}
			if bn != nil {
				nodes = append(nodes, bn)
			}
		}
	}

	return nodes, nil
}

func (c *MarkdownToBlock) convertListItem(node *ast.ListItem, isOrdered bool) (*BlockNode, error) {
	// Check for GFM task list checkbox
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		// Check if this is a paragraph or text block containing a TaskCheckBox
		if para, ok := child.(*ast.Paragraph); ok {
			if para.ChildCount() > 0 {
				if cb, ok := para.FirstChild().(*east.TaskCheckBox); ok {
					block, err := c.convertGFMTaskListItem(node, cb.IsChecked)
					if err != nil {
						return nil, err
					}
					return &BlockNode{Block: block}, nil
				}
			}
		}
		if tb, ok := child.(*ast.TextBlock); ok {
			if tb.ChildCount() > 0 {
				if cb, ok := tb.FirstChild().(*east.TaskCheckBox); ok {
					block, err := c.convertGFMTaskListItem(node, cb.IsChecked)
					if err != nil {
						return nil, err
					}
					return &BlockNode{Block: block}, nil
				}
				// Also check for raw text pattern
				if txt, ok := tb.FirstChild().(*ast.Text); ok {
					text := string(txt.Segment.Value(c.source))
					if strings.HasPrefix(text, "[ ] ") || strings.HasPrefix(text, "[x] ") || strings.HasPrefix(text, "[X] ") {
						block, err := c.convertTaskListItem(node, text)
						if err != nil {
							return nil, err
						}
						return &BlockNode{Block: block}, nil
					}
				}
			}
		}
	}

	// 只提取直接子节点的文本（跳过嵌套的 ast.List）
	elements := c.extractListItemDirectElements(node)

	// 收集嵌套子列表
	var children []*BlockNode
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if nestedList, ok := child.(*ast.List); ok {
			childNodes, err := c.convertList(nestedList)
			if err != nil {
				return nil, err
			}
			children = append(children, childNodes...)
		}
	}

	// 过滤空列表项（飞书 API 不接受空内容的列表块）
	if (len(elements) == 0 || !hasNonEmptyContent(elements)) && len(children) == 0 {
		return nil, nil
	}

	// 如果没有直接文本但有子列表，创建空文本的父块
	if len(elements) == 0 || !hasNonEmptyContent(elements) {
		empty := ""
		elements = []*larkdocx.TextElement{{TextRun: &larkdocx.TextRun{Content: &empty}}}
	}

	var block *larkdocx.Block
	if isOrdered {
		blockType := int(BlockTypeOrdered)
		block = &larkdocx.Block{
			BlockType: &blockType,
			Ordered:   &larkdocx.Text{Elements: elements},
		}
	} else {
		blockType := int(BlockTypeBullet)
		block = &larkdocx.Block{
			BlockType: &blockType,
			Bullet:    &larkdocx.Text{Elements: elements},
		}
	}

	return &BlockNode{Block: block, Children: children}, nil
}

// extractListItemDirectElements 提取 ListItem 直接子节点的文本元素，
// 跳过嵌套的 ast.List 节点（嵌套列表作为 Children 单独处理）
func (c *MarkdownToBlock) extractListItemDirectElements(node *ast.ListItem) []*larkdocx.TextElement {
	var elements []*larkdocx.TextElement
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		// 跳过嵌套列表——它们会成为 BlockNode.Children
		if _, ok := child.(*ast.List); ok {
			continue
		}
		childElements := c.extractTextElements(child)
		elements = append(elements, childElements...)
	}
	return elements
}

func (c *MarkdownToBlock) convertTaskListItem(node *ast.ListItem, text string) (*larkdocx.Block, error) {
	done := strings.HasPrefix(text, "[x] ") || strings.HasPrefix(text, "[X] ")

	// Remove checkbox prefix from text
	text = strings.TrimPrefix(text, "[ ] ")
	text = strings.TrimPrefix(text, "[x] ")
	text = strings.TrimPrefix(text, "[X] ")

	blockType := int(BlockTypeTodo)
	return &larkdocx.Block{
		BlockType: &blockType,
		Todo: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				},
			},
			Style: &larkdocx.TextStyle{
				Done: &done,
			},
		},
	}, nil
}

func (c *MarkdownToBlock) convertGFMTaskListItem(node *ast.ListItem, isChecked bool) (*larkdocx.Block, error) {
	// Extract text elements, skipping the TaskCheckBox node
	elements := c.extractTextElementsSkipCheckbox(node)

	blockType := int(BlockTypeTodo)
	return &larkdocx.Block{
		BlockType: &blockType,
		Todo: &larkdocx.Text{
			Elements: elements,
			Style: &larkdocx.TextStyle{
				Done: &isChecked,
			},
		},
	}, nil
}

func (c *MarkdownToBlock) extractTextElementsSkipCheckbox(node ast.Node) []*larkdocx.TextElement {
	var elements []*larkdocx.TextElement

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// Skip TaskCheckBox nodes
		if _, ok := n.(*east.TaskCheckBox); ok {
			return ast.WalkSkipChildren, nil
		}

		switch child := n.(type) {
		case *ast.Text:
			text := string(child.Segment.Value(c.source))
			if text != "" {
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				})
			}

		case *ast.String:
			text := string(child.Value)
			if text != "" {
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				})
			}

		case *ast.Emphasis:
			childElems := c.extractChildElements(child)
			bold := child.Level == 2
			italic := child.Level == 1
			for _, elem := range childElems {
				applyTextStyle(elem, bold, italic, false)
				elements = append(elements, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.CodeSpan:
			text := c.getNodeText(child)
			if text != "" {
				inlineCode := true
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
						TextElementStyle: &larkdocx.TextElementStyle{
							InlineCode: &inlineCode,
						},
					},
				})
			}
			return ast.WalkSkipChildren, nil

		case *east.Strikethrough:
			childElems := c.extractChildElements(child)
			for _, elem := range childElems {
				applyTextStyle(elem, false, false, true)
				elements = append(elements, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.Link:
			text := c.getNodeText(child)
			url := string(child.Destination)
			if text != "" {
				elements = append(elements, createLinkElement(text, url))
			}
			return ast.WalkSkipChildren, nil

		case *ast.AutoLink:
			linkURL := string(child.URL(c.source))
			label := string(child.Label(c.source))
			if label == "" {
				label = linkURL
			}
			if linkURL != "" {
				elements = append(elements, createLinkElement(label, linkURL))
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	return elements
}

func (c *MarkdownToBlock) convertBlockquote(node *ast.Blockquote) ([]*BlockNode, error) {
	// Check for callout syntax [!TYPE]
	// goldmark 可能将 [!NOTE] 拆分为多个 Text 节点，需要合并后匹配
	var calloutType string
	calloutRe := regexp.MustCompile(`^\[!(\w+)\]`)
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if para, ok := child.(*ast.Paragraph); ok {
			// 合并段落首行所有文本节点
			var firstLineText string
			for inline := para.FirstChild(); inline != nil; inline = inline.NextSibling() {
				if txt, ok := inline.(*ast.Text); ok {
					firstLineText += string(txt.Segment.Value(c.source))
					if txt.SoftLineBreak() {
						break
					}
				} else {
					break
				}
			}
			if match := calloutRe.FindStringSubmatch(firstLineText); match != nil {
				calloutType = match[1]
				break
			}
		}
	}

	if calloutType != "" {
		calloutNode, err := c.convertCallout(node, calloutType)
		if err != nil {
			return nil, err
		}
		return []*BlockNode{calloutNode}, nil
	}

	// 使用 QuoteContainer 容器块，支持嵌套结构
	containerBlockType := int(BlockTypeQuoteContainer)
	containerBlock := &larkdocx.Block{
		BlockType:      &containerBlockType,
		QuoteContainer: &larkdocx.QuoteContainer{},
	}

	var children []*BlockNode
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Paragraph:
			// 按行拆分段落内容
			lines := c.extractQuoteLines(n)
			textBlockType := int(BlockTypeText)
			for _, line := range lines {
				if len(line) > 0 {
					children = append(children, &BlockNode{
						Block: &larkdocx.Block{
							BlockType: &textBlockType,
							Text:      &larkdocx.Text{Elements: line},
						},
					})
				}
			}
		case *ast.List:
			listNodes, err := c.convertList(n)
			if err == nil {
				children = append(children, listNodes...)
			}
		case *ast.FencedCodeBlock:
			block, err := c.convertCodeBlock(n)
			if err == nil && block != nil {
				children = append(children, &BlockNode{Block: block})
			}
		case *ast.Blockquote:
			// 嵌套引用
			nestedNodes, err := c.convertBlockquote(n)
			if err == nil {
				children = append(children, nestedNodes...)
			}
		default:
			// 其他节点，提取文本
			lines := c.extractQuoteLines(child)
			textBlockType := int(BlockTypeText)
			for _, line := range lines {
				if len(line) > 0 {
					children = append(children, &BlockNode{
						Block: &larkdocx.Block{
							BlockType: &textBlockType,
							Text:      &larkdocx.Text{Elements: line},
						},
					})
				}
			}
		}
	}

	// 如果没有子块，创建一个空文本子块
	if len(children) == 0 {
		textBlockType := int(BlockTypeText)
		children = append(children, &BlockNode{
			Block: &larkdocx.Block{
				BlockType: &textBlockType,
				Text:      &larkdocx.Text{Elements: []*larkdocx.TextElement{}},
			},
		})
	}

	return []*BlockNode{{Block: containerBlock, Children: children}}, nil
}

// extractQuoteLines 从 AST 节点提取文本元素，按 SoftLineBreak 拆分为多行
// 用于引用块（blockquote），将连续 > 行正确拆分为独立的 Quote 块
func (c *MarkdownToBlock) extractQuoteLines(node ast.Node) [][]*larkdocx.TextElement {
	var lines [][]*larkdocx.TextElement
	var currentLine []*larkdocx.TextElement

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch child := n.(type) {
		case *ast.Text:
			text := string(child.Segment.Value(c.source))
			if text != "" {
				currentLine = append(currentLine, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &text},
				})
			}
			if child.SoftLineBreak() {
				if len(currentLine) > 0 {
					lines = append(lines, currentLine)
					currentLine = nil
				}
			}

		case *ast.String:
			text := string(child.Value)
			if text != "" {
				currentLine = append(currentLine, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &text},
				})
			}

		case *ast.Emphasis:
			childElems := c.extractChildElements(child)
			bold := child.Level == 2
			italic := child.Level == 1
			for _, elem := range childElems {
				applyTextStyle(elem, bold, italic, false)
				currentLine = append(currentLine, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.CodeSpan:
			text := c.getNodeText(child)
			if text != "" {
				inlineCode := true
				currentLine = append(currentLine, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content:          &text,
						TextElementStyle: &larkdocx.TextElementStyle{InlineCode: &inlineCode},
					},
				})
			}
			return ast.WalkSkipChildren, nil

		case *east.Strikethrough:
			childElems := c.extractChildElements(child)
			for _, elem := range childElems {
				applyTextStyle(elem, false, false, true)
				currentLine = append(currentLine, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.Link:
			text := c.getNodeText(child)
			url := string(child.Destination)
			if text != "" {
				currentLine = append(currentLine, createLinkElement(text, url))
			}
			return ast.WalkSkipChildren, nil

		case *ast.AutoLink:
			linkURL := string(child.URL(c.source))
			label := string(child.Label(c.source))
			if label == "" {
				label = linkURL
			}
			if linkURL != "" {
				currentLine = append(currentLine, createLinkElement(label, linkURL))
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	// 添加最后一行
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}

	return lines
}

func (c *MarkdownToBlock) convertCallout(node *ast.Blockquote, calloutType string) (*BlockNode, error) {
	// Map callout type to background color
	var bgColor int
	switch strings.ToUpper(calloutType) {
	case "WARNING":
		bgColor = 2 // Red
	case "CAUTION":
		bgColor = 3 // Orange
	case "TIP":
		bgColor = 4 // Yellow
	case "SUCCESS":
		bgColor = 5 // Green
	case "INFO", "NOTE":
		bgColor = 6 // Blue
	case "IMPORTANT":
		bgColor = 7 // Purple
	default:
		bgColor = 6 // Default blue
	}

	blockType := int(BlockTypeCallout)
	calloutBlock := &larkdocx.Block{
		BlockType: &blockType,
		Callout: &larkdocx.Callout{
			BackgroundColor: &bgColor,
		},
	}

	// 提取 Callout 子块内容（跳过 [!TYPE] 首行）
	var children []*BlockNode
	firstPara := true
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if firstPara {
			if para, ok := child.(*ast.Paragraph); ok {
				firstPara = false
				// 跳过 [!TYPE] 标记，提取剩余内容
				elements := c.extractCalloutParaElements(para, calloutType)
				if len(elements) > 0 && hasNonEmptyContent(elements) {
					textBlockType := int(BlockTypeText)
					children = append(children, &BlockNode{
						Block: &larkdocx.Block{
							BlockType: &textBlockType,
							Text:      &larkdocx.Text{Elements: elements},
						},
					})
				}
				continue
			}
			firstPara = false
		}

		// 处理后续子节点
		switch n := child.(type) {
		case *ast.Paragraph:
			elements := c.extractTextElements(n)
			if len(elements) > 0 {
				textBlockType := int(BlockTypeText)
				children = append(children, &BlockNode{
					Block: &larkdocx.Block{
						BlockType: &textBlockType,
						Text:      &larkdocx.Text{Elements: elements},
					},
				})
			}
		case *ast.List:
			listNodes, err := c.convertList(n)
			if err == nil {
				children = append(children, listNodes...)
			}
		case *ast.FencedCodeBlock:
			block, err := c.convertCodeBlock(n)
			if err == nil && block != nil {
				children = append(children, &BlockNode{Block: block})
			}
		default:
			// 其他块级节点，提取文本
			elements := c.extractTextElements(child)
			if len(elements) > 0 && hasNonEmptyContent(elements) {
				textBlockType := int(BlockTypeText)
				children = append(children, &BlockNode{
					Block: &larkdocx.Block{
						BlockType: &textBlockType,
						Text:      &larkdocx.Text{Elements: elements},
					},
				})
			}
		}
	}

	return &BlockNode{Block: calloutBlock, Children: children}, nil
}

// extractCalloutParaElements 提取 Callout 首段落的文本元素，跳过 [!TYPE] 标记
func (c *MarkdownToBlock) extractCalloutParaElements(para *ast.Paragraph, calloutType string) []*larkdocx.TextElement {
	elements := c.extractTextElements(para)
	if len(elements) == 0 {
		return nil
	}

	// goldmark 可能将 [!NOTE] 拆分为多个 TextElement（如 "[" + "!NOTE" + "]"），
	// 需要先合并文本再匹配，然后按匹配长度移除对应的元素。
	prefix := "[!" + strings.ToUpper(calloutType) + "]"

	// 先尝试单元素匹配
	for i, elem := range elements {
		if elem.TextRun != nil && elem.TextRun.Content != nil {
			content := *elem.TextRun.Content
			if idx := strings.Index(content, prefix); idx >= 0 {
				remaining := strings.TrimSpace(content[idx+len(prefix):])
				if remaining == "" {
					elements = append(elements[:i], elements[i+1:]...)
				} else {
					elements[i].TextRun.Content = &remaining
				}
				return elements
			}
		}
	}

	// 单元素未找到，尝试跨元素合并匹配
	var concat string
	for _, elem := range elements {
		if elem.TextRun != nil && elem.TextRun.Content != nil {
			concat += *elem.TextRun.Content
		}
	}

	if idx := strings.Index(concat, prefix); idx >= 0 {
		// 确定需要跳过的字节数
		skipBytes := idx + len(prefix)
		consumed := 0
		cutIdx := 0
		for cutIdx < len(elements) {
			elem := elements[cutIdx]
			if elem.TextRun != nil && elem.TextRun.Content != nil {
				consumed += len(*elem.TextRun.Content)
			}
			cutIdx++
			if consumed >= skipBytes {
				break
			}
		}

		// cutIdx 之前的元素全部移除
		remaining := elements[cutIdx:]

		// 如果最后一个被消费的元素有多余内容，保留尾部
		if consumed > skipBytes {
			lastElem := elements[cutIdx-1]
			if lastElem.TextRun != nil && lastElem.TextRun.Content != nil {
				tail := (*lastElem.TextRun.Content)[len(*lastElem.TextRun.Content)-(consumed-skipBytes):]
				tail = strings.TrimSpace(tail)
				if tail != "" {
					tailElem := &larkdocx.TextElement{
						TextRun: &larkdocx.TextRun{Content: &tail, TextElementStyle: lastElem.TextRun.TextElementStyle},
					}
					remaining = append([]*larkdocx.TextElement{tailElem}, remaining...)
				}
			}
		}

		// 移除剩余元素中的前导空白和换行
		for len(remaining) > 0 {
			first := remaining[0]
			if first.TextRun != nil && first.TextRun.Content != nil {
				trimmed := strings.TrimLeft(*first.TextRun.Content, " \t\n\r")
				if trimmed == "" {
					remaining = remaining[1:]
					continue
				}
				first.TextRun.Content = &trimmed
			}
			break
		}

		return remaining
	}

	return elements
}

func (c *MarkdownToBlock) convertImage(node *ast.Image) (*larkdocx.Block, error) {
	dest := string(node.Destination)

	// feishu://media/ 是飞书内部媒体引用，token 绑定源文档不可跨文档复用。
	// 导出时应使用 --download-images 下载实际文件，导入时自动上传。
	if strings.HasPrefix(dest, "feishu://media/") {
		c.imageStats.Skipped++
		return c.createImagePlaceholder(dest), nil
	}

	if !c.options.UploadImages {
		c.imageStats.Skipped++
		return c.createImagePlaceholder(dest), nil
	}

	// 飞书 Open API 限制：DocX 文档无法通过 API 插入带图片的 Image 块。
	// Drive upload API 不支持 DocX block ID 作为 parent_node（返回 "parent node not exist"），
	// 且 replace_image 在 token 通过 documentID 上传时返回 "relation mismatch"。
	// 因此创建空 Image 块作为占位符，用户可在飞书网页端手动添加图片。
	// 参考：https://github.com/cso1z/Feishu-MCP/issues/32
	c.imageStats.Skipped++
	blockType := int(BlockTypeImage)
	return &larkdocx.Block{
		BlockType: &blockType,
		Image:     &larkdocx.Image{},
	}, nil
}

func (c *MarkdownToBlock) createImagePlaceholder(url string) *larkdocx.Block {
	text := fmt.Sprintf("[Image: %s]", url)
	blockType := int(BlockTypeText)
	return &larkdocx.Block{
		BlockType: &blockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				},
			},
		},
	}
}

func (c *MarkdownToBlock) createDividerBlock() *larkdocx.Block {
	blockType := int(BlockTypeDivider)
	return &larkdocx.Block{
		BlockType: &blockType,
		Divider:   &larkdocx.Divider{},
	}
}

// TableData stores table information for later content filling
type TableData struct {
	Rows         int
	Cols         int
	CellContents []string                  // 纯文本内容（兼容）
	CellElements [][]*larkdocx.TextElement // 富文本元素（保留链接等样式）
	HasHeader    bool
}

// ConvertTableResult contains both the block and the table data for content filling
type ConvertTableResult struct {
	Block     *larkdocx.Block
	TableData *TableData
}

func (c *MarkdownToBlock) convertTable(node *east.Table) (*larkdocx.Block, error) {
	result := c.convertTableWithData(node)
	if result == nil {
		return nil, nil
	}
	return result.Block, nil
}

func (c *MarkdownToBlock) convertTableWithData(node *east.Table) *ConvertTableResult {
	results := c.convertTableWithDataMultiple(node)
	if len(results) == 0 {
		return nil
	}
	return results[0]
}

// convertTableWithDataMultiple 将大表格拆分成多个小表格（每个最多 9 行）
func (c *MarkdownToBlock) convertTableWithDataMultiple(node *east.Table) []*ConvertTableResult {
	// Count rows and columns, collect all cell contents (plain text + rich elements)
	var cols int
	var headerContents []string
	var headerElements [][]*larkdocx.TextElement
	var dataRows [][]string                         // 纯文本，用于列宽计算
	var dataRowElements [][][]*larkdocx.TextElement // 富文本元素，保留链接等样式
	hasHeader := false

	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		if header, ok := row.(*east.TableHeader); ok {
			cols = row.ChildCount()
			hasHeader = true
			for cell := header.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if tc, ok := cell.(*east.TableCell); ok {
					headerContents = append(headerContents, c.getNodeText(tc))
					headerElements = append(headerElements, c.extractChildElements(tc))
				}
			}
		} else if tr, ok := row.(*east.TableRow); ok {
			if cols == 0 {
				cols = row.ChildCount()
			}
			var rowContents []string
			var rowElements [][]*larkdocx.TextElement
			for cell := tr.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if tc, ok := cell.(*east.TableCell); ok {
					rowContents = append(rowContents, c.getNodeText(tc))
					rowElements = append(rowElements, c.extractChildElements(tc))
				}
			}
			dataRows = append(dataRows, rowContents)
			dataRowElements = append(dataRowElements, rowElements)
		}
	}

	totalRows := len(dataRows)
	if hasHeader {
		totalRows++
	}
	if totalRows == 0 || cols == 0 {
		return nil
	}

	// 计算列宽（根据内容自动调整）
	columnWidths := calculateColumnWidths(headerContents, dataRows, cols)

	// 构建 TableData 的辅助函数
	buildTableData := func(rows, cols int, hasHeader bool, chunkDataRows [][]string, chunkDataElements [][][]*larkdocx.TextElement) *TableData {
		var cellContents []string
		var cellElements [][]*larkdocx.TextElement
		if hasHeader {
			cellContents = append(cellContents, headerContents...)
			cellElements = append(cellElements, headerElements...)
		}
		for _, row := range chunkDataRows {
			cellContents = append(cellContents, row...)
		}
		for _, row := range chunkDataElements {
			cellElements = append(cellElements, row...)
		}
		return &TableData{
			Rows:         rows,
			Cols:         cols,
			CellContents: cellContents,
			CellElements: cellElements,
			HasHeader:    hasHeader,
		}
	}

	// 如果表格不超过限制，直接返回单个表格
	if totalRows <= maxTableRows {
		blockType := int(BlockTypeTable)
		headerRow := hasHeader
		rows := totalRows
		block := &larkdocx.Block{
			BlockType: &blockType,
			Table: &larkdocx.Table{
				Property: &larkdocx.TableProperty{
					RowSize:     &rows,
					ColumnSize:  &cols,
					ColumnWidth: columnWidths,
					HeaderRow:   &headerRow,
				},
			},
		}

		return []*ConvertTableResult{{
			Block:     block,
			TableData: buildTableData(rows, cols, hasHeader, dataRows, dataRowElements),
		}}
	}

	// 需要拆分表格
	// 每个子表格最多有 maxTableRows 行，第一个表格包含表头+数据，后续表格复制表头
	var results []*ConvertTableResult
	maxDataRowsPerTable := maxTableRows
	if hasHeader {
		maxDataRowsPerTable = maxTableRows - 1 // 留一行给表头
	}

	for i := 0; i < len(dataRows); i += maxDataRowsPerTable {
		end := i + maxDataRowsPerTable
		if end > len(dataRows) {
			end = len(dataRows)
		}
		chunkDataRows := dataRows[i:end]
		chunkDataElements := dataRowElements[i:end]

		rows := len(chunkDataRows)
		if hasHeader {
			rows++
		}

		blockType := int(BlockTypeTable)
		headerRow := hasHeader
		block := &larkdocx.Block{
			BlockType: &blockType,
			Table: &larkdocx.Table{
				Property: &larkdocx.TableProperty{
					RowSize:     &rows,
					ColumnSize:  &cols,
					ColumnWidth: columnWidths,
					HeaderRow:   &headerRow,
				},
			},
		}

		results = append(results, &ConvertTableResult{
			Block:     block,
			TableData: buildTableData(rows, cols, hasHeader, chunkDataRows, chunkDataElements),
		})
	}

	return results
}

// inlineMathRegex 匹配行内公式 $...$，不匹配 $$ 或 \$
var inlineMathRegex = regexp.MustCompile(`(?:^|[^\\$])\$([^$\n]+?)\$`)

// isPlainTextRun 判断是否为完全无样式的纯文本元素（可安全合并）
func isPlainTextRun(elem *larkdocx.TextElement) bool {
	if elem == nil || elem.TextRun == nil || elem.TextRun.Content == nil {
		return false
	}
	if elem.TextRun.TextElementStyle == nil {
		return true
	}
	style := elem.TextRun.TextElementStyle
	if (style.Bold != nil && *style.Bold) ||
		(style.Italic != nil && *style.Italic) ||
		(style.Strikethrough != nil && *style.Strikethrough) ||
		(style.Underline != nil && *style.Underline) ||
		(style.InlineCode != nil && *style.InlineCode) ||
		style.Link != nil {
		return false
	}
	return true
}

// mergeAdjacentPlainTextRuns 合并相邻的无样式纯文本元素
// goldmark 的 GFM 扩展可能将连续文本拆分为多个 Text 节点，
// 合并后才能正确匹配跨节点的 $...$ 行内公式。
func mergeAdjacentPlainTextRuns(elements []*larkdocx.TextElement) []*larkdocx.TextElement {
	if len(elements) <= 1 {
		return elements
	}
	var merged []*larkdocx.TextElement
	for _, elem := range elements {
		if isPlainTextRun(elem) && len(merged) > 0 && isPlainTextRun(merged[len(merged)-1]) {
			combined := *merged[len(merged)-1].TextRun.Content + *elem.TextRun.Content
			merged[len(merged)-1].TextRun.Content = &combined
			continue
		}
		merged = append(merged, elem)
	}
	return merged
}

// splitInlineMath 将包含行内 $...$ 公式的文本元素拆分为文本+公式+文本
func splitInlineMath(elements []*larkdocx.TextElement) []*larkdocx.TextElement {
	// 先合并相邻纯文本元素，避免 goldmark 拆分导致 $...$ 被截断
	elements = mergeAdjacentPlainTextRuns(elements)

	var result []*larkdocx.TextElement
	for _, elem := range elements {
		if !isPlainTextRun(elem) {
			result = append(result, elem)
			continue
		}

		text := *elem.TextRun.Content
		// 查找所有 $...$ 匹配
		matches := inlineMathRegex.FindAllStringSubmatchIndex(text, -1)
		if len(matches) == 0 {
			result = append(result, elem)
			continue
		}

		pos := 0
		for _, match := range matches {
			// match[0]:match[1] 是完整匹配, match[2]:match[3] 是公式内容
			// 完整匹配可能包含前导字符（[^\\$] 消耗了一个字符）
			dollarStart := match[0]
			for dollarStart < match[1] && text[dollarStart] != '$' {
				dollarStart++
			}

			// 前导文本
			if dollarStart > pos {
				prefix := text[pos:dollarStart]
				result = append(result, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &prefix, TextElementStyle: elem.TextRun.TextElementStyle},
				})
			}

			// 公式内容
			formula := text[match[2]:match[3]]
			result = append(result, &larkdocx.TextElement{
				Equation: &larkdocx.Equation{Content: &formula},
			})

			pos = match[1]
		}

		// 剩余文本
		if pos < len(text) {
			remaining := text[pos:]
			result = append(result, &larkdocx.TextElement{
				TextRun: &larkdocx.TextRun{Content: &remaining, TextElementStyle: elem.TextRun.TextElementStyle},
			})
		}
	}
	return result
}

func (c *MarkdownToBlock) extractTextElements(node ast.Node) []*larkdocx.TextElement {
	var elements []*larkdocx.TextElement

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch child := n.(type) {
		case *ast.Text:
			text := string(child.Segment.Value(c.source))
			if text != "" {
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				})
			}

		case *ast.String:
			text := string(child.Value)
			if text != "" {
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				})
			}

		case *ast.Emphasis:
			// 递归提取子元素，保留内部链接等信息，然后叠加粗体/斜体样式
			childElems := c.extractChildElements(child)
			bold := child.Level == 2
			italic := child.Level == 1
			for _, elem := range childElems {
				applyTextStyle(elem, bold, italic, false)
				elements = append(elements, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.CodeSpan:
			text := c.getNodeText(child)
			if text != "" {
				inlineCode := true
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
						TextElementStyle: &larkdocx.TextElementStyle{
							InlineCode: &inlineCode,
						},
					},
				})
			}
			return ast.WalkSkipChildren, nil

		case *east.Strikethrough:
			// 递归提取子元素，保留内部链接等信息，然后叠加删除线样式
			childElems := c.extractChildElements(child)
			for _, elem := range childElems {
				applyTextStyle(elem, false, false, true)
				elements = append(elements, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.Link:
			text := c.getNodeText(child)
			url := string(child.Destination)
			if text != "" {
				elements = append(elements, createLinkElement(text, url))
			}
			return ast.WalkSkipChildren, nil

		case *ast.AutoLink:
			linkURL := string(child.URL(c.source))
			label := string(child.Label(c.source))
			if label == "" {
				label = linkURL
			}
			if linkURL != "" {
				elements = append(elements, createLinkElement(label, linkURL))
			}
			return ast.WalkSkipChildren, nil

		case *ast.Image:
			// 内联图片：网络 URL 转为可点击链接，本地路径转为文本占位符
			dest := string(child.Destination)
			alt := c.getNodeText(child)
			if alt == "" {
				alt = dest
			}
			if strings.HasPrefix(dest, "http://") || strings.HasPrefix(dest, "https://") {
				elements = append(elements, createLinkElement(fmt.Sprintf("[图片: %s]", alt), dest))
			} else {
				placeholder := fmt.Sprintf("[Image: %s]", dest)
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &placeholder},
				})
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	// 行内公式 $...$ 后处理
	elements = splitInlineMath(elements)

	return elements
}

func (c *MarkdownToBlock) getNodeText(node ast.Node) string {
	return c.getNodeTextWithDepth(node, 0)
}

func (c *MarkdownToBlock) getNodeTextWithDepth(node ast.Node, depth int) string {
	// 递归深度检查，防止栈溢出
	if depth > maxRecursionDepth {
		return "[递归深度超限]"
	}

	var buf bytes.Buffer
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			buf.Write(n.Segment.Value(c.source))
		case *ast.String:
			buf.Write(n.Value)
		case *ast.RawHTML:
			// 处理 <br> 标签为换行符
			var htmlBuf bytes.Buffer
			for i := 0; i < n.Segments.Len(); i++ {
				seg := n.Segments.At(i)
				htmlBuf.Write(c.source[seg.Start:seg.Stop])
			}
			raw := strings.TrimSpace(strings.ToLower(htmlBuf.String()))
			if raw == "<br>" || raw == "<br/>" || raw == "<br />" {
				buf.WriteString("\n")
			}
		default:
			buf.WriteString(c.getNodeTextWithDepth(child, depth+1))
		}
	}
	return buf.String()
}

// extractChildElements 递归提取子节点的 TextElement，保留链接等内联信息。
// 用于 Emphasis/Strikethrough 内部可能包含 Link 等节点的场景。
func (c *MarkdownToBlock) extractChildElements(node ast.Node) []*larkdocx.TextElement {
	var elements []*larkdocx.TextElement
	inUnderline := false // 跟踪 <u>...</u> 状态

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			text := string(n.Segment.Value(c.source))
			if text != "" {
				elem := &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &text},
				}
				if inUnderline {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *ast.String:
			text := string(n.Value)
			if text != "" {
				elem := &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &text},
				}
				if inUnderline {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *ast.Link:
			text := c.getNodeText(n)
			url := string(n.Destination)
			if text != "" {
				elem := createLinkElement(text, url)
				if inUnderline && elem.TextRun != nil {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *ast.CodeSpan:
			text := c.getNodeText(n)
			if text != "" {
				inlineCode := true
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content:          &text,
						TextElementStyle: &larkdocx.TextElementStyle{InlineCode: &inlineCode},
					},
				})
			}
		case *ast.Emphasis:
			childElems := c.extractChildElements(n)
			bold := n.Level == 2
			italic := n.Level == 1
			for _, elem := range childElems {
				applyTextStyle(elem, bold, italic, false)
				if inUnderline && elem.TextRun != nil {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *east.Strikethrough:
			childElems := c.extractChildElements(n)
			for _, elem := range childElems {
				applyTextStyle(elem, false, false, true)
				if inUnderline && elem.TextRun != nil {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *ast.AutoLink:
			linkURL := string(n.URL(c.source))
			label := string(n.Label(c.source))
			if label == "" {
				label = linkURL
			}
			if linkURL != "" {
				elem := createLinkElement(label, linkURL)
				if inUnderline && elem.TextRun != nil {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *ast.RawHTML:
			// 处理 HTML 标签
			var htmlBuf bytes.Buffer
			for i := 0; i < n.Segments.Len(); i++ {
				seg := n.Segments.At(i)
				htmlBuf.Write(c.source[seg.Start:seg.Stop])
			}
			raw := strings.TrimSpace(strings.ToLower(htmlBuf.String()))
			switch {
			case raw == "<br>" || raw == "<br/>" || raw == "<br />":
				newline := "\n"
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &newline},
				})
			case raw == "<u>":
				inUnderline = true
			case raw == "</u>":
				inUnderline = false
			}
		default:
			// 未知内联节点，递归提取子元素
			childElems := c.extractChildElements(child)
			if inUnderline {
				for _, elem := range childElems {
					if elem.TextRun != nil {
						underline := true
						if elem.TextRun.TextElementStyle == nil {
							elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
						}
						elem.TextRun.TextElementStyle.Underline = &underline
					}
				}
			}
			elements = append(elements, childElems...)
		}
	}
	return elements
}

// applyTextStyle 向 TextElement 叠加样式（不覆盖已有样式）
func applyTextStyle(elem *larkdocx.TextElement, bold, italic, strikethrough bool) {
	if elem == nil || elem.TextRun == nil {
		return
	}
	if elem.TextRun.TextElementStyle == nil {
		elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
	}
	s := elem.TextRun.TextElementStyle
	if bold {
		s.Bold = &bold
	}
	if italic {
		s.Italic = &italic
	}
	if strikethrough {
		s.Strikethrough = &strikethrough
	}
}

// normalizeURL 尝试标准化 URL
// 1. 将 feishu:// 内部协议转换为 https:// 链接（API 不接受 feishu:// 协议）
// 2. 解码 URL 编码的链接，例如 "https%3A%2F%2Fexample.com" → "https://example.com"
func normalizeURL(rawURL string) string {
	// feishu:// 内部协议转换为 HTTPS 链接
	if strings.HasPrefix(rawURL, "feishu://doc/") {
		return "https://feishu.cn/docx/" + strings.TrimPrefix(rawURL, "feishu://doc/")
	}
	if strings.HasPrefix(rawURL, "feishu://wiki/") {
		return "https://feishu.cn/wiki/" + strings.TrimPrefix(rawURL, "feishu://wiki/")
	}
	if strings.HasPrefix(rawURL, "feishu://") {
		// 其他 feishu:// 链接，尝试通用转换
		return "https://feishu.cn/" + strings.TrimPrefix(rawURL, "feishu://")
	}

	// URL 解码
	if decoded, err := url.QueryUnescape(rawURL); err == nil && decoded != rawURL {
		return decoded
	}
	return rawURL
}

// hasValidURLPrefix 检查 URL 是否以支持的协议开头
func hasValidURLPrefix(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// createLinkElement 创建链接 TextElement，自动过滤无效 URL 并解码 URL 编码的链接
func createLinkElement(text, rawURL string) *larkdocx.TextElement {
	u := normalizeURL(rawURL)
	if !hasValidURLPrefix(u) {
		return &larkdocx.TextElement{
			TextRun: &larkdocx.TextRun{Content: &text},
		}
	}
	return &larkdocx.TextElement{
		TextRun: &larkdocx.TextRun{
			Content: &text,
			TextElementStyle: &larkdocx.TextElementStyle{
				Link: &larkdocx.Link{Url: &u},
			},
		},
	}
}

// hasNonEmptyContent checks if text elements have non-empty content
func hasNonEmptyContent(elements []*larkdocx.TextElement) bool {
	for _, e := range elements {
		if e.TextRun != nil && e.TextRun.Content != nil {
			content := strings.TrimSpace(*e.TextRun.Content)
			if content != "" {
				return true
			}
		}
	}
	return false
}

// languageNameToCode converts language name to Feishu language code
func languageNameToCode(name string) int {
	languages := map[string]int{
		"plaintext":    1,
		"abap":         2,
		"ada":          3,
		"apache":       4,
		"apex":         5,
		"assembly":     6,
		"bash":         7,
		"sh":           7,
		"shell":        57,
		"csharp":       8,
		"cs":           8,
		"cpp":          9,
		"c++":          9,
		"c":            10,
		"cobol":        11,
		"css":          12,
		"coffeescript": 13,
		"coffee":       13,
		"d":            14,
		"dart":         15,
		"delphi":       16,
		"django":       17,
		"dockerfile":   18,
		"docker":       18,
		"erlang":       19,
		"fortran":      20,
		"foxpro":       21,
		"go":           22,
		"golang":       22,
		"groovy":       23,
		"html":         24,
		"htmlbars":     25,
		"http":         26,
		"haskell":      27,
		"json":         28,
		"java":         29,
		"javascript":   30,
		"js":           30,
		"julia":        31,
		"kotlin":       32,
		"kt":           32,
		"latex":        33,
		"tex":          33,
		"lisp":         34,
		"lua":          35,
		"matlab":       36,
		"makefile":     37,
		"make":         37,
		"markdown":     38,
		"md":           38,
		"nginx":        39,
		"objectivec":   40,
		"objc":         40,
		"openedgeabl":  41,
		"php":          42,
		"perl":         43,
		"powershell":   44,
		"ps1":          44,
		"prolog":       45,
		"protobuf":     46,
		"proto":        46,
		"python":       47,
		"py":           47,
		"r":            48,
		"rpm":          49,
		"ruby":         50,
		"rb":           50,
		"rust":         51,
		"rs":           51,
		"sas":          52,
		"scss":         53,
		"sql":          54,
		"scala":        55,
		"scheme":       56,
		"swift":        58,
		"thrift":       59,
		"typescript":   60,
		"ts":           60,
		"vbscript":     61,
		"verilog":      62,
		"vhdl":         63,
		"visualbasic":  64,
		"vb":           64,
		"xml":          65,
		"yaml":         66,
		"yml":          66,
	}

	if code, ok := languages[strings.ToLower(name)]; ok {
		return code
	}
	return 1 // plaintext
}
