package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"

	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/riba2534/feishu-cli/v2/internal/skillbundle"
	"github.com/riba2534/feishu-cli/v2/internal/skillinstall"
	"github.com/spf13/cobra"
)

// skipConfigInitAnnotation 标记无需加载 config.yaml 的纯本地命令（技能读取/安装、版本更新）。
// 配置损坏时这些命令仍应可用；由 root.go 的 shouldSkipConfigInit 识别。
const skipConfigInitAnnotation = "feishu-cli/skip-config-init"

var skillContent struct {
	sync.Mutex
	fsys     fs.FS
	bundle   *skillbundle.Bundle
	manifest *skillbundle.Manifest
}

// SetEmbeddedSkills 由 main 包注入编译期内嵌的技能根目录（含 manifest.yaml 与 9 个领域技能）。
func SetEmbeddedSkills(fsys fs.FS) {
	skillContent.Lock()
	defer skillContent.Unlock()
	skillContent.fsys = fsys
	skillContent.bundle = nil
	skillContent.manifest = nil
}

func embeddedSkillsFS() fs.FS {
	skillContent.Lock()
	defer skillContent.Unlock()
	return skillContent.fsys
}

// loadSkillBundle 懒加载并缓存内嵌技能（含全部文件哈希，仅 skills/doctor 使用）。
func loadSkillBundle() (*skillbundle.Bundle, error) {
	skillContent.Lock()
	defer skillContent.Unlock()
	if skillContent.bundle != nil {
		return skillContent.bundle, nil
	}
	b, err := skillbundle.Load(skillContent.fsys)
	if err != nil {
		return nil, err
	}
	skillContent.bundle = b
	skillContent.manifest = b.Manifest
	return b, nil
}

// loadSkillManifest 只解析 manifest.yaml（--help 指针用，不计算哈希）。
func loadSkillManifest() (*skillbundle.Manifest, error) {
	skillContent.Lock()
	defer skillContent.Unlock()
	if skillContent.manifest != nil {
		return skillContent.manifest, nil
	}
	if skillContent.fsys == nil {
		return nil, errors.New("当前构建未内嵌技能内容")
	}
	m, err := skillbundle.LoadManifest(skillContent.fsys)
	if err != nil {
		return nil, err
	}
	skillContent.manifest = m
	return m, nil
}

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "与当前 CLI 版本配套的 AI 技能（列出 / 读取 / 安装）",
	Long: `9 个领域技能（SKILL.md + references 工作流 + 脚本/模板/素材）在编译期内嵌进二进制，
内容与当前 CLI 版本严格配套。AI Agent 可以直接读取内嵌内容，或安装到本地技能目录。

子命令:
  list      列出内嵌技能，或列出某个技能内的文件
  read      输出内嵌技能文件内容（默认 SKILL.md）
  install   安装/同步到本地技能目录（默认 ~/.claude/skills）

技能目录优先级: --dir > FEISHU_CLI_SKILLS_DIR > ~/.claude/skills。
安装状态记录在 <技能目录>/.feishu-cli-skills.json；feishu-cli doctor 会据此检查技能是否与 CLI 版本漂移。`,
}

var (
	skillsListJSON     bool
	skillsInstallDir   string
	skillsInstallDry   bool
	skillsInstallForce bool
	skillsInstallPrune bool
	skillsInstallJSON  bool
)

