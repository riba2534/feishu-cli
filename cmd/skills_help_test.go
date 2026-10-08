package cmd

import (
	"io/fs"
	"strings"
	"testing"

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
// 且指向的工作流文件真实存在于（将被内嵌的）技能内容中。
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
		ref, ok := relatedSkillRef(c)
		if !ok {
			t.Errorf("命令 %q 没有唯一的相关技能归属，请在 skills/manifest.yaml owners 中声明", c.CommandPath())
			return
		}
		target := ref.Skill + "/" + ref.Path()
		if info, err := fs.Stat(fsys, target); err != nil || info.IsDir() {
			t.Errorf("命令 %q 指向的技能文件不存在: %s", c.CommandPath(), target)
			return
		}
		help := relatedSkillHelp(c)
		if !strings.Contains(help, "相关技能:") || !strings.Contains(help, "feishu-cli skills read "+ref.Skill+" "+ref.Path()) {
			t.Errorf("命令 %q 的技能指针格式不对: %q", c.CommandPath(), help)
		}
		checked++
	})
	if checked < 400 {
		t.Fatalf("只检查了 %d 个命令，命令树可能未完整注册", checked)
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
