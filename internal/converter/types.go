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
	DownloadImages bool
	AssetsDir      string
	UploadImages   bool
	DocumentID     string
}
