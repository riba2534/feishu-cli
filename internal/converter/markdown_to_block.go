package converter

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// 最大递归深度常量（在 block_to_markdown.go 中定义）
// const maxRecursionDepth = 100

// MarkdownToBlock converts Markdown to Feishu blocks
type MarkdownToBlock struct {
	source   []byte
	options  ConvertOptions
	basePath string // base path for resolving relative image paths
}

// NewMarkdownToBlock creates a new converter
func NewMarkdownToBlock(source []byte, options ConvertOptions, basePath string) *MarkdownToBlock {
	return &MarkdownToBlock{
		source:   source,
		options:  options,
		basePath: basePath,
	}
}

// Convert converts Markdown to Feishu blocks
func (c *MarkdownToBlock) Convert() ([]*larkdocx.Block, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	reader := text.NewReader(c.source)
	doc := md.Parser().Parse(reader)

	var blocks []*larkdocx.Block
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
				blocks = append(blocks, block)
			}
			return ast.WalkSkipChildren, nil

		case *ast.Paragraph:
			block, err := c.convertParagraph(node)
			if err != nil {
				return ast.WalkStop, err
			}
			if block != nil {
				blocks = append(blocks, block)
			}
			return ast.WalkSkipChildren, nil

		case *ast.FencedCodeBlock:
			block, err := c.convertCodeBlock(node)
			if err != nil {
				return ast.WalkStop, err
			}
			if block != nil {
				blocks = append(blocks, block)
			}
			return ast.WalkSkipChildren, nil

		case *ast.List:
			listBlocks, err := c.convertList(node)
			if err != nil {
				return ast.WalkStop, err
			}
			blocks = append(blocks, listBlocks...)
			return ast.WalkSkipChildren, nil

		case *ast.Blockquote:
			block, err := c.convertBlockquote(node)
			if err != nil {
				return ast.WalkStop, err
			}
			if block != nil {
				blocks = append(blocks, block)
			}
			return ast.WalkSkipChildren, nil

		case *east.Table:
			block, err := c.convertTable(node)
			if err != nil {
				return ast.WalkStop, err
			}
			if block != nil {
				blocks = append(blocks, block)
			}
			return ast.WalkSkipChildren, nil

		case *ast.ThematicBreak:
			blocks = append(blocks, c.createDividerBlock())
			return ast.WalkContinue, nil
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		return nil, err
	}

	return blocks, nil
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

func (c *MarkdownToBlock) convertList(node *ast.List) ([]*larkdocx.Block, error) {
	var blocks []*larkdocx.Block
	isOrdered := node.IsOrdered()

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if listItem, ok := child.(*ast.ListItem); ok {
			block, err := c.convertListItem(listItem, isOrdered)
			if err != nil {
				return nil, err
			}
			if block != nil {
				blocks = append(blocks, block)
			}
		}
	}

	return blocks, nil
}

