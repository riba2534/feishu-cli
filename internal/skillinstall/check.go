package skillinstall

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/skillbundle"
)

// CheckStatus 与 doctor 的状态值保持一致。
type CheckStatus string

const (
	CheckPass CheckStatus = "pass"
	CheckWarn CheckStatus = "warn"
	CheckSkip CheckStatus = "skip"
)

// CheckReport 是本地技能与当前 CLI 内嵌技能的一致性检查结果（只读、零网络）。
type CheckReport struct {
	Status           CheckStatus `json:"status"`
	Message          string      `json:"message"`
	Hint             string      `json:"hint,omitempty"`
	Dir              ResolvedDir `json:"dir"`
	CLIVersion       string      `json:"cli_version"`
	InstalledVersion string      `json:"installed_version,omitempty"`
	Stale            []string    `json:"stale,omitempty"`
	Modified         []string    `json:"modified,omitempty"`
	External         []string    `json:"external,omitempty"`
	Legacy           []string    `json:"legacy,omitempty"`
}

// Check 检查 dir 下已安装技能是否与当前 CLI 内嵌内容一致。
func Check(b *skillbundle.Bundle, dir, cliVersion string) CheckReport {
	rep := CheckReport{CLIVersion: cliVersion}
	installCmd := installCommand(dir)
	resolved, err := ResolveDir(dir)
	if err != nil {
		rep.Status = CheckWarn
		rep.Message = fmt.Sprintf("无法解析技能目录 %s: %v", dir, err)
		rep.Hint = "用 --skills-dir 或 FEISHU_CLI_SKILLS_DIR 指定正确的技能目录"
		return rep
	}
	rep.Dir = resolved
	location := describeDir(resolved)
	if !resolved.Exists {
		rep.Status = CheckSkip
		rep.Message = fmt.Sprintf("技能目录 %s 不存在，未安装 feishu-cli 技能", location)
		rep.Hint = "AI Agent 需要技能时运行: " + installCmd
		return rep
	}
	plan, err := BuildPlan(b, resolved, cliVersion)
	if err != nil {
		rep.Status = CheckWarn
		rep.Message = fmt.Sprintf("扫描技能目录 %s 失败: %v", location, err)
		return rep
	}
	present := 0
	var unmanagedDiff []string
	for _, sp := range plan.Skills {
		switch sp.Action {
		case ActionCreate:
			rep.Stale = append(rep.Stale, sp.Name)
			continue
		case ActionUpdate:
			rep.Stale = append(rep.Stale, sp.Name)
		case ActionConflict:
			rep.Modified = append(rep.Modified, sp.Name)
			if !sp.Managed {
				unmanagedDiff = append(unmanagedDiff, sp.Name)
			}
		case ActionExternal:
			rep.External = append(rep.External, sp.Name)
		}
		present++
	}
	for _, lp := range plan.Legacy {
		if lp.Status == LegacyConfirmed {
			rep.Legacy = append(rep.Legacy, lp.Name)
		}
	}
	if plan.State != nil {
		rep.InstalledVersion = plan.State.CLIVersion
	}
	if present == 0 {
		rep.Status = CheckSkip
		rep.Message = fmt.Sprintf("%s 中未发现 feishu-cli 技能", location)
		rep.Hint = "AI Agent 需要技能时运行: " + installCmd
		if len(rep.Legacy) > 0 {
			rep.Status = CheckWarn
			rep.Message += fmt.Sprintf("，但存在 %d 个旧版技能目录（%s）", len(rep.Legacy), strings.Join(rep.Legacy, ", "))
			rep.Hint = installCmd + " --prune-legacy（安装新版 9 个领域技能并清理已确认的旧目录）"
		}
		rep.Stale = nil
		return rep
	}

	total := len(plan.Skills)
	var problems []string
	var hints []string
	switch {
	case plan.StateError != "":
		problems = append(problems, "安装状态文件损坏: "+plan.StateError)
	case plan.State == nil:
		problems = append(problems, "没有 feishu-cli 安装记录（可能来自 npx skills add 或手动复制）")
	}
	if len(rep.Stale) > 0 {
		problems = append(problems, fmt.Sprintf("%d/%d 个技能与当前 CLI 内嵌内容不一致或缺失（%s）", len(rep.Stale), total, strings.Join(rep.Stale, ", ")))
	}
	if n := len(rep.Modified) - len(unmanagedDiff); n > 0 {
		var managed []string
		for _, name := range rep.Modified {
			if !contains(unmanagedDiff, name) {
				managed = append(managed, name)
			}
		}
		problems = append(problems, fmt.Sprintf("%d 个技能在上次安装后被本地修改（%s）", n, strings.Join(managed, ", ")))
	}
	if len(unmanagedDiff) > 0 {
		problems = append(problems, fmt.Sprintf("%d 个技能内容与当前 CLI 内嵌版本不同，无法区分是旧版本还是本地修改（%s）", len(unmanagedDiff), strings.Join(unmanagedDiff, ", ")))
	}
	if len(rep.Legacy) > 0 {
		problems = append(problems, fmt.Sprintf("存在 %d 个旧版技能目录（%s）", len(rep.Legacy), strings.Join(rep.Legacy, ", ")))
	}
	versionNote := fmt.Sprintf("当前 CLI %s", cliVersion)
	if rep.InstalledVersion != "" {
		versionNote = fmt.Sprintf("安装时 CLI %s，当前 CLI %s", rep.InstalledVersion, cliVersion)
	}
	externalNote := ""
	if len(rep.External) > 0 {
		externalNote = fmt.Sprintf("；%d 个技能目录是符号链接（外部管理，未比对: %s）", len(rep.External), strings.Join(rep.External, ", "))
	}

	// 仅"无安装记录但内容完全一致"属于可接受状态
	onlyUnrecorded := plan.State == nil && plan.StateError == "" && len(rep.Stale) == 0 && len(rep.Modified) == 0 && len(rep.Legacy) == 0
	if len(problems) == 0 || onlyUnrecorded {
		rep.Status = CheckPass
		rep.Message = fmt.Sprintf("%s 中的技能与 CLI 内嵌内容一致（%s）%s", location, versionNote, externalNote)
		if onlyUnrecorded {
			rep.Message = fmt.Sprintf("%s 中的技能内容与 CLI %s 内嵌版本一致，但没有安装记录%s", location, cliVersion, externalNote)
			rep.Hint = "运行 " + installCmd + " 写入安装记录，便于以后检测漂移"
		}
		return rep
	}

	rep.Status = CheckWarn
	rep.Message = fmt.Sprintf("%s：%s（%s）%s", location, strings.Join(problems, "；"), versionNote, externalNote)
	switch {
	case len(rep.Modified) > 0 || plan.State == nil:
		hints = append(hints, "先运行 "+installCmd+" --dry-run 查看差异；确认本地改动可丢弃后加 --force 覆盖")
	case len(rep.Stale) > 0:
		hints = append(hints, "运行 "+installCmd+" 同步到当前 CLI 版本")
	}
	if len(rep.Legacy) > 0 {
		if len(hints) == 0 {
			hints = append(hints, "运行 "+installCmd+" --prune-legacy 清理已确认的旧技能目录")
		} else {
			hints = append(hints, "加 --prune-legacy 清理已确认的旧技能目录")
		}
	}
	rep.Hint = strings.Join(hints, "；")
	return rep
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

func installCommand(dir string) string {
	def, err := DefaultDir()
	if err == nil && dir == def {
		return "feishu-cli skills install"
	}
	return fmt.Sprintf("feishu-cli skills install --dir %s", shellQuote(dir))
}

func describeDir(r ResolvedDir) string {
	if r.Symlinked() {
		return fmt.Sprintf("%s（实际路径 %s）", r.Abs, r.Real)
	}
	return r.Abs
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"$`\\*?[]{}()<>|&;#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
