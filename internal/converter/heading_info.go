package converter

import (
	"strings"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// HeadingInfo 判断 block 是否为标题块（Heading1-9），是则返回 (级别 1-9, 纯文本, true)。
// 供 doc read --outline 等需要在命令层遍历标题结构的场景使用。
func HeadingInfo(block *larkdocx.Block) (int, string, bool) {
	if block == nil || block.BlockType == nil {
		return 0, "", false
	}
	bt := BlockType(*block.BlockType)
	if bt < BlockTypeHeading1 || bt > BlockTypeHeading9 {
		return 0, "", false
	}
	elements, _ := getHeadingTextAndStyle(block, bt)
	var sb strings.Builder
	for _, elem := range elements {
		if elem != nil && elem.TextRun != nil && elem.TextRun.Content != nil {
			sb.WriteString(*elem.TextRun.Content)
		}
	}
	return int(bt) - int(BlockTypeHeading1) + 1, strings.TrimSpace(sb.String()), true
}
