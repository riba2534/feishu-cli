package cmd

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/skillbundle"
	"github.com/spf13/cobra"
)

func walkCommands(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, child := range c.Commands() {
		walkCommands(child, fn)
	}
}

func isHiddenCommand(c *cobra.Command) bool {
	for cur := c; cur != nil; cur = cur.Parent() {
		if cur.Hidden {
			return true
		}
	}
	return false
}

// TestSkillHelpPointersCoverAllCommands 所有非隐藏命令（含命令组）都有相关技能指针，
// 指向的工作流文件真实存在于（将被内嵌的）技能内容中；命令组的指针覆盖其全部可见子孙命令的归属。
func TestSkillHelpPointersCoverAllCommands(t *testing.T) {
	fsys := useRepoSkills(t)
	manifest, err := loadSkillManifest()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	walkCommands(rootCmd, func(c *cobra.Command) {
		if !c.HasParent() || isHiddenCommand(c) {
			return
		}
		parts := commandPathParts(c)
		if manifest.IsExcludedCommand(parts) {
			return
		}
		if _, ok := relatedSkillRef(c); !ok {
			t.Errorf("命令 %q 没有唯一的相关技能归属，请在 skills/manifest.yaml owners 中声明", c.CommandPath())
			return
		}
		entries := relatedSkillEntries(c)
		if len(entries) == 0 {
			t.Errorf("命令 %q 没有相关技能指针", c.CommandPath())
			return
		}
		help := relatedSkillHelp(c)
		listed := map[skillbundle.WorkflowRef]bool{}
		for _, entry := range entries {
			ref := entry.Ref
			listed[ref] = true
			target := ref.Skill + "/" + ref.Path()
			if info, err := fs.Stat(fsys, target); err != nil || info.IsDir() {
				t.Errorf("命令 %q 指向的技能文件不存在: %s", c.CommandPath(), target)
			}
			if !strings.Contains(help, ref.Skill+"（"+ref.Path()+"）") {
				t.Errorf("命令 %q 的技能指针缺少 %s/%s: %q", c.CommandPath(), ref.Skill, ref.Path(), help)
			}
		}
		if !strings.Contains(help, "相关技能:") || !strings.Contains(help, "feishu-cli skills read "+entries[0].Ref.Skill+" "+entries[0].Ref.Path()) {
			t.Errorf("命令 %q 的技能指针格式不对: %q", c.CommandPath(), help)
		}
		// 命令组必须列出全部可见子孙命令归属的工作流，不能只指向兜底前缀的一个工作流
		walkAvailableCommands(c, func(sub *cobra.Command) {
			if sub == c {
				return
			}
			if ref, ok := relatedSkillRef(sub); ok && !listed[ref] {
				t.Errorf("命令组 %q 的技能指针漏掉子命令 %q 的工作流 %s/%s", c.CommandPath(), sub.CommandPath(), ref.Skill, ref.Workflow)
			}
		})
		checked++
	})
	if checked < 400 {
		t.Fatalf("只检查了 %d 个命令，命令树可能未完整注册", checked)
	}
}