var skillsListCmd = &cobra.Command{
	Use:   "list [技能[/子目录]]",
	Short: "列出内嵌技能，或列出某个技能内的文件",
	Long: `不带参数时列出全部内嵌技能（文件数、大小、内容哈希、工作流）。
带技能名（可附子目录）时列出该范围内的全部文件，路径可直接交给 skills read。`,
	Example: `  feishu-cli skills list
  feishu-cli skills list --json
  feishu-cli skills list feishu-cli-docs
  feishu-cli skills list feishu-cli-docs/references/workflows/import`,
	Args:        cobra.MaximumNArgs(1),
	Annotations: map[string]string{skipConfigInitAnnotation: "1"},
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := loadSkillBundle()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if len(args) == 1 {
			name, sub, _ := strings.Cut(strings.Trim(args[0], "/"), "/")
			files, err := b.ListFiles(name, sub)
			if err != nil {
				return err
			}
			if skillsListJSON {
				return writeJSON(out, map[string]any{"skill": name, "path": sub, "files": files, "count": len(files)})
			}
			rows := make([][]string, 0, len(files))
			for _, f := range files {
				rows = append(rows, []string{f.Path, humanBytes(f.Size)})
			}
			if err := renderColumns(out, []string{"路径", "大小"}, rows); err != nil {
				return err
			}
			fmt.Fprintf(out, "\n共 %d 个文件。读取: feishu-cli skills read %s <路径>\n", len(files), name)
			return nil
		}
		if skillsListJSON {
			return writeJSON(out, map[string]any{
				"cli_version": version,
				"bundle_hash": b.Hash,
				"skills":      b.Skills(),
				"count":       len(b.Skills()),
			})
		}
		var total int64
		files := 0
		for _, s := range b.Skills() {
			total += s.TotalBytes
			files += s.FileCount
		}
		fmt.Fprintf(out, "feishu-cli %s 内嵌 %d 个领域技能（%d 个文件，%s，bundle %s）\n\n", version, len(b.Skills()), files, humanBytes(total), shortHash(b.Hash))
		rows := make([][]string, 0, len(b.Skills()))
		for _, s := range b.Skills() {
			rows = append(rows, []string{s.Name, fmt.Sprint(s.FileCount), humanBytes(s.TotalBytes), shortHash(s.Hash), strings.Join(s.Workflows, ", ")})
		}
		if err := renderColumns(out, []string{"技能", "文件", "大小", "内容哈希", "工作流"}, rows); err != nil {
			return err
		}
		fmt.Fprintln(out, "\n读取: feishu-cli skills read <技能> [相对路径]    安装: feishu-cli skills install [--dir <目录>]")
		return nil
	},
}

var skillsReadCmd = &cobra.Command{
	Use:   "read <技能>[/<相对路径>] [相对路径]",
	Short: "输出内嵌技能文件内容（默认 SKILL.md）",
	Long: `把内嵌技能文件原样写到 stdout（不追加任何内容），与当前 CLI 版本严格一致。

路径相对技能目录；也接受技能 Markdown 中的跨技能写法 ../feishu-cli-xxx/...。
不允许绝对路径或越出技能目录；评测与测试文件不随 CLI 分发。`,
	Example: `  feishu-cli skills read feishu-cli-docs
  feishu-cli skills read feishu-cli-docs SKILL.md
  feishu-cli skills read feishu-cli-docs references/workflows/import/workflow.md
  feishu-cli skills read feishu-cli-platform/references/workflows/auth/references/identity.md`,
	Args:        cobra.RangeArgs(1, 2),
	Annotations: map[string]string{skipConfigInitAnnotation: "1"},
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := loadSkillBundle()
		if err != nil {
			return err
		}
		name, rel, _ := strings.Cut(args[0], "/")
		if len(args) == 2 {
			if rel != "" {
				return fmt.Errorf("路径只能给一次：要么写成 %s/<路径>，要么作为第二个参数", name)
			}
			rel = args[1]
		}
		_, _, data, err := b.ReadFile(name, rel)
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(data)
		return err
	},
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "把内嵌技能安装/同步到本地技能目录（默认 ~/.claude/skills）",
	Long: `把与当前 CLI 版本配套的 9 个领域技能写入本地技能目录，并记录安装状态。

目录优先级: --dir > FEISHU_CLI_SKILLS_DIR > ~/.claude/skills。
写入前解析符号链接并打印真实路径（例如 ~/.agents/skills 指向 ~/.claude/skills 时只写一份）。

安全规则:
  - 只覆盖上次由本命令写入且未被本地修改的文件；发现本地修改（或目录没有安装记录且内容不同）时
    不带 --force 整体拒绝，不写入任何文件
  - 非本命令管理的额外文件（如自己加的笔记、旧安装留下的 evals/）始终保留
  - 技能目录本身是符号链接时视为外部管理，任何情况下都不写入、不删除
  - 旧版技能目录（manifest 的 legacy_skill_mappings）默认只列出；加 --prune-legacy 且其 SKILL.md
    的 name 与旧技能名一致时才删除

先用 --dry-run 预览；安装后可用 feishu-cli doctor 检查技能与 CLI 是否漂移。`,
	Example: `  feishu-cli skills install --dry-run
  feishu-cli skills install
  feishu-cli skills install --dir /tmp/skills-test
  feishu-cli skills install --force
  feishu-cli skills install --prune-legacy --dry-run`,
	Args:        cobra.NoArgs,
	Annotations: map[string]string{skipConfigInitAnnotation: "1"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSkillsInstall(cmd.OutOrStdout())
	},
}

