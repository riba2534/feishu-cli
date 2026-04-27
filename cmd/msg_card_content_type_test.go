package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveCardContentType(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		want      string
		wantErr   bool
	}{
		{"未传 flag 默认空", "", "", false},
		{"短写 user", "user", "user_card_content", false},
		{"短写 raw", "raw", "raw_card_content", false},
		{"全名 user_card_content", "user_card_content", "user_card_content", false},
		{"全名 raw_card_content", "raw_card_content", "raw_card_content", false},
		{"大小写不敏感 USER", "USER", "user_card_content", false},
		{"大小写不敏感 Raw", "Raw", "raw_card_content", false},
		{"前后空格被裁剪", "  user  ", "user_card_content", false},
		{"非法值报错", "userdsl", "", true},
		{"乱写报错", "abc", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			addCardContentTypeFlag(cmd)
			if tt.flagValue != "" {
				if err := cmd.Flags().Set("card-content-type", tt.flagValue); err != nil {
					t.Fatalf("flag set 失败: %v", err)
				}
			}

			got, err := resolveCardContentType(cmd)
			if tt.wantErr {
				if err == nil {
					t.Errorf("resolveCardContentType(%q) 期望返回错误，但返回 %q", tt.flagValue, got)
				}
				return
			}
			if err != nil {
				t.Errorf("resolveCardContentType(%q) 返回意外错误: %v", tt.flagValue, err)
				return
			}
			if got != tt.want {
				t.Errorf("resolveCardContentType(%q) = %q, 期望 %q", tt.flagValue, got, tt.want)
			}
		})
	}
}
