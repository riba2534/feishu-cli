package client

import "strings"

// 导入型 Office 文档（pptx/xlsx/docx 导入后生成的在线文档）token 的历史合成前缀。
const (
	FakeOfficeTokenPrefix  = "fake_office_"
	LocalOfficeTokenPrefix = "local_office_"
)

var officeTokenPrefixes = []string{FakeOfficeTokenPrefix, LocalOfficeTokenPrefix}

// officeTokenMarker 是导入型 Office token 在固定偏移处交织的产品/区域标记。
const officeTokenMarker = "OFL0X"

// officeTokenMarkerOffsets 是标记字符所在的 0-based 下标（人类 1-based 第 5/10/15/20/25 位）。
// 数组长度与标记长度绑定：任何一方改动而另一方没跟上会直接编译失败。
var officeTokenMarkerOffsets = [len(officeTokenMarker)]int{4, 9, 14, 19, 24}

// officeTokenMinLen 是能容纳标记的最短 token 长度（最后一个偏移 + 1）。
// 这是下限而非精确长度，见 IsLocalOfficeToken 注释。
const officeTokenMinLen = 25

// IsLocalOfficeToken 判断 token 是否属于「导入型 Office 文档」（由 pptx/xlsx/docx
// 导入生成，而非通过 API 原生创建）。对齐官方 lark-cli common.IsLocalOfficeToken。
//
// 判定规则：
//  1. 兼容历史前缀 fake_office_ / local_office_；
//  2. 长度 ≥ 25，且 0-based 下标 4/9/14/19/24 依次为 'O','F','L','0','X'。
//
// 两点是承重设计：
//   - 按固定偏移精确读取，而不是 strings.Contains 宽松匹配：误判为 Office 是更危险的方向——
//     Drive 后端不校验 parent_node 是否真是 Office 文件，原生文档被误判后上传仍会成功，
//     但图片之后静默不渲染，远离出错位置难以排查；漏判则会在上传时直接报错。
//   - 长度只设下限：旧实现要求恰好 28 位，而当前格式（官方 #2509：OFL0X + 21 位随机 +
//     1 位 Office 类型枚举）是 27 位，会被整类误判为原生文档。放宽长度之所以安全，
//     正是因为偏移固定——同长度但携带其他标记（原生 pptcn/shtcn 交织）的 token 仍判为原生。
//
// 只有 token 形状是 Drive 层共享属性；各领域据此选择的 parent_type
// （office_sheet_file / office_slide_file）由各自领域维护。
func IsLocalOfficeToken(token string) bool {
	for _, prefix := range officeTokenPrefixes {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	if len(token) < officeTokenMinLen {
		return false
	}
	for i, offset := range officeTokenMarkerOffsets {
		if token[offset] != officeTokenMarker[i] {
			return false
		}
	}
	return true
}