func runSkillsInstall(out io.Writer) error {
	b, err := loadSkillBundle()
	if err != nil {
		return err
	}
	dir := strings.TrimSpace(skillsInstallDir)
	if dir == "" {
		if dir, err = skillinstall.DefaultDir(); err != nil {
			return err
		}
	}
	resolved, err := skillinstall.ResolveDir(dir)
	if err != nil {
		return err
	}
	// 技能目录是用户指定的写入位置：解析符号链接后拒绝 ~/.ssh、~/.feishu-cli、/etc 等敏感目录
	if err := safefile.ValidateOutputPath(resolved.Real); err != nil {
		return fmt.Errorf("技能目录无效: %w", err)
	}
	plan, err := skillinstall.BuildPlan(b, resolved, version)
	if err != nil {
		return err
	}
	conflicts := plan.Conflicts()
	if skillsInstallJSON {
		payload := map[string]any{"dry_run": skillsInstallDry, "force": skillsInstallForce, "plan": plan}
		if skillsInstallDry {
			return writeJSON(out, payload)
		}
		res, applyErr := skillinstall.Apply(b, plan, skillinstall.Options{Force: skillsInstallForce, PruneLegacy: skillsInstallPrune})
		payload["result"] = res
		payload["ok"] = applyErr == nil
		if applyErr != nil {
			payload["error"] = applyErr.Error()
		}
		if err := writeJSON(out, payload); err != nil {
			return err
		}
		return applyErr
	}

	printSkillsInstallPlan(out, plan)
	if skillsInstallDry {
		fmt.Fprintln(out, "\n[dry-run] 未写入任何文件。")
		if len(conflicts) > 0 && !skillsInstallForce {
			fmt.Fprintln(out, "实际安装会因上述冲突被拒绝；确认本地改动可丢弃后加 --force。")
		}
		return nil
	}
	res, err := skillinstall.Apply(b, plan, skillinstall.Options{Force: skillsInstallForce, PruneLegacy: skillsInstallPrune})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\n完成: 写入 %d 个文件，删除 %d 个旧文件", res.Written, res.Removed)
	if len(res.Pruned) > 0 {
		fmt.Fprintf(out, "，清理旧技能目录 %s", strings.Join(res.Pruned, ", "))
	}
	if res.StateUnchanged {
		fmt.Fprintf(out, "\n状态文件: %s（无变化）\n", res.StatePath)
	} else {
		fmt.Fprintf(out, "\n状态文件: %s\n", res.StatePath)
	}
	return nil
}

