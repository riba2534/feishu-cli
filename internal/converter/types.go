package converter

import (
	"fmt"
	"strings"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

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

// SheetDataProvider 提供电子表格数据，用于导出 docx 内嵌 Sheet 块。
type SheetDataProvider func(spreadsheetToken, sheetID, userAccessToken string) ([]*SheetData, error)

// SyncBlockProvider 提供引用同步块的源块及全部子孙块。
// 返回值应包含 sourceBlockID 对应的源同步块本身（with_descendants=true 的飞书 API 语义）。
type SyncBlockProvider func(sourceDocumentID, sourceBlockID, userAccessToken string) ([]*larkdocx.Block, error)

// ConvertOptions holds conversion options
type ConvertOptions struct {
	DownloadImages bool
	AssetsDir      string
	UploadImages   bool
	// EmbedTableImages 为 true 时，Markdown 表格单元格内的图片在转换期被收集到 TableData.CellImages，
	// 由导入层在表格填充后真正嵌入为单元格内的 Image 子块（issue #164）。为 false 时（如 doc content-update），
	// 单元格图片降级为占位文本/链接，避免静默丢失。
	EmbedTableImages    bool
	DocumentID          string
	UserAccessToken     string // User Access Token，用于下载图片和画板等需要权限的资源
	Debug               bool   // 为 true 时，输出下载失败等调试信息到 stderr
	DegradeDeepHeadings bool   // 为 true 时，Heading 7-9 输出为粗体段落而非 ######
	FrontMatter         bool   // 为 true 时，导出时添加 YAML front matter
	Highlight           bool   // 为 true 时，导出文本颜色和背景色为 HTML span
	ExpandMentions      bool   // 导出时展开 @用户为友好格式（默认 false，CLI 默认 true）
	ExpandSheets        bool   // 导出时展开内嵌电子表格为 Markdown 表格
	SheetDataProvider   SheetDataProvider
	SyncBlockProvider   SyncBlockProvider

	// ColumnWidthMode 控制 Markdown 表格转飞书表格时的列宽策略：
	//   - "" 或 "auto"：保留默认启发式（按字符宽度估算，CJK 14px / ASCII 8px）
	//   - "fixed"：所有列等分 defaultDocWidth/cols 像素
	//   - "explicit"：使用 ColumnWidthValues 指定每列宽度（不足补 minColumnWidth，超出截断）
	// 单表注释 <!-- feishu-colwidth: ... --> 优先级最高，可临时覆盖该字段。
	ColumnWidthMode string
	// ColumnWidthValues 仅在 ColumnWidthMode == "explicit" 时使用。
	// 单位：像素整数；0 表示该列走 auto；最终结果会过 [minColumnWidth, maxColumnWidth] clamp。
	ColumnWidthValues []int
}

// ConvertResult contains converted blocks and table data
type ConvertResult struct {
	BlockNodes []*BlockNode // 支持嵌套层级的块树
	TableDatas []*TableData // Table data in order of appearance, used for filling content（仅顶层表格）
	// TableDataByBlock 以表格块指针为键的填充数据，覆盖嵌套在分栏列等容器内的表格（TableDatas 只含顶层表格）。
	TableDataByBlock map[*larkdocx.Block]*TableData
	ImageStats       ImageStats // 图片处理统计
	ImageSources     []string   // 每个本地/网络图片 Image Block 的来源路径（按出现顺序；token 复用的图片不在此列，见 MediaRefs）
	VideoStats       VideoStats // 视频处理统计
	VideoSources     []string   // 每个本地视频 File Block 的来源路径（按出现顺序；token 复用的视频不在此列，见 MediaRefs）
	FileStats        VideoStats // 附件（非视频 <file token>）统计，字段含义同 VideoStats
	// MediaRefs 记录「建块后才能补齐内容」的块（以块指针为键，与 BlockNodes 中的 Block 同一指针）：
	// 图片、附件/视频、带 token 的画板。导入层建块后按此描述上传素材 / 复用 token / 复制画板。
	MediaRefs map[*larkdocx.Block]*MediaRef
	// Degradations 记录转换期已确定无法原样导入、已降级为占位文本的内容（如带 token 的 <sheet>/<bitable>），
	// 导入层应计入 failures，避免静默丢失。
	Degradations []Degradation
}

// MediaKind 标识建块后需要补齐内容的资源类型。
type MediaKind string

const (
	// MediaKindImage 图片：建空 Image 块 → 上传素材到该块 → replace_image。
	MediaKindImage MediaKind = "image"
	// MediaKindFile 附件/视频：建空 File 块（服务端外包一层 View 块）→ 上传素材到 File 块 → replace_file。
	MediaKindFile MediaKind = "file"
	// MediaKindWhiteboard 带 token 的画板：建空 Board 块 → 复制源画板节点。
	MediaKindWhiteboard MediaKind = "whiteboard"
)

// FeishuMediaScheme 是飞书素材 token 引用前缀（feishu://media/<token>），导入时按 token 复用素材。
const FeishuMediaScheme = "feishu://media/"

// MediaRef 描述一个块在建块后才能补齐的资源。
//
// 服务端约束（2026-10 实测）：docx 建块接口拒绝带 token 的 Image/File/Board/Sheet/Bitable（1770001 invalid param），
// File 块只能以 {"token":""} 建空块（带 name 同样 1770001）；跨文档素材 token 直接 replace_image 报
// 1770013 relation mismatch——素材必须「上传到该块」。因此导入层统一先建空块，再按本描述补齐。
type MediaRef struct {
	Kind   MediaKind
	Source string // 本地路径或 http(s) URL；为空时复用 Token
	Token  string // 复用的已有素材 token（图片/附件/视频）或源画板 token
	Name   string // 附件/视频文件名（建块时不能带 name，上传素材时用作文件名）
	Video  bool   // 附件是否为视频（统计口径）
	Width  int    // 图片显示宽度（<image width>），0 表示按原图像素
	Height int    // 图片显示高度（<image height>）
	Align  int    // 图片对齐（1 左 2 中 3 右），0 表示默认
}

// UploadSource 返回素材上传来源：本地路径/URL，或 feishu://media/<token>（下载后重新上传）。
func (r *MediaRef) UploadSource() string {
	if r == nil {
		return ""
	}
	if r.Source != "" {
		return r.Source
	}
	if r.Token != "" {
		return FeishuMediaScheme + r.Token
	}
	return ""
}

// Degradation 记录转换期已降级为占位文本的内容。
type Degradation struct {
	Kind   string // sheet / bitable / file
	Source string // 原始引用（token、文件名等）
	Reason string
}

// ImageStats 记录图片处理统计
type ImageStats struct {
	Total   int // 需要上传的图片总数
	Success int // 上传成功数
	Failed  int // 上传失败数
	Skipped int // 跳过（feishu://media/ 引用或 upload-images=false）数
}

// VideoStats 记录视频处理统计
type VideoStats struct {
	Total   int // 需要上传/下载的视频总数
	Success int // 成功数
	Failed  int // 失败数
	Skipped int // 跳过数
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

// Callout 背景色枚举（飞书 docx CalloutBackgroundColor，经 docs_ai 写入与读取实测校准）：
// 1 浅红、2 浅橙、3 浅黄、4 浅绿、5 浅蓝、6 浅紫、7 中灰；8-14 为对应的深色（红/橙/黄/绿/蓝/紫/灰）。
//
// 历史实现把映射整体错了一位（WARNING=2、NOTE=6、IMPORTANT=7），导致导入的 NOTE 显示为浅紫、
// WARNING 显示为浅橙、IMPORTANT 显示为灰色。
const (
	CalloutBgLightRed    = 1
	CalloutBgLightOrange = 2
	CalloutBgLightYellow = 3
	CalloutBgLightGreen  = 4
	CalloutBgLightBlue   = 5
	CalloutBgLightPurple = 6
	CalloutBgMediumGray  = 7
)

// CalloutColorForType 把 GitHub 风格高亮块类型映射为背景色枚举；未知类型按 NOTE（蓝）处理。
func CalloutColorForType(calloutType string) int {
	switch strings.ToUpper(strings.TrimSpace(calloutType)) {
	case "WARNING":
		return CalloutBgLightRed
	case "CAUTION":
		return CalloutBgLightOrange
	case "TIP":
		return CalloutBgLightYellow
	case "SUCCESS":
		return CalloutBgLightGreen
	case "IMPORTANT":
		return CalloutBgLightPurple
	default: // NOTE / INFO / 其它
		return CalloutBgLightBlue
	}
}

// CalloutTypeForColor 把背景色枚举映射回 GitHub 风格类型（深色按同色相处理）；
// 灰色没有对应类型，按 IMPORTANT 输出以兼容旧版导入的 IMPORTANT（历史上被写成 7）。
func CalloutTypeForColor(color int) string {
	switch color {
	case 1, 8:
		return "WARNING"
	case 2, 9:
		return "CAUTION"
	case 3, 10:
		return "TIP"
	case 4, 11:
		return "SUCCESS"
	case 6, 13:
		return "IMPORTANT"
	case 7, 14:
		return "IMPORTANT"
	default: // 5 / 12 / 未知
		return "NOTE"
	}
}
