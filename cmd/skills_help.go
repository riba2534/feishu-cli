package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/skillbundle"
	"github.com/spf13/cobra"
)

// skillHelpInstalledAnnotation 防止重复包装 help（测试与多次 Execute 时保持幂等）。
const skillHelpInstalledAnnotation = "feishu-cli/skill-help"

// installSkillHelpPointers 在全部命令注册完成后包装根命令的 help：每个命令的 --help 末尾追加
// "相关技能" 段，指向 skills/manifest.yaml owners 声明的领域技能工作流。
// 子命令未自定义 HelpFunc 时会沿父链继承到这里，因此无需逐个修改命令文件。
func installSkillHelpPointers(root *cobra.Command) {
	if root.Annotations[skillHelpInstalledAnnotation] == "1" {
		return
	}
	if root.Annotations == nil {
		root.Annotations = map[string]string{}
	}
	root.Annotations[skillHelpInstalledAnnotation] = "1"
	base := root.HelpFunc()
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		base(c, args)
		if text := relatedSkillHelp(c); text != "" {
			fmt.Fprint(c.OutOrStdout(), text)
		}
	})
}

// commandPathParts 返回去掉根命令名后的命令路径，例如 ["doc", "import"]。
func commandPathParts(c *cobra.Command) []string {
	var parts []string
	for cur := c; cur != nil && cur.HasParent(); cur = cur.Parent() {
		parts = append([]string{cur.Name()}, parts...)
	}
	return parts
}

// relatedSkillRef 返回命令归属的技能工作流；根命令与 completion/help 不归属任何工作流。
func relatedSkillRef(c *cobra.Command) (skillbundle.WorkflowRef, bool) {
	manifest, err := loadSkillManifest()
	if err != nil {
		return skillbundle.WorkflowRef{}, false
	}
	parts := commandPathParts(c)
	if len(parts) == 0 || manifest.IsExcludedCommand(parts) {
		return skillbundle.WorkflowRef{}, false
	}
	return manifest.OwnerOf(parts)
}

// relatedSkillHelp 生成 help 末尾的技能指针段。
// 段落以无缩进的 "相关技能:" 标题开头，与 cobra 的 Flags:/Global Flags: 段同构，
// 解析 --help 的脚本（scripts/skill_command_contracts.py）会把它识别为独立段落而不是 flag。
func relatedSkillHelp(c *cobra.Command) string {
	if !c.HasParent() {
		if embeddedSkillsFS() == nil {
			return ""
		}
		return "\n相关技能:\n  feishu-cli skills list 查看与本版本配套的领域技能；feishu-cli skills install 安装到 ~/.claude/skills\n"
	}
	ref, ok := relatedSkillRef(c)
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n相关技能:\n")
	fmt.Fprintf(&b, "  %s（%s）\n", ref.Skill, ref.Path())
	fmt.Fprintf(&b, "  离线读取: feishu-cli skills read %s %s\n", ref.Skill, ref.Path())
	return b.String()
}
