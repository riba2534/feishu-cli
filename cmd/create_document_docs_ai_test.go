package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newDocCreateTestCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	createDocumentCmd.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
	c := &cobra.Command{Use: "create"}
	c.Flags().AddFlagSet(createDocumentCmd.Flags())
	if err := c.Flags().Parse(args); err != nil {
		t.Fatalf("解析参数失败: %v", err)
	}
	t.Cleanup(func() {
		createDocumentCmd.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
	})
	return c
}

func TestBuildDocsAICreateBody(t *testing.T) {
	c := newDocCreateTestCmd(t, "--title", "周报 <A&B>", "--content", "> [!NOTE]\\n> 提示", "--folder", "fldcnX")
	body, _, err := buildDocsAICreateBody(c)
	if err != nil {
		t.Fatalf("构造请求体失败: %v", err)
	}
	content, _ := body["content"].(string)
	if !strings.HasPrefix(content, "<title>周报 &lt;A&amp;B&gt;</title>\n") {
		t.Fatalf("--title 应转义后以 <title> 前置: %q", content)
	}
	if !strings.Contains(content, `<callout background-color="light-blue" border-color="blue">`) {
		t.Fatalf("本地方言应转换: %q", content)
	}
	if body["format"] != "markdown" || body["parent_token"] != "fldcnX" {
		t.Fatalf("body = %#v", body)
	}

	bad := []struct {
		args []string
		want string
	}{
		{[]string{"--content", ""}, "为空且未提供 --title"},
		{[]string{"--content", "x", "--folder", "a", "--parent-token", "b"}, "只能使用其中一个"},
		{[]string{"--content", "x", "--parent-token", "a", "--parent-position", "my_library"}, "互斥"},
		{[]string{"--content", "x", "--doc-format", "html"}, "不支持的 --doc-format"},
		{[]string{"--content", "![本地](./missing.png)"}, "本地图片不存在"},
		{[]string{"--content", "<whiteboard token=\"w\" type=\"blank\"/>"}, "画板占位"},
	}
	for _, tc := range bad {
		c := newDocCreateTestCmd(t, tc.args...)
		if _, _, err := buildDocsAICreateBody(c); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v 期望错误含 %q，得到 %v", tc.args, tc.want, err)
		}
	}
}
