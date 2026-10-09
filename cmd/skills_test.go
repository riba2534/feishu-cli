package cmd

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/skillinstall"
	"github.com/spf13/cobra"
)

// useRepoSkills 让 cmd 包测试使用仓库 skills/ 目录（与 main 包 go:embed 的内容同源；
// 两者一致性由仓库根 skills_embed_test.go 保证）。
func useRepoSkills(t *testing.T) fs.FS {
	t.Helper()
	prev := embeddedSkillsFS()
	fsys := os.DirFS(filepath.Join("..", "skills"))
	SetEmbeddedSkills(fsys)
	t.Cleanup(func() { SetEmbeddedSkills(prev) })
	return fsys
}

// runSkillsCLI 通过 rootCmd 执行命令并用 SetOut/SetErr 捕获输出（skills 命令只写 cmd.OutOrStdout）。
func runSkillsCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetCommandTreeFlags(rootCmd)
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		resetCommandTreeFlags(rootCmd)
	})
	err := rootCmd.Execute()
	return out.String(), err
}

func TestSkillsWithoutEmbeddedContent(t *testing.T) {
	prev := embeddedSkillsFS()
	SetEmbeddedSkills(nil)
	t.Cleanup(func() { SetEmbeddedSkills(prev) })
	if _, err := runSkillsCLI(t, "skills", "list"); err == nil || !strings.Contains(err.Error(), "未内嵌") {
		t.Fatalf("未内嵌技能时 skills list 应给出中文错误, got %v", err)
	}
}

func TestSkillsListAndRead(t *testing.T) {
	useRepoSkills(t)
	out, err := runSkillsCLI(t, "skills", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"内嵌 9 个领域技能", "feishu-cli-platform", "feishu-cli-docs", "author, export, import, markdown, mindnote, read, write"} {
		if !strings.Contains(out, want) {
			t.Fatalf("skills list 缺少 %q:\n%s", want, out)
		}
	}

	out, err = runSkillsCLI(t, "skills", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		CLIVersion string `json:"cli_version"`
		BundleHash string `json:"bundle_hash"`
		Count      int    `json:"count"`
		Skills     []struct {
			Name      string   `json:"name"`
			FileCount int      `json:"file_count"`
			Workflows []string `json:"workflows"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("skills list --json 不是合法 JSON: %v\n%s", err, out)
	}
	if listed.Count != 9 || !strings.HasPrefix(listed.BundleHash, "sha256:") || listed.Skills[0].FileCount == 0 {
		t.Fatalf("unexpected list json: %+v", listed)
	}

	out, err = runSkillsCLI(t, "skills", "list", "feishu-cli-platform/references/workflows/auth")
	if err != nil || !strings.Contains(out, "references/workflows/auth/references/identity.md") {
		t.Fatalf("skills list <skill>/<dir> = %v\n%s", err, out)
	}

	want, _ := os.ReadFile(filepath.Join("..", "skills", "feishu-cli-docs", "SKILL.md"))
	for _, args := range [][]string{
		{"skills", "read", "feishu-cli-docs"},
		{"skills", "read", "feishu-cli-docs", "SKILL.md"},
		{"skills", "read", "feishu-cli-docs/SKILL.md"},
	} {
		out, err = runSkillsCLI(t, args...)
		if err != nil || out != string(want) {
			t.Fatalf("%v 输出必须与文件逐字节一致 (err=%v)", args, err)
		}
	}
	identity, _ := os.ReadFile(filepath.Join("..", "skills", "feishu-cli-platform", "references", "workflows", "auth", "references", "identity.md"))
	out, err = runSkillsCLI(t, "skills", "read", "feishu-cli-docs", "../feishu-cli-platform/references/workflows/auth/references/identity.md")
	if err != nil || out != string(identity) {
		t.Fatalf("跨技能相对路径读取失败: %v", err)
	}

	for _, bad := range [][]string{
		{"skills", "read", "feishu-cli-nope"},
		{"skills", "read", "feishu-cli-docs", "../../go.mod"},
		{"skills", "read", "feishu-cli-docs", "/etc/hosts"},
		{"skills", "read", "feishu-cli-docs", "evals/evals.json"},
		{"skills", "read", "feishu-cli-docs/SKILL.md", "SKILL.md"},
	} {
		if _, err := runSkillsCLI(t, bad...); err == nil {
			t.Errorf("%v 应失败", bad)
		}
	}
}

func TestSkillsInstallFlowInTempDir(t *testing.T) {
	useRepoSkills(t)
	dir := filepath.Join(t.TempDir(), "skills")

	out, err := runSkillsCLI(t, "skills", "install", "--dir", dir, "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[dry-run]") || !strings.Contains(out, "新建") {
		t.Fatalf("dry-run 输出不对:\n%s", out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("dry-run 不能创建目录")
	}

	out, err = runSkillsCLI(t, "skills", "install", "--dir", dir)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "状态文件: ") {
		t.Fatalf("install 输出缺少状态文件:\n%s", out)
	}
	st, err := skillinstall.ReadState(dir)
	if err != nil || st == nil || len(st.Skills) != 9 || st.CLIVersion != version {
		t.Fatalf("state = %+v, err=%v", st, err)
	}

	out, err = runSkillsCLI(t, "skills", "install", "--dir", dir)
	if err != nil || len(regexp.MustCompile(`feishu-cli-[a-z]+\s+无变化`).FindAllString(out, -1)) != 9 ||
		!strings.Contains(out, "写入 0 个文件") || !strings.Contains(out, ".feishu-cli-skills.json（无变化）") {
		t.Fatalf("重复安装应幂等: %v\n%s", err, out)
	}

	edited := filepath.Join(dir, "feishu-cli-docs", "SKILL.md")
	if err := os.WriteFile(edited, []byte("local edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = runSkillsCLI(t, "skills", "install", "--dir", dir)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("本地修改后应拒绝覆盖: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(edited); string(got) != "local edit" {
		t.Fatal("拒绝时不能改动本地文件")
	}
	out, err = runSkillsCLI(t, "skills", "install", "--dir", dir, "--force", "--json")
	if err != nil {
		t.Fatalf("--force: %v\n%s", err, out)
	}
	var payload struct {
		OK     bool `json:"ok"`
		Result struct {
			Written int `json:"written_files"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil || !payload.OK || payload.Result.Written != 1 {
		t.Fatalf("--force --json = %s (err=%v)", out, err)
	}
}

func TestSkillsCommandsSkipConfigInit(t *testing.T) {
	for _, c := range []*cobra.Command{skillsListCmd, skillsReadCmd, skillsInstallCmd} {
		if !shouldSkipConfigInit(c) {
			t.Errorf("%s 不应依赖 config.yaml", c.CommandPath())
		}
	}
}
