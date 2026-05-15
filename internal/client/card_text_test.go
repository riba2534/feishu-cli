package client

import (
	"encoding/json"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestExtractCardTexts_UserCardContent(t *testing.T) {
	content := `{
		"schema": "2.0",
		"body": {
			"elements": [
				{"tag": "markdown", "content": "Review 已提交到 MR。"},
				{"tag": "plain_text", "content": "需修复后合并"},
				{"tag": "markdown", "content": "Review 已提交到 MR。"}
			]
		},
		"config": {
			"summary": {
				"content": "Review 摘要"
			}
		}
	}`
	msg := interactiveMessage("om_card_user", content)

	got := ExtractCardTexts(msg)
	want := []string{"Review 已提交到 MR。", "需修复后合并", "Review 摘要"}
	if !equalStringSlices(got, want) {
		t.Fatalf("ExtractCardTexts() = %#v, want %#v", got, want)
	}
}

func TestExtractCardTexts_RawCardContent(t *testing.T) {
	raw := map[string]any{
		"json_card": `{
			"body": {
				"elements": [
					{"tag": "markdown", "property": {"content": "CI 全部通过"}},
					{"tag": "plain_text", "property": {"content": "review_not_passed"}}
				]
			}
		}`,
		"card_schema": 2,
	}
	contentBytes, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal raw card: %v", err)
	}
	msg := interactiveMessage("om_card_raw", string(contentBytes))

	got := ExtractCardTexts(msg)
	want := []string{"CI 全部通过", "review_not_passed"}
	if !equalStringSlices(got, want) {
		t.Fatalf("ExtractCardTexts() = %#v, want %#v", got, want)
	}
}

func TestExtractCardTexts_RenderedUpgradeFallback(t *testing.T) {
	msg := interactiveMessage("om_card_rendered", `{"title":null,"elements":[[{"tag":"text","text":"请升级至最新版本客户端，以查看内容"}]]}`)

	got := ExtractCardTexts(msg)
	want := []string{"请升级至最新版本客户端，以查看内容"}
	if !equalStringSlices(got, want) {
		t.Fatalf("ExtractCardTexts() = %#v, want %#v", got, want)
	}
}

func TestExtractCardTexts_NestedTextObject(t *testing.T) {
	msg := interactiveMessage("om_card_nested", `{
		"body": {
			"elements": [
				{
					"tag": "div",
					"text": {"tag": "lark_md", "content": "嵌套正文"}
				},
				{
					"tag": "button",
					"text": {"tag": "plain_text", "content": "查看详情"},
					"value": {"label": "隐藏 payload 不展示"}
				}
			]
		}
	}`)

	got := ExtractCardTexts(msg)
	want := []string{"嵌套正文", "查看详情"}
	if !equalStringSlices(got, want) {
		t.Fatalf("ExtractCardTexts() = %#v, want %#v", got, want)
	}
}

func TestExtractCardTextMap(t *testing.T) {
	textContent := `{"text":"hello"}`
	got := ExtractCardTextMap([]*larkim.Message{
		interactiveMessage("om_card", `{"body":{"elements":[{"tag":"plain_text","content":"hello card"}]}}`),
		{MessageId: strPtr("om_text"), MsgType: strPtr("text"), Body: &larkim.MessageBody{Content: &textContent}},
	})

	if len(got) != 1 || !equalStringSlices(got["om_card"], []string{"hello card"}) {
		t.Fatalf("ExtractCardTextMap() = %#v", got)
	}
}

func interactiveMessage(id, content string) *larkim.Message {
	return &larkim.Message{
		MessageId: strPtr(id),
		MsgType:   strPtr("interactive"),
		Body:      &larkim.MessageBody{Content: &content},
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func strPtr(s string) *string {
	return &s
}
