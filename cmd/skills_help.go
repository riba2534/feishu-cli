package cmd

import (
	"fmt"
	"sort"
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

// relatedSkillRef 返回命令自身归属的技能工作流；根命令与 completion/help 不归属任何工作流。
func relatedSkillRef(c *cobra.Command) (skillbundle.WorkflowRef, bool) {
	manifest, err := loadSkillManifest()
	if err != nil {
		return skillbundle.WorkflowRef{}, false
	}
	return ownerOfCommand(manifest, c)
}

func ownerOfCommand(manifest *skillbundle.Manifest, c *cobra.Command) (skillbundle.WorkflowRef, bool) {
	parts := commandPathParts(c)
	if len(parts) == 0 || manifest.IsExcludedCommand(parts) {
		return skillbundle.WorkflowRef{}, false
	}
	return manifest.OwnerOf(parts)
}

// relatedSkillEntry 是 help 技能指针中的一条：一个工作流及命令组内归属它的直接子命令名。
type relatedSkillEntry struct {
	Ref      skillbundle.WorkflowRef
	Commands []string
}

// selfCommandLabel 标记可独立运行的命令组自身（区别于其子命令）。
const selfCommandLabel = "（本命令）"

// relatedSkillEntries 返回命令相关的全部技能工作流，按 (技能, 工作流) 去重排序。
//   - 叶子命令：只有自身归属的工作流（Commands 为空）。
//   - 命令组：汇总全部可见子孙命令的归属，按直接子命令名聚合；命令组本身可独立运行
//     （不是命令组守卫注入的 RunE）时，自身归属也计入。仅作兜底的命令组前缀
//     （如 ["doc"]）不单独列出——子命令分散在多个工作流时，只指一个工作流会误导 Agent。
func relatedSkillEntries(c *cobra.Command) []relatedSkillEntry {
	manifest, err := loadSkillManifest()
	if err != nil {
		return nil
	}
	self, selfOK := ownerOfCommand(manifest, c)
	if !c.HasAvailableSubCommands() {
		if !selfOK {
			return nil
		}
		return []relatedSkillEntry{{Ref: self}}
	}

	byRef := map[skillbundle.WorkflowRef]map[string]bool{}
	add := func(ref skillbundle.WorkflowRef, name string) {
		if byRef[ref] == nil {
			byRef[ref] = map[string]bool{}
		}
		byRef[ref][name] = true
	}
	if selfOK && c.Runnable() && c.Annotations[groupGuardAnnotation] != "1" {
		add(self, selfCommandLabel)
	}
	for _, child := range c.Commands() {
		if !child.IsAvailableCommand() {
			continue
		}
		walkAvailableCommands(child, func(sub *cobra.Command) {
			if ref, ok := ownerOfCommand(manifest, sub); ok {
				add(ref, child.Name())
			}
		})
	}
	if len(byRef) == 0 {
		if !selfOK {
			return nil
		}
		return []relatedSkillEntry{{Ref: self}}
	}

	entries := make([]relatedSkillEntry, 0, len(byRef))
	for ref, names := range byRef {
		entry := relatedSkillEntry{Ref: ref}
		for name := range names {
			entry.Commands = append(entry.Commands, name)
		}
		sort.Slice(entry.Commands, func(i, j int) bool {
			// "（本命令）" 排在子命令名之前
			if (entry.Commands[i] == selfCommandLabel) != (entry.Commands[j] == selfCommandLabel) {
				return entry.Commands[i] == selfCommandLabel
			}
			return entry.Commands[i] < entry.Commands[j]
		})
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Ref.Skill != entries[j].Ref.Skill {
			return entries[i].Ref.Skill < entries[j].Ref.Skill
		}
		return entries[i].Ref.Workflow < entries[j].Ref.Workflow
	})
	return entries
}

// walkAvailableCommands 前序遍历命令及其全部可见子孙命令（跳过隐藏/废弃命令）。
func walkAvailableCommands(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, child := range c.Commands() {
		if child.IsAvailableCommand() {
			walkAvailableCommands(child, fn)
		}
	}
}

// relatedSkillHelp 生成 help 末尾的技能指针段。
// 段落以无缩进的 "相关技能:" 标题开头，与 cobra 的 Flags:/Global Flags: 段同构，
// 解析 --help 的脚本（scripts/skill_command_contracts.py）会把它识别为独立段落而不是 flag。
// 命令组的子命令分布在多个工作流时逐条列出工作流及对应子命令。
func relatedSkillHelp(c *cobra.Command) string {
	if !c.HasParent() {
		if embeddedSkillsFS() == nil {
			return ""
		}
		return "\n相关技能:\n  feishu-cli skills list 查看与本版本配套的领域技能；feishu-cli skills install 安装到 ~/.claude/skills\n"
	}
	entries := relatedSkillEntries(c)
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n相关技能:\n")
	if len(entries) == 1 {
		ref := entries[0].Ref
		fmt.Fprintf(&b, "  %s（%s）\n", ref.Skill, ref.Path())
		fmt.Fprintf(&b, "  离线读取: feishu-cli skills read %s %s\n", ref.Skill, ref.Path())
		return b.String()
	}
	fmt.Fprintf(&b, "  本命令组的子命令分布在 %d 个工作流（冒号后为对应子命令），按要执行的子命令读取:\n", len(entries))
	for _, entry := range entries {
		fmt.Fprintf(&b, "  %s（%s）: %s\n", entry.Ref.Skill, entry.Ref.Path(), strings.Join(entry.Commands, ", "))
	}
	first := entries[0].Ref
	fmt.Fprintf(&b, "  离线读取: feishu-cli skills read <技能> <工作流路径>，例如 feishu-cli skills read %s %s\n", first.Skill, first.Path())
	return b.String()
}
