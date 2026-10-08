package client

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// atMentionFixRe 匹配 AI 常见的 @ 标签变体：
//
//	<at id=ou_xxx>  /  <at open_id="ou_xxx">  /  <at user_id=ou_xxx/>
//
// 统一规范化为 <at user_id="ou_xxx"> 形式。
// <at email="..."/> 不在匹配范围内，会原样保留 —— 飞书 API 原生支持邮箱艾特。
var atMentionFixRe = regexp.MustCompile(`<at\s+(id|open_id|user_id)=("?)([^"\s/>]+)"?\s*/?>`)

// NormalizeAtMentions 修复纯文本中常见的 @ 标签格式错误，使其符合飞书 API 接受的
// <at user_id="..."> 标准形式。入参必须是**未经 JSON 编码的原始文本**（--text、--markdown
// 的正文）；已是 JSON 的 text/post 消息体请用 NormalizeAtMentionsInJSON，直接对 JSON 串做
// 正则替换会把裸双引号写进字符串字面量、破坏 JSON。
// interactive 卡片 JSON 不应在此处理。
func NormalizeAtMentions(content string) string {
	return atMentionFixRe.ReplaceAllString(content, `<at user_id="$3">`)
}

// NormalizeAtMentionsInJSON 对 text / post 消息体 JSON 里的所有字符串值做 @ 标签规范化
// （对齐官方 im +messages-send/+messages-reply 对 text/post 统一规范化的行为）。
//
//   - 先解码再逐个字符串替换，再重新编码，保证插入的双引号被正确转义；
//   - 没有任何改动时原样返回，避免无谓地改变用户 JSON 的键顺序与格式；
//   - 解析失败（非法 JSON）时原样返回，交由调用方的 JSON 校验或服务端报错。
func NormalizeAtMentionsInJSON(content string) string {
	if !strings.Contains(content, "<at") {
		return content
	}
	dec := json.NewDecoder(strings.NewReader(content))
	dec.UseNumber() // 保留数字原样，避免大整数被转成 float 丢精度
	var v any
	if err := dec.Decode(&v); err != nil {
		return content
	}
	changed := false
	v = normalizeAtMentionsValue(v, &changed)
	if !changed {
		return content
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // 保持 <at ...> 可读，不把 < > 转义成 \u003c \u003e
	if err := enc.Encode(v); err != nil {
		return content
	}
	return strings.TrimRight(buf.String(), "\n")
}

func normalizeAtMentionsValue(v any, changed *bool) any {
	switch t := v.(type) {
	case string:
		n := NormalizeAtMentions(t)
		if n != t {
			*changed = true
		}
		return n
	case map[string]any:
		for k, val := range t {
			t[k] = normalizeAtMentionsValue(val, changed)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = normalizeAtMentionsValue(val, changed)
		}
		return t
	default:
		return v
	}
}