func (c *MarkdownToBlock) convertListItem(node *ast.ListItem, isOrdered bool) (*larkdocx.Block, error) {
	// Check for GFM task list checkbox
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		// Check if this is a paragraph or text block containing a TaskCheckBox
		if para, ok := child.(*ast.Paragraph); ok {
			if para.ChildCount() > 0 {
				if cb, ok := para.FirstChild().(*east.TaskCheckBox); ok {
					return c.convertGFMTaskListItem(node, cb.IsChecked)
				}
			}
		}
		if tb, ok := child.(*ast.TextBlock); ok {
			if tb.ChildCount() > 0 {
				if cb, ok := tb.FirstChild().(*east.TaskCheckBox); ok {
					return c.convertGFMTaskListItem(node, cb.IsChecked)
				}
				// Also check for raw text pattern
				if txt, ok := tb.FirstChild().(*ast.Text); ok {
					text := string(txt.Segment.Value(c.source))
					if strings.HasPrefix(text, "[ ] ") || strings.HasPrefix(text, "[x] ") || strings.HasPrefix(text, "[X] ") {
						return c.convertTaskListItem(node, text)
					}
				}
			}
		}
	}

	elements := c.extractTextElements(node)

	// 过滤空列表项（飞书 API 不接受空内容的列表块）
	if len(elements) == 0 || !hasNonEmptyContent(elements) {
		return nil, nil
	}

	if isOrdered {
		blockType := int(BlockTypeOrdered)
		return &larkdocx.Block{
			BlockType: &blockType,
			Ordered:   &larkdocx.Text{Elements: elements},
		}, nil
	}

	blockType := int(BlockTypeBullet)
	return &larkdocx.Block{
		BlockType: &blockType,
		Bullet:    &larkdocx.Text{Elements: elements},
	}, nil
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
			text := c.getNodeText(child)
			if text != "" {
				bold := child.Level == 2
				italic := child.Level == 1
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
						TextElementStyle: &larkdocx.TextElementStyle{
							Bold:   &bold,
							Italic: &italic,
						},
					},
				})
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
			text := c.getNodeText(child)
			if text != "" {
				strikethrough := true
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
						TextElementStyle: &larkdocx.TextElementStyle{
							Strikethrough: &strikethrough,
						},
					},
				})
			}
			return ast.WalkSkipChildren, nil

		case *ast.Link:
			text := c.getNodeText(child)
			url := string(child.Destination)
			if text != "" {
				// 飞书不支持页内锚点链接（以 # 开头），将其转换为普通文本
				if strings.HasPrefix(url, "#") {
					elements = append(elements, &larkdocx.TextElement{
						TextRun: &larkdocx.TextRun{
							Content: &text,
						},
					})
				} else {
					elements = append(elements, &larkdocx.TextElement{
						TextRun: &larkdocx.TextRun{
							Content: &text,
							TextElementStyle: &larkdocx.TextElementStyle{
								Link: &larkdocx.Link{
									Url: &url,
								},
							},
						},
					})
				}
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	return elements
}

func (c *MarkdownToBlock) convertBlockquote(node *ast.Blockquote) (*larkdocx.Block, error) {
	// Check for callout syntax [!TYPE]
	var calloutType string
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if para, ok := child.(*ast.Paragraph); ok {
			if txt, ok := para.FirstChild().(*ast.Text); ok {
				text := string(txt.Segment.Value(c.source))
				if match := regexp.MustCompile(`^\[!(\w+)\]`).FindStringSubmatch(text); match != nil {
					calloutType = match[1]
					break
				}
			}
		}
	}

	if calloutType != "" {
		return c.convertCallout(node, calloutType)
	}

	elements := c.extractTextElements(node)
	blockType := int(BlockTypeQuote)
	return &larkdocx.Block{
		BlockType: &blockType,
		Quote:     &larkdocx.Text{Elements: elements},
	}, nil
}

func (c *MarkdownToBlock) convertCallout(node *ast.Blockquote, calloutType string) (*larkdocx.Block, error) {
	// Map callout type to background color
	var bgColor int
	switch strings.ToUpper(calloutType) {
	case "WARNING", "CAUTION":
		bgColor = 2 // Red
	case "TIP":
		bgColor = 4 // Yellow
	case "SUCCESS":
		bgColor = 5 // Green
	case "INFO", "NOTE":
		bgColor = 6 // Blue
	default:
		bgColor = 6 // Default blue
	}

	blockType := int(BlockTypeCallout)
	return &larkdocx.Block{
		BlockType: &blockType,
		Callout: &larkdocx.Callout{
			BackgroundColor: &bgColor,
		},
	}, nil
}

