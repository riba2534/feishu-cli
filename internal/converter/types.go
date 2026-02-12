package converter

import "fmt"

// BlockType represents Feishu block types
type BlockType int

const (
	BlockTypePage              BlockType = 1
	BlockTypeText              BlockType = 2
	BlockTypeHeading1          BlockType = 3
	BlockTypeHeading2          BlockType = 4
	BlockTypeHeading3          BlockType = 5
	BlockTypeHeading4          BlockType = 6
	BlockTypeHeading5          BlockType = 7
	BlockTypeHeading6          BlockType = 8
	BlockTypeHeading7          BlockType = 9
	BlockTypeHeading8          BlockType = 10
	BlockTypeHeading9          BlockType = 11
	BlockTypeBullet            BlockType = 12
	BlockTypeOrdered           BlockType = 13
	BlockTypeCode              BlockType = 14
	BlockTypeQuote             BlockType = 15
	BlockTypeEquation          BlockType = 16
	BlockTypeTodo              BlockType = 17
	BlockTypeBitable           BlockType = 18
	BlockTypeCallout           BlockType = 19
	BlockTypeChatCard          BlockType = 20
	BlockTypeDiagram           BlockType = 21 // Mermaid/UML 绘图块
	BlockTypeDivider           BlockType = 22
	BlockTypeFile              BlockType = 23
	BlockTypeGrid              BlockType = 24
	BlockTypeGridColumn        BlockType = 25
	BlockTypeIframe            BlockType = 26
	BlockTypeImage             BlockType = 27
	BlockTypeISV               BlockType = 28
	BlockTypeMindNote          BlockType = 29
	BlockTypeSheet             BlockType = 30
	BlockTypeTable             BlockType = 31
	BlockTypeTableCell         BlockType = 32
	BlockTypeView              BlockType = 33
	BlockTypeQuoteContainer    BlockType = 34
	BlockTypeTask              BlockType = 35
	BlockTypeOKR               BlockType = 36
	BlockTypeOKRObjective      BlockType = 37
	BlockTypeOKRKeyResult      BlockType = 38
	BlockTypeOKRProgress       BlockType = 39
	BlockTypeAddOns            BlockType = 40
	BlockTypeJiraIssue         BlockType = 41
	BlockTypeWikiCatalog       BlockType = 42
	BlockTypeBoard             BlockType = 43 // 画板块
	BlockTypeAgenda            BlockType = 44 // 议程块
	BlockTypeAgendaItem        BlockType = 45 // 议程项
	BlockTypeAgendaItemTitle   BlockType = 46 // 议程项标题
	BlockTypeAgendaItemContent BlockType = 47 // 议程项内容
	BlockTypeLinkPreview       BlockType = 48 // 链接预览
	BlockTypeSyncSource        BlockType = 49 // 同步源块
	BlockTypeSyncReference     BlockType = 50 // 同步引用块
	BlockTypeWikiCatalogV2     BlockType = 51 // 知识库目录 V2
	BlockTypeAITemplate        BlockType = 52 // AI 模板块
	BlockTypeUndefined         BlockType = 999
)

// DiagramType represents Feishu diagram types
type DiagramType int

const (
	DiagramTypeFlowchart DiagramType = 1 // 流程图
	DiagramTypeUML       DiagramType = 2 // UML 图
)

// TextStyle represents text styling
type TextStyle struct {
	Bold          bool
	Italic        bool
	Strikethrough bool
	Underline     bool
	InlineCode    bool
	Link          *LinkInfo
}

// LinkInfo represents link information
type LinkInfo struct {
	URL string
}

// ImageInfo holds image information for export
type ImageInfo struct {
	Token     string
	URL       string
	LocalPath string
}

// ConvertOptions holds conversion options
type ConvertOptions struct {
	DownloadImages      bool
	AssetsDir           string
	UploadImages        bool
	DocumentID          string
	DegradeDeepHeadings bool // 为 true 时，Heading 7-9 输出为粗体段落而非 ######
	FrontMatter         bool // 为 true 时，导出时添加 YAML front matter
	Highlight           bool // 为 true 时，导出文本颜色和背景色为 HTML span
	ExpandMentions      bool // 导出时展开 @用户为友好格式（默认 false，CLI 默认 true）
}

// ImageStats 记录图片处理统计
type ImageStats struct {
	Skipped int // 跳过（API 不支持插入图片）数
}

// MentionUserInfo 保存 @用户 的解析信息
type MentionUserInfo struct {
	Name  string
	Email string
}