func printSkillsInstallPlan(out io.Writer, plan *skillinstall.Plan) {
	if plan.Dir.Symlinked() {
		fmt.Fprintf(out, "目标目录: %s\n实际写入: %s（已解析符号链接）\n", plan.Dir.Abs, plan.Dir.Real)
	} else {
		fmt.Fprintf(out, "目标目录: %s\n", plan.Dir.Real)
	}
	fmt.Fprintf(out, "CLI 版本: %s，内嵌 %d 个技能（bundle %s）\n", plan.CLIVersion, len(plan.Skills), shortHash(plan.BundleHash))
	switch {
	case plan.StateError != "":
		fmt.Fprintf(out, "安装记录: 无法读取（%s），按无记录处理\n", plan.StateError)
	case plan.State != nil:
		fmt.Fprintf(out, "安装记录: CLI %s 于 %s 安装\n", plan.State.CLIVersion, plan.State.InstalledAt)
	default:
		fmt.Fprintln(out, "安装记录: 无")
	}
	fmt.Fprintln(out)
	rows := make([][]string, 0, len(plan.Skills))
	for _, sp := range plan.Skills {
		rows = append(rows, []string{sp.Name, describeSkillPlan(sp)})
	}
	_ = renderColumns(out, []string{"技能", "计划"}, rows)
	for _, sp := range plan.Skills {
		if sp.Action != skillinstall.ActionConflict || len(sp.Modified) == 0 {
			continue
		}
		fmt.Fprintf(out, "\n%s 的冲突文件（覆盖会丢失本地内容）:\n", sp.Name)
		for i, p := range sp.Modified {
			if i == 10 {
				fmt.Fprintf(out, "    ... 另有 %d 个\n", len(sp.Modified)-10)
				break
			}
			fmt.Fprintf(out, "    %s\n", p)
		}
	}
	if len(plan.Legacy) > 0 {
		fmt.Fprintln(out, "\n旧版技能目录（manifest legacy_skill_mappings）:")
		for _, lp := range plan.Legacy {
			switch lp.Status {
			case skillinstall.LegacyConfirmed:
				action := "加 --prune-legacy 删除"
				if skillsInstallPrune {
					action = "将删除（--prune-legacy）"
				}
				fmt.Fprintf(out, "  %s  已确认是 feishu-cli 旧技能，%s\n", lp.Path, action)
			case skillinstall.LegacySymlink:
				fmt.Fprintf(out, "  %s  是符号链接，跳过（请手动确认指向）\n", lp.Path)
			default:
				found := lp.FoundName
				if found == "" {
					found = "无法识别"
				}
				fmt.Fprintf(out, "  %s  SKILL.md name=%s，不是 feishu-cli 旧技能，保留\n", lp.Path, found)
			}
		}
	}
}

func describeSkillPlan(sp skillinstall.SkillPlan) string {
	var s string
	switch sp.Action {
	case skillinstall.ActionCreate:
		s = fmt.Sprintf("新建（%d 个文件）", len(sp.Write))
	case skillinstall.ActionUpdate:
		s = fmt.Sprintf("更新（写入 %d 个文件", len(sp.Write))
		if len(sp.Remove) > 0 {
			s += fmt.Sprintf("，删除 %d 个旧文件", len(sp.Remove))
		}
		s += "）"
		if !sp.Managed {
			s += "，补齐缺失文件并纳入管理"
		}
	case skillinstall.ActionUnchanged:
		s = "无变化"
		if !sp.Managed {
			s += "（内容一致，纳入管理）"
		}
	case skillinstall.ActionConflict:
		s = "冲突: " + sp.Reason
		if skillsInstallForce {
			s += "（--force 将覆盖）"
		}
	case skillinstall.ActionExternal:
		s = fmt.Sprintf("跳过: 符号链接 → %s（外部管理）", sp.LinkTarget)
	}
	if len(sp.Untracked) > 0 {
		s += fmt.Sprintf("；保留 %d 个非本工具管理的文件", len(sp.Untracked))
	}
	return s
}

func shortHash(h string) string {
	h = strings.TrimPrefix(h, "sha256:")
	if len(h) > 12 {
		h = h[:12]
	}
	return "sha256:" + h
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func init() {
	skillsListCmd.Flags().BoolVar(&skillsListJSON, "json", false, "输出 JSON")
	skillsInstallCmd.Flags().StringVar(&skillsInstallDir, "dir", "", "技能目录（默认 FEISHU_CLI_SKILLS_DIR 或 ~/.claude/skills）")
	skillsInstallCmd.Flags().BoolVar(&skillsInstallDry, "dry-run", false, "只预览安装计划，不写入任何文件")
	skillsInstallCmd.Flags().BoolVar(&skillsInstallForce, "force", false, "覆盖有本地修改或无安装记录的技能文件（非本工具管理的额外文件仍保留）")
	skillsInstallCmd.Flags().BoolVar(&skillsInstallPrune, "prune-legacy", false, "删除已确认的旧版技能目录（SKILL.md name 与旧技能名一致）")
	skillsInstallCmd.Flags().BoolVar(&skillsInstallJSON, "json", false, "输出 JSON（计划与结果）")
	skillsCmd.AddCommand(skillsListCmd, skillsReadCmd, skillsInstallCmd)
	rootCmd.AddCommand(skillsCmd)
}