func (c *MarkdownToBlock) convertImage(node *ast.Image) (*larkdocx.Block, error) {
	dest := string(node.Destination)

	// Check if it's a feishu media reference
	if strings.HasPrefix(dest, "feishu://media/") {
		token := strings.TrimPrefix(dest, "feishu://media/")
		blockType := int(BlockTypeImage)
		return &larkdocx.Block{
			BlockType: &blockType,
			Image: &larkdocx.Image{
				Token: &token,
			},
		}, nil
	}

	// Handle local file
	if c.options.UploadImages && !strings.HasPrefix(dest, "http://") && !strings.HasPrefix(dest, "https://") {
		// Resolve relative path
		imagePath := dest
		if !filepath.IsAbs(imagePath) {
			imagePath = filepath.Join(c.basePath, dest)
		}

		// Upload image
		token, err := client.UploadMedia(imagePath, "doc_image", c.options.DocumentID, "")
		if err != nil {
			// If upload fails, create placeholder
			return c.createImagePlaceholder(dest), nil
		}

		blockType := int(BlockTypeImage)
		return &larkdocx.Block{
			BlockType: &blockType,
			Image: &larkdocx.Image{
				Token: &token,
			},
		}, nil
	}

	// For URLs or when not uploading, create placeholder
	return c.createImagePlaceholder(dest), nil
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

func (c *MarkdownToBlock) convertTable(node *east.Table) (*larkdocx.Block, error) {
	// Count rows and columns
	var rows, cols int
	var cellContents []string
	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		if header, ok := row.(*east.TableHeader); ok {
			cols = row.ChildCount()
			rows++
			for cell := header.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if tc, ok := cell.(*east.TableCell); ok {
					cellContents = append(cellContents, c.getNodeText(tc))
				}
			}
		} else if tr, ok := row.(*east.TableRow); ok {
			if cols == 0 {
				cols = row.ChildCount()
			}
			rows++
			for cell := tr.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if tc, ok := cell.(*east.TableCell); ok {
					cellContents = append(cellContents, c.getNodeText(tc))
				}
			}
		}
	}

	if rows == 0 || cols == 0 {
		return nil, nil
	}

	// 飞书 API 对表格创建的限制：
	// 1. 通过 CreateBlock API 批量创建时，表格 cells 必须为空
	// 2. 表格内容无法在创建时一次性填充
	// 3. 将表格内容转换为带格式的文本，保留信息
	var textContent strings.Builder
	textContent.WriteString(fmt.Sprintf("┌ 表格 (%d×%d) ┐\n", rows, cols))
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			idx := i*cols + j
			if idx < len(cellContents) {
				content := cellContents[idx]
				if j > 0 {
					textContent.WriteString(" | ")
				}
				textContent.WriteString(content)
			}
		}
		textContent.WriteString("\n")
	}
	textContent.WriteString("└────────────┘")

	placeholder := textContent.String()
	blockType := int(BlockTypeText)
	return &larkdocx.Block{
		BlockType: &blockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: &placeholder,
					},
				},
			},
		},
	}, nil
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
			// Handle emphasis (italic/bold)
			text := c.getNodeText(child)
			if text != "" {
				bold := child.Level == 2
				italic := child.Level == 1
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
						TextElementStyle: &larkdocx.TextElementStyle{
							Bold:   &bold,
							Italic: &italic,
						},
					},
				})
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
			text := c.getNodeText(child)
			if text != "" {
				strikethrough := true
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
						TextElementStyle: &larkdocx.TextElementStyle{
							Strikethrough: &strikethrough,
						},
					},
				})
			}
			return ast.WalkSkipChildren, nil

		case *ast.Link:
			text := c.getNodeText(child)
			url := string(child.Destination)
			if text != "" {
				// 飞书不支持页内锚点链接（以 # 开头），将其转换为普通文本
				if strings.HasPrefix(url, "#") {
					elements = append(elements, &larkdocx.TextElement{
						TextRun: &larkdocx.TextRun{
							Content: &text,
						},
					})
				} else {
					elements = append(elements, &larkdocx.TextElement{
						TextRun: &larkdocx.TextRun{
							Content: &text,
							TextElementStyle: &larkdocx.TextElementStyle{
								Link: &larkdocx.Link{
									Url: &url,
								},
							},
						},
					})
				}
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

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
		default:
			buf.WriteString(c.getNodeTextWithDepth(child, depth+1))
		}
	}
	return buf.String()
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
