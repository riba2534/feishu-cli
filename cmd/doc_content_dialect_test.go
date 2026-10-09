package cmd

import (
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

func TestConvertLocalDialectCallouts(t *testing.T) {
	in := "段落\n> [!WARNING]\n> 第一行 **加粗**\n>\n> 第二段\n\n> 普通引用\n"
	got, conv, err := convertLocalDialectForDocsAI(in)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	want := "段落\n\n<callout background-color=\"light-red\" border-color=\"red\">\n\n第一行 **加粗**\n\n第二段\n\n</callout>\n\n> 普通引用\n"
	if got != want {
		t.Fatalf("转换结果 =\n%q\n期望\n%q", got, want)
	}
	if conv.summary() != "callout×1" {
		t.Fatalf("统计 = %q", conv.summary())
	}
}

func TestConvertLocalDialectCalloutTypes(t *testing.T) {
	for typ, color := range map[string]string{"NOTE": "blue", "TIP": "yellow", "CAUTION": "orange", "SUCCESS": "green", "IMPORTANT": "purple", "FOO": "blue"} {
		got, _, err := convertLocalDialectForDocsAI("> [!" + typ + "]\n> 内容")
		if err != nil {
			t.Fatalf("%s 转换失败: %v", typ, err)
		}
		if !strings.Contains(got, `background-color="light-`+color+`" border-color="`+color+`"`) {
			t.Fatalf("%s 颜色映射错误: %q", typ, got)
		}
	}
	// 本地导入语法 <callout type="X">：type/color 改写为 docs_ai 属性，内容保持
	got, _, err := convertLocalDialectForDocsAI(`<callout type="WARNING">本地 **加粗**</callout>` + "\n" + `<callout color="4">绿</callout>`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `<callout background-color="light-red" border-color="red">本地 **加粗**</callout>`) ||
		!strings.Contains(got, `<callout background-color="light-green" border-color="green">绿</callout>`) {
		t.Fatalf("本地 callout 标签转换错误: %q", got)
	}
}

func TestConvertLocalDialectInlineTags(t *testing.T) {
	in := strings.Join([]string{
		`<image token="imgTok" width="2048" height="1024" align="center"/>`,
		`用户 <mention-user id="ou_xxx"/> 文档 <mention-doc token="docTok" type="docx">标题</mention-doc>`,
		`<span style="color: #ef4444; background-color: #fefce8">红字黄底</span> <span style="color: #123456">自定义</span>`,
		`<file token="fileTok" name="报告.pdf" view-type="1"/>`,
	}, "\n")
	got, conv, err := convertLocalDialectForDocsAI(in)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	for _, want := range []string{
		`<img src="imgTok" width="2048" height="1024"/>`,
		`<cite type="user" user-id="ou_xxx"/>`,
		`<cite type="doc" doc-id="docTok"/>`,
		`<span text-color="red" background-color="light-yellow">红字黄底</span>`,
		`<span style="color: #123456">自定义</span>`, // 非本地导出颜色原样保留
		`<source token="fileTok" name="报告.pdf"/>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("缺少 %q，实际:\n%s", want, got)
		}
	}
	if s := conv.summary(); !strings.Contains(s, "image×1") || !strings.Contains(s, "mention-doc×1") || !strings.Contains(s, "span×1") {
		t.Fatalf("统计异常: %q", s)
	}
}

func TestConvertLocalDialectGrid(t *testing.T) {
	in := "<grid cols=\"2\">\n<column>\n左 **粗**\n</column>\n<column>\n右\n</column>\n</grid>"
	got, _, err := convertLocalDialectForDocsAI(in)
	if err != nil {
		t.Fatal(err)
	}
	want := "<grid>\n<column width-ratio=\"0.5000\">\n\n左 **粗**\n\n</column>\n<column width-ratio=\"0.5000\">\n\n右\n\n</column>\n</grid>"
	if got != want {
		t.Fatalf("grid 转换 =\n%q\n期望\n%q", got, want)
	}
	got, _, err = convertLocalDialectForDocsAI(`<grid cols="3"><column>a</column><column>b</column><column>c</column></grid>`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "<grid><column width-ratio=\"0.3333\">a</column>") || strings.Contains(got, "cols=") {
		t.Fatalf("单行 grid 转换错误: %q", got)
	}
}

func TestConvertLocalDialectLeavesCodeUntouched(t *testing.T) {
	in := "```markdown\n> [!NOTE]\n<image token=\"x\"/>\n<whiteboard token=\"w\" type=\"blank\"/>\n```\n行内 `<image token=\"y\"/>` 代码"
	got, conv, err := convertLocalDialectForDocsAI(in)
	if err != nil {
		t.Fatalf("代码块内的方言不应触发 fail-closed: %v", err)
	}
	if got != in || conv.summary() != "" {
		t.Fatalf("代码块/行内代码不应改写: %q (%s)", got, conv.summary())
	}
}

func TestConvertLocalDialectFailClosed(t *testing.T) {
	cases := map[string]string{
		"画板占位":    `<whiteboard token="EEr5xxx" type="blank"/>`,
		"多维表格":    `<bitable token="bascnxxx" view="table"/>`,
		"电子表格占位":  `<sheet token="shtxxx" id="abc" rows="3" cols="3"/>`,
		"未下载视频":   `<video controls src="feishu://media/boxxxx" data-name="a.mp4"></video>`,
		"展开的电子表格": "<!-- sheet token=\"shtxxx\" id=\"abc\" -->\n\n| a |\n|---|\n| 1 |",
		"不支持的块":   `<!-- 不支持的块类型: OKR (type=36) -->`,
		"同步块未展开":  "> [!WARNING]\n> 同步块内容未展开（source_document_id=doc, source_block_id=blk）：无权限",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := convertLocalDialectForDocsAI("前文\n\n" + in + "\n")
			if err == nil {
				t.Fatalf("%s 应 fail-closed", name)
			}
			if !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "第 3 行") {
				t.Fatalf("%s 错误应为用法错误并定位行号: %v", name, err)
			}
		})
	}
	// 显式的 docs_ai 写法放行：新建空白画板、按 mermaid 生成
	for _, ok := range []string{`<whiteboard type="blank"></whiteboard>`, "<whiteboard type=\"mermaid\">\ngraph TD\nA-->B\n</whiteboard>", `<sheet type="blank"/>`} {
		if _, _, err := convertLocalDialectForDocsAI(ok); err != nil {
			t.Fatalf("%q 不应被拒绝: %v", ok, err)
		}
	}
}

