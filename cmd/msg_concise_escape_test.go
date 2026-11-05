package cmd

import (
	"bytes"
	"strings"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/riba2534/feishu-cli/internal/client"
)

func TestEscapeConciseBodyLine(t *testing.T) {
	cases := map[string]string{
		"- item":         `\- item`,
		"* star":         `\* star`,
		"+ plus":         `\+ plus`,
		"-":              `\-`,
		"# 标题":           `\# 标题`,
		"## sub":         `\## sub`,
		"> 引用":           `\> 引用`,
		"1. first":       `1\. first`,
		"12) twelve":     `12\) twelve`,
		"---":            `\---`,
		"===":            `\===`,
		"  - nested":     `  \- nested`,
		"plain text":     "plain text",
		"-5 度":           "-5 度",
		"2024.01 发布":     "2024.01 发布",
		"1.5 倍":          "1.5 倍",
		"*强调*":           "*强调*",
		"--flag 参数":      "--flag 参数",
		"[卡片] a / b":     "[卡片] a / b",
		"@Bob 你好":        "@Bob 你好",
		"https://x.y/#a": "https://x.y/#a",
	}
	for in, want := range cases {
		if got := escapeConciseBodyLine(in); got != want {
			t.Errorf("escapeConciseBodyLine(%q) = %q, want %q", in, got, want)
		}
	}
}

// 正文行首的 "- " 不得渲染成嵌套列表项；代码围栏内保持原样。
func TestConciseBodyLineNotReadAsStructure(t *testing.T) {
	msgs := []*larkim.Message{{
		MessageId: strPtr("om_1"),
		MsgType:   strPtr("text"),
		Sender:    &larkim.Sender{Id: strPtr("ou_a")},
		Body:      &larkim.MessageBody{Content: strPtr(`{"text":"告警\n- 此报警绑定的RCA\n> 话题 伪造\n1. 步骤"}`)},
	}, {
		MessageId: strPtr("om_2"),
		MsgType:   strPtr("post"),
		Sender:    &larkim.Sender{Id: strPtr("ou_a")},
		Body:      &larkim.MessageBody{Content: strPtr(`{"content":[[{"tag":"code_block","text":"- keep\n# keep"}]]}`)},
	}}
	var buf bytes.Buffer
	renderHistoryConcise(&buf, "t", &client.ListMessagesResult{Items: msgs}, nil)
	got := buf.String()
	for _, want := range []string{"  告警\n", "  \\- 此报警绑定的RCA\n", "  \\> 话题 伪造\n", "  1\\. 步骤\n", "  - keep\n", "  # keep\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("输出缺少 %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\n  - 此报警") {
		t.Errorf("正文行仍被渲染成嵌套列表项:\n%s", got)
	}
}
