package client

import (
	"reflect"
	"testing"
)

func TestBuildReplaceImagePayload(t *testing.T) {
	cases := []struct {
		name string
		opts ReplaceImageOptions
		want map[string]any
	}{
		{
			name: "仅 token：宽高对齐描述均省略",
			opts: ReplaceImageOptions{},
			want: map[string]any{"token": "tok"},
		},
		{
			name: "宽高同时为正才下发",
			opts: ReplaceImageOptions{Width: 1916, Height: 821},
			want: map[string]any{"token": "tok", "width": 1916, "height": 821},
		},
		{
			name: "只有宽：两个字段都省略",
			opts: ReplaceImageOptions{Width: 1916},
			want: map[string]any{"token": "tok"},
		},
		{
			name: "只有高：两个字段都省略",
			opts: ReplaceImageOptions{Height: 821},
			want: map[string]any{"token": "tok"},
		},
		{
			name: "align 非 0 才下发",
			opts: ReplaceImageOptions{Align: 2},
			want: map[string]any{"token": "tok", "align": 2},
		},
		{
			name: "caption 非空才下发",
			opts: ReplaceImageOptions{Caption: "公司 Logo"},
			want: map[string]any{"token": "tok", "caption": map[string]string{"content": "公司 Logo"}},
		},
		{
			name: "全量字段",
			opts: ReplaceImageOptions{Width: 100, Height: 200, Align: 3, Caption: "c"},
			want: map[string]any{
				"token": "tok", "width": 100, "height": 200,
				"align": 3, "caption": map[string]string{"content": "c"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildReplaceImagePayload("tok", tc.opts)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("buildReplaceImagePayload() = %#v，期望 %#v", got, tc.want)
			}
		})
	}
}
