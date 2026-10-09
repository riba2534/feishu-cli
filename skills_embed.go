package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/riba2534/feishu-cli/v2/cmd"
)

// embeddedSkills 把领域技能随二进制一起分发，保证技能内容与 CLI 版本严格配套。
//
// 白名单只包含运行期需要的内容：manifest.yaml、各技能 SKILL.md 与 references/
// （工作流、参考资料、脚本、模板、素材）。evals/、trigger-evals*.json 不在白名单内；
// 以 . 或 _ 开头的文件/目录（如 __pycache__）被 go:embed 自动排除；
// 测试脚本（test_*.py）由 internal/skillbundle 的分发过滤器排除，不会被列出、读取或安装。
// go:embed 不能引用上级目录，因此必须放在仓库根的 main 包。
//
//go:embed skills/manifest.yaml skills/feishu-cli-*/SKILL.md skills/feishu-cli-*/references
var embeddedSkills embed.FS

func init() {
	sub, err := fs.Sub(embeddedSkills, "skills")
	if err != nil {
		fmt.Fprintln(os.Stderr, "警告: 内嵌技能装配失败，skills 命令不可用:", err)
		return
	}
	cmd.SetEmbeddedSkills(sub)
}
