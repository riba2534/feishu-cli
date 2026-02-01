package converter

// BlockType represents Feishu block types
type BlockType int

const (
	BlockTypePage           BlockType = 1
	BlockTypeText           BlockType = 2
	BlockTypeHeading1       BlockType = 3
	BlockTypeHeading2       BlockType = 4
	BlockTypeHeading3       BlockType = 5
	BlockTypeHeading4       BlockType = 6
	BlockTypeHeading5       BlockType = 7
	BlockTypeHeading6       BlockType = 8
	BlockTypeHeading7       BlockType = 9
	BlockTypeHeading8       BlockType = 10
	BlockTypeHeading9       BlockType = 11
	BlockTypeBullet         BlockType = 12
	BlockTypeOrdered        BlockType = 13
	BlockTypeCode           BlockType = 14
	BlockTypeQuote          BlockType = 15
	BlockTypeEquation       BlockType = 16
	BlockTypeTodo           BlockType = 17
	BlockTypeBitable        BlockType = 18
	BlockTypeCallout        BlockType = 19
	BlockTypeChatCard       BlockType = 20
	BlockTypeDiagram        BlockType = 21 // Mermaid/UML 绘图块
	BlockTypeDivider        BlockType = 22
	BlockTypeFile           BlockType = 23
	BlockTypeGrid           BlockType = 24
	BlockTypeGridColumn     BlockType = 25
	BlockTypeIframe         BlockType = 26
	BlockTypeImage          BlockType = 27
	BlockTypeISV            BlockType = 28
	BlockTypeMindNote       BlockType = 29
	BlockTypeSheet          BlockType = 30
	BlockTypeTable          BlockType = 31
	BlockTypeTableCell      BlockType = 32
	BlockTypeView           BlockType = 33
	BlockTypeQuoteContainer BlockType = 34
	BlockTypeTask           BlockType = 35
	BlockTypeOKR            BlockType = 36
	BlockTypeOKRObjective   BlockType = 37
	BlockTypeOKRKeyResult   BlockType = 38
	BlockTypeOKRProgress    BlockType = 39
	BlockTypeAddOns         BlockType = 40
	BlockTypeJiraIssue      BlockType = 41
	BlockTypeWikiCatalog    BlockType = 42
	BlockTypeBoard          BlockType = 43 // 画板块
	BlockTypeUndefined      BlockType = 999
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
}

// ImageStats 记录图片处理统计
type ImageStats struct {
	Skipped int // 跳过（API 不支持插入图片）数
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
