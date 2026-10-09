package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorSkillsCheck(t *testing.T) {
	useRepoSkills(t)
	if !validOnlyNames["skills"] {
		t.Fatal("doctor --only 应接受 skills")
	}
	dir := filepath.Join(t.TempDir(), "skills")
	doctorSkillsDir = dir
	t.Cleanup(func() { doctorSkillsDir = "" })

	if r := checkSkills(); r.Status != "skip" || !strings.Contains(r.Hint, "skills install --dir") {
		t.Fatalf("未安装时应 skip 并提示安装: %+v", r)
	}
	if out, err := runSkillsCLI(t, "skills", "install", "--dir", dir); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if r := checkSkills(); r.Status != "pass" {
		t.Fatalf("安装后 doctor 应通过: %+v", r)
	}

	edited := filepath.Join(dir, "feishu-cli-docs", "SKILL.md")
	if err := os.WriteFile(edited, []byte("local edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := checkSkills(); r.Status != "warn" || !strings.Contains(r.Message, "feishu-cli-docs") || !strings.Contains(r.Hint, "--force") {
		t.Fatalf("doctor 应报告本地修改: %+v", r)
	}
	if out, err := runSkillsCLI(t, "skills", "install", "--dir", dir, "--force"); err != nil {
		t.Fatalf("--force: %v\n%s", err, out)
	}
	if r := checkSkills(); r.Status != "pass" {
		t.Fatalf("覆盖后 doctor 应通过: %+v", r)
	}
}

func TestDoctorSkillsCheckWithoutEmbeddedContent(t *testing.T) {
	prev := embeddedSkillsFS()
	SetEmbeddedSkills(nil)
	t.Cleanup(func() { SetEmbeddedSkills(prev) })
	if r := checkSkills(); r.Status != "skip" {
		t.Fatalf("未内嵌技能时应 skip: %+v", r)
	}
}
