package client

import (
	"encoding/json"
	"testing"
)

func TestNormalizeAtMentions(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "id 属性无引号无斜杠",
			in:   `<at id=ou_alpha> hi`,
			want: `<at user_id="ou_alpha"> hi`,
		},
		{
			name: "open_id 属性带引号",
			in:   `<at open_id="ou_beta"> hello`,
			want: `<at user_id="ou_beta"> hello`,
		},
		{
			name: "user_id 自闭合无引号",
			in:   `<at user_id=ou_gamma /> bye`,
			want: `<at user_id="ou_gamma"> bye`,
		},
		{
			name: "email 形式保留不动（飞书原生支持）",
			in:   `<at email="alice@example.com"/> hi`,
			want: `<at email="alice@example.com"/> hi`,
		},
		{
			name: "@ all 透传",
			in:   `<at user_id="all"></at> attention`,
			want: `<at user_id="all"></at> attention`,
		},
		{
			name: "多种混合形式",
			in:   `<at id=ou_a/> 和 <at open_id="ou_b"> 还有 <at user_id=ou_c /> 以及 <at email="x@y.com"/>`,
			want: `<at user_id="ou_a"> 和 <at user_id="ou_b"> 还有 <at user_id="ou_c"> 以及 <at email="x@y.com"/>`,
		},
		{
			name: "无 @ 标签原样返回",
			in:   `普通文本 没有艾特`,
			want: `普通文本 没有艾特`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeAtMentions(c.in)
			if got != c.want {
				t.Errorf("NormalizeAtMentions() =\n  got:  %q\n  want: %q", got, c.want)
			}
		})
	}
}

func TestNormalizeAtMentionsInJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "text 消息体：插入的双引号被正确转义",
			in:   `{"text":"<at id=ou_alpha> hi"}`,
			want: `{"text":"<at user_id=\"ou_alpha\"> hi"}`,
		},
		{
			name: "post 消息体：md/text 节点都规范化",
			in:   `{"zh_cn":{"title":"t","content":[[{"tag":"md","text":"<at open_id=\"ou_beta\"> 看下"},{"tag":"text","text":"<at user_id=ou_gamma/>"}]]}}`,
			want: `{"zh_cn":{"content":[[{"tag":"md","text":"<at user_id=\"ou_beta\"> 看下"},{"tag":"text","text":"<at user_id=\"ou_gamma\">"}]],"title":"t"}}`,
		},
		{
			name: "无需修正时原样返回（保持键顺序与格式）",
			in:   `{ "text" : "<at user_id=\"ou_ok\"> hi", "z": 1 }`,
			want: `{ "text" : "<at user_id=\"ou_ok\"> hi", "z": 1 }`,
		},
		{
			name: "大整数保持精度",
			in:   `{"text":"<at id=ou_a>","n":12345678901234567890}`,
			want: `{"n":12345678901234567890,"text":"<at user_id=\"ou_a\">"}`,
		},
		{
			name: "非法 JSON 原样返回",
			in:   `{"text":"<at id=ou_a>"`,
			want: `{"text":"<at id=ou_a>"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeAtMentionsInJSON(c.in)
			if got != c.want {
				t.Fatalf("NormalizeAtMentionsInJSON()\n got: %s\nwant: %s", got, c.want)
			}
			if c.in != got && !json.Valid([]byte(got)) {
				t.Fatalf("结果不是合法 JSON: %s", got)
			}
		})
	}
}