// TestSkillHelpGroupListsAllWorkflows 跨工作流的命令组（doc/msg）列出全部工作流及对应子命令，
// 去重且按 (技能, 工作流) 稳定排序；叶子命令与单工作流命令组保持单条指针。
func TestSkillHelpGroupListsAllWorkflows(t *testing.T) {
	useRepoSkills(t)
	refsOf := func(path ...string) []string {
		t.Helper()
		c, _, err := rootCmd.Find(path)
		if err != nil {
			t.Fatalf("找不到命令 %v: %v", path, err)
		}
		var out []string
		for _, entry := range relatedSkillEntries(c) {
			out = append(out, entry.Ref.Skill+"/"+entry.Ref.Workflow+":"+strings.Join(entry.Commands, ","))
		}
		return out
	}
	cases := []struct {
		path []string
		want []string
	}{
		{[]string{"doc"}, []string{
			"feishu-cli-docs/export:export,export-file,media-download,media-preview",
			"feishu-cli-docs/import:import",
			"feishu-cli-docs/read:blocks,get,read",
			"feishu-cli-docs/write:add,add-board,add-callout,batch-update,content-update,create,delete,history,media-insert,resource,table,update",
			"feishu-cli-storage/drive:import-file",
			"feishu-cli-visual/htmlbox:htmlbox",
		}},
		{[]string{"msg"}, []string{
			"feishu-cli-messaging/chat:flag,get,history,list,mget,pin,pins,reaction,read-users,resource-download,search-chats,thread-messages,unpin",
			"feishu-cli-messaging/msg:delete,edit,forward,merge-forward,reply,send,urgent",
		}},
		// 叶子命令：只有自身归属，不带子命令列表
		{[]string{"doc", "import"}, []string{"feishu-cli-docs/import:"}},
		{[]string{"msg", "send"}, []string{"feishu-cli-messaging/msg:"}},
		// 单工作流命令组：仍是一条指针
		{[]string{"doc", "table"}, []string{"feishu-cli-docs/write:delete-columns,delete-rows,insert-column,insert-row,merge-cells,unmerge-cells"}},
	}
	for _, tc := range cases {
		got := refsOf(tc.path...)
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("%v 的相关技能 = \n%s\nwant\n%s", tc.path, strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		}
	}

	docCmd, _, _ := rootCmd.Find([]string{"doc"})
	help := relatedSkillHelp(docCmd)
	for _, want := range []string{
		"本命令组的子命令分布在 6 个工作流",
		"feishu-cli-docs（references/workflows/import/workflow.md）: import\n",
		"feishu-cli-visual（references/workflows/htmlbox/workflow.md）: htmlbox\n",
		"feishu-cli-storage（references/workflows/drive/workflow.md）: import-file\n",
		"离线读取: feishu-cli skills read <技能> <工作流路径>",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("doc --help 技能指针缺少 %q:\n%s", want, help)
		}
	}
	if relatedSkillHelp(docCmd) != help {
		t.Fatal("同一命令组的技能指针输出应稳定")
	}
	leaf, _, _ := rootCmd.Find([]string{"doc", "import"})
	if got := relatedSkillHelp(leaf); strings.Count(got, "（references/workflows/") != 1 || strings.Contains(got, "本命令组") {
		t.Fatalf("叶子命令应保持单条指针: %q", got)
	}
}

func TestSkillHelpPointerAppearsInHelpOutput(t *testing.T) {
	useRepoSkills(t)
	installSkillHelpPointers(rootCmd)
	installSkillHelpPointers(rootCmd) // 幂等：不能重复追加

	out, err := runSkillsCLI(t, "doc", "import", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "相关技能:") != 1 {
		t.Fatalf("相关技能段应恰好出现一次:\n%s", out)
	}
	if !strings.Contains(out, "feishu-cli-docs（references/workflows/import/workflow.md）") {
		t.Fatalf("doc import --help 缺少技能指针:\n%s", out)
	}
	// 段落必须在 Global Flags 之后、以无缩进标题开始，避免被 --help 解析脚本当作 flag
	if strings.Index(out, "相关技能:") < strings.Index(out, "Global Flags:") || !strings.Contains(out, "\n相关技能:\n") {
		t.Fatalf("技能指针位置/格式不对:\n%s", out)
	}

	out, err = runSkillsCLI(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "feishu-cli skills list") {
		t.Fatalf("根命令 help 应提示 skills list:\n%s", out)
	}

	out, err = runSkillsCLI(t, "completion", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "相关技能:") {
		t.Fatalf("completion 不归属任何技能，不应输出指针:\n%s", out)
	}
}

func TestSkillHelpWithoutEmbeddedContent(t *testing.T) {
	prev := embeddedSkillsFS()
	SetEmbeddedSkills(nil)
	t.Cleanup(func() { SetEmbeddedSkills(prev) })
	if got := relatedSkillHelp(docCmd); got != "" {
		t.Fatalf("未内嵌技能时不应输出指针: %q", got)
	}
	if got := relatedSkillHelp(rootCmd); got != "" {
		t.Fatalf("未内嵌技能时根命令不应输出指针: %q", got)
	}
}
