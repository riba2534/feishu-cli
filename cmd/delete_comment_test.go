package cmd

import (
	"strings"
	"testing"
)

// TestDeleteCommentCmdRegistered 验证 delete 子命令仍注册在 comment 命令组下，且对外可见。
func TestDeleteCommentCmdRegistered(t *testing.T) {
	if deleteCommentCmd.Use != "delete <file_token> <comment_id>" {
		t.Fatalf("Use = %q", deleteCommentCmd.Use)
	}
	if deleteCommentCmd.Hidden {
		t.Error("delete 命令应保持可见，便于用户/Agent 在 help 中看到替代方案")
	}
	found := false
	for _, sub := range commentCmd.Commands() {
		if sub == deleteCommentCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("deleteCommentCmd 应是 commentCmd 的子命令")
	}
}

// TestDeleteCommentReturnsGuidance 验证无论是否带参数，delete 都返回可执行的替代方案指引，
// 而不是去调用一个并不存在的「删除整条评论」API。
func TestDeleteCommentReturnsGuidance(t *testing.T) {
	cases := [][]string{
		{},
		{"doccnXXX"},
		{"doccnXXX", "cmt123", "--type", "docx"},
	}
	for _, args := range cases {
		err := deleteCommentCmd.RunE(deleteCommentCmd, args)
		if err == nil {
			t.Fatalf("args=%v: 期望返回指引错误，得到 nil", args)
		}
		msg := err.Error()
		for _, want := range []string{"comment reply delete", "comment resolve", "不支持"} {
			if !strings.Contains(msg, want) {
				t.Errorf("args=%v: 错误信息缺少 %q，实际: %s", args, want, msg)
			}
		}
	}
}

// TestDeleteCommentTypeFlagOptional 验证 --type 仅作兼容占位、非必填，
// 这样历史调用写法不会因 "unknown flag" 报错，从而能看到指引。
func TestDeleteCommentTypeFlagOptional(t *testing.T) {
	f := deleteCommentCmd.Flags().Lookup("type")
	if f == nil {
		t.Fatal("--type flag 应存在（兼容占位）")
	}
	ann := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]
	if len(ann) != 0 && ann[0] == "true" {
		t.Error("--type 不应为必填")
	}
}