func TestConvertLocalDialectDropsViewComment(t *testing.T) {
	got, _, err := convertLocalDialectForDocsAI("<!-- 不支持的块类型: View (type=33) -->\n\n<file token=\"f\" name=\"a.txt\"/>")
	if err != nil {
		t.Fatalf("视图块注释不应 fail-closed: %v", err)
	}
	if strings.Contains(got, "View") || !strings.Contains(got, `<source token="f" name="a.txt"/>`) {
		t.Fatalf("转换结果异常: %q", got)
	}
}

func TestMapOutsideInlineCode(t *testing.T) {
	got := mapOutsideInlineCode("a `b` c ``d`e`` f `未闭合", strings.ToUpper)
	if got != "A `b` C ``d`e`` F `未闭合" {
		t.Fatalf("got %q", got)
	}
}

// TestContentUpdateCommandConvertsDialectBeforeSend 命令入口：本地方言在发送前被转换；
// 画板占位在任何网络请求前 fail-closed。
func TestContentUpdateCommandConvertsDialectBeforeSend(t *testing.T) {
	f := newFakeDocsAI(t, "")
	_ = docContentUpdateCmd.Flags().Set("mode", "append")
	_ = docContentUpdateCmd.Flags().Set("markdown", "> [!NOTE]\n> 提示内容\n\n<image token=\"imgTok\" width=\"10\" height=\"10\"/>")
	defer func() {
		_ = docContentUpdateCmd.Flags().Set("mode", "")
		_ = docContentUpdateCmd.Flags().Set("markdown", "")
	}()
	if err := docContentUpdateCmd.RunE(docContentUpdateCmd, []string{"doc"}); err != nil {
		t.Fatalf("append 失败: %v", err)
	}
	if len(f.bodies) != 1 {
		t.Fatalf("PUT 次数 = %d", len(f.bodies))
	}
	content, _ := f.bodies[0]["content"].(string)
	if !strings.Contains(content, `<callout background-color="light-blue" border-color="blue">`) ||
		!strings.Contains(content, `<img src="imgTok" width="10" height="10"/>`) || strings.Contains(content, "[!NOTE]") {
		t.Fatalf("发送内容未转换为 docs_ai 写法: %q", content)
	}

	initDocUpdateTestConfig(t, "http://127.0.0.1:59997") // 不可达：若发出网络请求会得到连接错误
	_ = docContentUpdateCmd.Flags().Set("mode", "overwrite")
	_ = docContentUpdateCmd.Flags().Set("markdown", "# 标题\n\n<whiteboard token=\"EEr5xxx\" type=\"blank\"/>\n")
	err := docContentUpdateCmd.RunE(docContentUpdateCmd, []string{"doc"})
	if err == nil || !strings.Contains(err.Error(), "画板占位") || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("画板占位应在网络请求前 fail-closed（exit 2），得到: %v", err)
	}
}