// UserResolver 定义用户信息批量解析接口（解耦 converter 与 client 依赖）
type UserResolver interface {
	BatchResolve(userIDs []string) map[string]MentionUserInfo
}

// blockTypeName 映射所有已知块类型到可读名称
var blockTypeName = map[BlockType]string{
	BlockTypePage:              "Page",
	BlockTypeText:              "Text",
	BlockTypeHeading1:          "Heading1",
	BlockTypeHeading2:          "Heading2",
	BlockTypeHeading3:          "Heading3",
	BlockTypeHeading4:          "Heading4",
	BlockTypeHeading5:          "Heading5",
	BlockTypeHeading6:          "Heading6",
	BlockTypeHeading7:          "Heading7",
	BlockTypeHeading8:          "Heading8",
	BlockTypeHeading9:          "Heading9",
	BlockTypeBullet:            "Bullet",
	BlockTypeOrdered:           "Ordered",
	BlockTypeCode:              "Code",
	BlockTypeQuote:             "Quote",
	BlockTypeEquation:          "Equation",
	BlockTypeTodo:              "Todo",
	BlockTypeBitable:           "Bitable",
	BlockTypeCallout:           "Callout",
	BlockTypeChatCard:          "ChatCard",
	BlockTypeDiagram:           "Diagram",
	BlockTypeDivider:           "Divider",
	BlockTypeFile:              "File",
	BlockTypeGrid:              "Grid",
	BlockTypeGridColumn:        "GridColumn",
	BlockTypeIframe:            "Iframe",
	BlockTypeImage:             "Image",
	BlockTypeISV:               "ISV",
	BlockTypeMindNote:          "MindNote",
	BlockTypeSheet:             "Sheet",
	BlockTypeTable:             "Table",
	BlockTypeTableCell:         "TableCell",
	BlockTypeView:              "View",
	BlockTypeQuoteContainer:    "QuoteContainer",
	BlockTypeTask:              "Task",
	BlockTypeOKR:               "OKR",
	BlockTypeOKRObjective:      "OKRObjective",
	BlockTypeOKRKeyResult:      "OKRKeyResult",
	BlockTypeOKRProgress:       "OKRProgress",
	BlockTypeAddOns:            "AddOns",
	BlockTypeJiraIssue:         "JiraIssue",
	BlockTypeWikiCatalog:       "WikiCatalog",
	BlockTypeBoard:             "Board",
	BlockTypeAgenda:            "Agenda",
	BlockTypeAgendaItem:        "AgendaItem",
	BlockTypeAgendaItemTitle:   "AgendaItemTitle",
	BlockTypeAgendaItemContent: "AgendaItemContent",
	BlockTypeLinkPreview:       "LinkPreview",
	BlockTypeSyncSource:        "SyncSource",
	BlockTypeSyncReference:     "SyncReference",
	BlockTypeWikiCatalogV2:     "WikiCatalogV2",
	BlockTypeAITemplate:        "AITemplate",
	BlockTypeUndefined:         "Undefined",
}

// BlockTypeName 返回块类型的可读名称，未知类型返回 "Unknown(N)"
func BlockTypeName(bt BlockType) string {
	if name, ok := blockTypeName[bt]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", int(bt))
}

// ISV 块类型 ID 常量（飞书团队互动应用）
const (
	ISVTypeTextDrawing = "blk_631fefbbae02400430b8f9f4" // Mermaid 绘图
	ISVTypeTimeline    = "blk_6358a421bca0001c22536e4c" // 时间线
)

// fontColorMap 将飞书字体颜色枚举值映射为 CSS 颜色
var fontColorMap = map[int]string{
	1: "#ef4444", // Red
	2: "#f97316", // Orange
	3: "#eab308", // Yellow
	4: "#22c55e", // Green
	5: "#3b82f6", // Blue
	6: "#a855f7", // Purple
	7: "#6b7280", // Gray
}

// fontBgColorMap 将飞书字体背景色枚举值映射为 CSS 颜色
var fontBgColorMap = map[int]string{
	1:  "#fef2f2", // LightRed
	2:  "#fff7ed", // LightOrange
	3:  "#fefce8", // LightYellow
	4:  "#f0fdf4", // LightGreen
	5:  "#eff6ff", // LightBlue
	6:  "#faf5ff", // LightPurple
	7:  "#f9fafb", // LightGray
	8:  "#fecaca", // DarkRed
	9:  "#fed7aa", // DarkOrange
	10: "#fef08a", // DarkYellow
	11: "#bbf7d0", // DarkGreen
	12: "#bfdbfe", // DarkBlue
	13: "#e9d5ff", // DarkPurple
	14: "#e5e7eb", // DarkGray
}
