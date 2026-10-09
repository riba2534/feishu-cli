package cmd

import (
	"strings"

	"github.com/riba2534/feishu-cli/internal/skillinstall"
)

var doctorSkillsDir string

// checkSkills 检查本地技能目录是否与当前 CLI 内嵌技能一致（只读本地文件，零网络）。
// 目录优先级: --skills-dir > FEISHU_CLI_SKILLS_DIR > ~/.claude/skills。
func checkSkills() checkResult {
	const name = "skills"
	b, err := loadSkillBundle()
	if err != nil {
		return checkResult{Name: name, Status: "skip", Message: "当前构建未内嵌技能: " + err.Error(), Hint: "使用 make build 或 go build . 从仓库根编译"}
	}
	dir := strings.TrimSpace(doctorSkillsDir)
	if dir == "" {
		if dir, err = skillinstall.DefaultDir(); err != nil {
			return checkWarn(name, err.Error(), "用 --skills-dir 指定技能目录")
		}
	}
	rep := skillinstall.Check(b, dir, version)
	return checkResult{Name: name, Status: string(rep.Status), Message: rep.Message, Hint: rep.Hint}
}

func init() {
	validOnlyNames["skills"] = true
	doctorCmd.Flags().StringVar(&doctorSkillsDir, "skills-dir", "", "skills 检查的技能目录（默认 FEISHU_CLI_SKILLS_DIR 或 ~/.claude/skills）")
}
