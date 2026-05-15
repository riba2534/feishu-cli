package client

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// ExtractCardTexts 从 interactive 卡片 body.content 中提取适合 CLI 阅读的文本。
// 支持 user_card_content 的 schema 2.0 结构、raw_card_content 的 json_card 字段，
// 以及 OAPI 默认渲染版的 post-like elements/content 结构。
func ExtractCardTexts(msg *larkim.Message) []string {
	if msg == nil || StringVal(msg.MsgType) != "interactive" || msg.Body == nil || msg.Body.Content == nil {
		return nil
	}
	var root any
	if err := json.Unmarshal([]byte(*msg.Body.Content), &root); err != nil {
		text := strings.TrimSpace(*msg.Body.Content)
		if text == "" {
			return nil
		}
		return []string{text}
	}
	texts := collectCardTexts(root)
	return compactCardTexts(texts)
}

// ExtractCardTextMap 以 message_id 为 key，批量提取 interactive 卡片文本。
func ExtractCardTextMap(messages []*larkim.Message) map[string][]string {
	out := make(map[string][]string)
	for _, msg := range messages {
		id := StringVal(msg.MessageId)
		texts := ExtractCardTexts(msg)
		if id != "" && len(texts) > 0 {
			out[id] = texts
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectCardTexts(v any) []string {
	switch value := v.(type) {
	case string:
		var nested any
		if json.Unmarshal([]byte(value), &nested) == nil {
			return collectCardTexts(nested)
		}
		return []string{value}
	case []any:
		var texts []string
		for _, item := range value {
			texts = append(texts, collectCardTexts(item)...)
		}
		return texts
	case map[string]any:
		return collectCardTextsFromMap(value)
	default:
		return nil
	}
}

func collectCardTextsFromMap(m map[string]any) []string {
	var texts []string
	hasDirectText := false
	for _, key := range []string{"text", "content", "summary", "title", "subtitle"} {
		if text, ok := m[key].(string); ok {
			texts = append(texts, text)
			hasDirectText = true
		}
	}
	if !hasDirectText {
		for _, key := range []string{"i18nContent", "i18n_content"} {
			if i18n, ok := m[key].(map[string]any); ok {
				texts = append(texts, collectI18nTexts(i18n)...)
			}
		}
	}
	if jsonCard, ok := m["json_card"].(string); ok {
		texts = append(texts, collectCardTexts(jsonCard)...)
	}
	for _, key := range []string{
		"body", "newBody", "header", "config", "summary", "elements", "items", "columns",
		"content", "text", "fields", "options", "actions", "extra",
	} {
		child, ok := m[key]
		if ok {
			texts = append(texts, collectCardTexts(child)...)
		}
	}
	if property, ok := m["property"].(map[string]any); ok {
		texts = append(texts, collectCardTexts(property)...)
	}
	return texts
}

func collectI18nTexts(m map[string]any) []string {
	for _, key := range []string{"zh_cn", "zh-CN", "zh_CN", "zh-hans", "en_us", "en-US", "en_US"} {
		if text, ok := m[key].(string); ok {
			return []string{text}
		}
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if text, ok := m[key].(string); ok {
			return []string{fmt.Sprintf("%s: %s", key, text)}
		}
	}
	return nil
}

func compactCardTexts(texts []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(texts))
	for _, text := range texts {
		text = strings.TrimSpace(text)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	return out
}
