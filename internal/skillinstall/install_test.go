package skillinstall

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/skillbundle"
)

const manifestJSON = `{
  "version": 1,
  "expected_top_level_skills": ["feishu-cli-alpha", "feishu-cli-beta"],
  "excluded_command_prefixes": [],
  "legacy_skill_mappings": [
    {"legacy_skill": "feishu-cli-old", "skill": "feishu-cli-alpha", "workflow": "one"},
    {"legacy_skill": "feishu-cli-other", "skill": "feishu-cli-alpha", "workflow": "one"},
    {"legacy_skill": "feishu-cli-linked", "skill": "feishu-cli-alpha", "workflow": "one"},
    {"legacy_skill": "feishu-cli-beta", "skill": "feishu-cli-beta", "workflow": "two"}
  ],
  "owners": [
    {"skill": "feishu-cli-alpha", "workflow": "one", "prefixes": [["doc"]]},
    {"skill": "feishu-cli-beta", "workflow": "two", "prefixes": [["msg"]]}
  ]
}`

func baseFS() fstest.MapFS {
	return fstest.MapFS{
		"manifest.yaml":             {Data: []byte(manifestJSON)},
		"feishu-cli-alpha/SKILL.md": {Data: []byte("---\nname: feishu-cli-alpha\ndescription: a\n---\nalpha v1\n")},
		"feishu-cli-alpha/references/workflows/one/workflow.md":      {Data: []byte("one v1")},
		"feishu-cli-alpha/references/workflows/one/scripts/run.py":   {Data: []byte("#!/usr/bin/env python3\nprint(1)\n")},
		"feishu-cli-alpha/references/workflows/one/old-reference.md": {Data: []byte("old")},
		"feishu-cli-beta/SKILL.md":                                   {Data: []byte("---\nname: feishu-cli-beta\ndescription: b\n---\nbeta v1\n")},
		"feishu-cli-beta/references/workflows/two/workflow.md":       {Data: []byte("two v1")},
	}
}

func mustBundle(t *testing.T, fsys fstest.MapFS) *skillbundle.Bundle {
	t.Helper()
	b, err := skillbundle.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func plan(t *testing.T, b *skillbundle.Bundle, dir, version string) *Plan {
	t.Helper()
	resolved, err := ResolveDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := BuildPlan(b, resolved, version)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func install(t *testing.T, b *skillbundle.Bundle, dir, version string, opts Options) (*Plan, *Result, error) {
	t.Helper()
	p := plan(t, b, dir, version)
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC) }
	}
	res, err := Apply(b, p, opts)
	return p, res, err
}

func actions(p *Plan) map[string]Action {
	out := map[string]Action{}
	for _, s := range p.Skills {
		out[s.Name] = s.Action
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestFreshInstallIsIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	b := mustBundle(t, baseFS())
	p, res, err := install(t, b, dir, "v1.0.0", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := actions(p); got["feishu-cli-alpha"] != ActionCreate || got["feishu-cli-beta"] != ActionCreate {
		t.Fatalf("actions = %v", got)
	}
	if res.Written != 6 {
		t.Fatalf("written = %d", res.Written)
	}
	if got := readFile(t, filepath.Join(dir, "feishu-cli-alpha/references/workflows/one/workflow.md")); got != "one v1" {
		t.Fatalf("content = %q", got)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(dir, "feishu-cli-alpha/references/workflows/one/scripts/run.py"))
		if info.Mode().Perm()&0o100 == 0 {
			t.Fatalf("shebang script should be executable, mode=%v", info.Mode())
		}
	}
	st, err := ReadState(dir)
	if err != nil || st == nil {
		t.Fatalf("state = %v %v", st, err)
	}
	if st.CLIVersion != "v1.0.0" || st.BundleHash != b.Hash || len(st.Skills) != 2 || st.InstalledAt != "2026-10-08T00:00:00Z" {
		t.Fatalf("unexpected state: %+v", st)
	}

	// 再次安装：全部无变化，不写任何技能文件
	p2, res2, err := install(t, b, dir, "v1.0.0", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := actions(p2); got["feishu-cli-alpha"] != ActionUnchanged || got["feishu-cli-beta"] != ActionUnchanged {
		t.Fatalf("second run actions = %v", got)
	}
	if res2.Written != 0 || res2.Removed != 0 || !res2.StateUnchanged {
		t.Fatalf("second run should not write: %+v", res2)
	}
	if st2, _ := ReadState(dir); st2.InstalledAt != st.InstalledAt {
		t.Fatal("idempotent reinstall must keep the original state file")
	}
	// CLI 版本变化但内容一致：更新状态文件中的版本
	if _, res3, err := install(t, b, dir, "v1.0.1", Options{}); err != nil || res3.StateUnchanged {
		t.Fatalf("version bump should refresh state: %+v %v", res3, err)
	}
}

func TestLocalModificationBlocksWithoutForce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	b := mustBundle(t, baseFS())
	if _, _, err := install(t, b, dir, "v1.0.0", Options{}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "feishu-cli-alpha/SKILL.md")
	if err := os.WriteFile(target, []byte("my local notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 用户自己加的文件始终保留
	extra := filepath.Join(dir, "feishu-cli-alpha/NOTES.md")
	if err := os.WriteFile(extra, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	fsys := baseFS()
	fsys["feishu-cli-beta/references/workflows/two/workflow.md"] = &fstest.MapFile{Data: []byte("two v2")}
	b2 := mustBundle(t, fsys)
	stateBefore := readFile(t, filepath.Join(dir, StateFileName))

	p, _, err := install(t, b2, dir, "v1.1.0", Options{})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	conflicts := p.Conflicts()
	if len(conflicts) != 1 || conflicts[0].Name != "feishu-cli-alpha" || strings.Join(conflicts[0].Modified, ",") != "SKILL.md" {
		t.Fatalf("conflicts = %+v", conflicts)
	}
	// 冲突时整体不写：beta 也不能被更新，状态文件不变
	if got := readFile(t, filepath.Join(dir, "feishu-cli-beta/references/workflows/two/workflow.md")); got != "two v1" {
		t.Fatalf("beta must not be updated on conflict, got %q", got)
	}
	if readFile(t, target) != "my local notes" || readFile(t, filepath.Join(dir, StateFileName)) != stateBefore {
		t.Fatal("conflict must not touch files or state")
	}

	_, res, err := install(t, b2, dir, "v1.1.0", Options{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Written != 2 {
		t.Fatalf("force should overwrite SKILL.md and beta workflow, written=%d", res.Written)
	}
	if !strings.Contains(readFile(t, target), "alpha v1") || readFile(t, extra) != "mine" {
		t.Fatal("force must restore managed file and keep untracked file")
	}
	st, _ := ReadState(dir)
	if st.CLIVersion != "v1.1.0" || st.BundleHash != b2.Hash {
		t.Fatalf("state not updated: %+v", st)
	}
}

func TestUpdateRemovesObsoleteManagedFilesOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	if _, _, err := install(t, mustBundle(t, baseFS()), dir, "v1.0.0", Options{}); err != nil {
		t.Fatal(err)
	}
	fsys := baseFS()
	delete(fsys, "feishu-cli-alpha/references/workflows/one/old-reference.md")
	delete(fsys, "feishu-cli-alpha/references/workflows/one/scripts/run.py")
	b2 := mustBundle(t, fsys)
	// run.py 被本地改过：新版本不再分发，但不能删，应作为未管理文件保留
	modified := filepath.Join(dir, "feishu-cli-alpha/references/workflows/one/scripts/run.py")
	if err := os.WriteFile(modified, []byte("# changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, res, err := install(t, b2, dir, "v1.1.0", Options{})
	if err != nil {
		t.Fatal(err)
	}
	var alpha SkillPlan
	for _, s := range p.Skills {
		if s.Name == "feishu-cli-alpha" {
			alpha = s
		}
	}
	if alpha.Action != ActionUpdate || strings.Join(alpha.Remove, ",") != "references/workflows/one/old-reference.md" {
		t.Fatalf("alpha plan = %+v", alpha)
	}
	if strings.Join(alpha.Untracked, ",") != "references/workflows/one/scripts/run.py" {
		t.Fatalf("modified obsolete file should be kept as untracked: %+v", alpha)
	}
	if res.Removed != 1 {
		t.Fatalf("removed = %d", res.Removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "feishu-cli-alpha/references/workflows/one/old-reference.md")); !os.IsNotExist(err) {
		t.Fatal("obsolete managed file should be removed")
	}
	if readFile(t, modified) != "# changed" {
		t.Fatal("modified obsolete file must be kept")
	}
}

func TestUnmanagedExistingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	b := mustBundle(t, baseFS())
	// 模拟 npx / 手动复制：与内嵌内容完全一致，外加 evals 目录
	for _, s := range b.Skills() {
		for _, f := range s.Files {
			_, _, data, _ := b.ReadFile(s.Name, f.Path)
			p := filepath.Join(dir, s.Name, filepath.FromSlash(f.Path))
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			if err := os.WriteFile(p, data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	_ = os.MkdirAll(filepath.Join(dir, "feishu-cli-alpha/evals"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "feishu-cli-alpha/evals/evals.json"), []byte("{}"), 0o644)

	p := plan(t, b, dir, "v1.0.0")
	if got := actions(p); got["feishu-cli-alpha"] != ActionUnchanged || got["feishu-cli-beta"] != ActionUnchanged {
		t.Fatalf("identical unmanaged dirs should be adoptable: %v", got)
	}
	if rep := Check(b, dir, "v1.0.0"); rep.Status != CheckPass || !strings.Contains(rep.Message, "没有安装记录") {
		t.Fatalf("check = %+v", rep)
	}

	// 内容不同的未管理目录：必须冲突
	_ = os.WriteFile(filepath.Join(dir, "feishu-cli-beta/SKILL.md"), []byte("older version"), 0o644)
	p = plan(t, b, dir, "v1.0.0")
	if got := actions(p); got["feishu-cli-beta"] != ActionConflict {
		t.Fatalf("differing unmanaged dir must conflict: %v", got)
	}
	if rep := Check(b, dir, "v1.0.0"); rep.Status != CheckWarn || !strings.Contains(rep.Hint, "--force") {
		t.Fatalf("check = %+v", rep)
	}
	if _, _, err := install(t, b, dir, "v1.0.0", Options{Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "feishu-cli-alpha/evals/evals.json")); err != nil {
		t.Fatal("untracked evals from previous install must be kept")
	}
}

func TestSymlinkedRootResolvesToRealPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink 需要特权")
	}
	tmp := t.TempDir()
	real := filepath.Join(tmp, "claude", "skills")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "agents-skills")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveDir(link)
	if err != nil {
		t.Fatal(err)
	}
	realResolved, _ := filepath.EvalSymlinks(real)
	if resolved.Real != realResolved || !resolved.Symlinked() || !resolved.Exists {
		t.Fatalf("resolved = %+v", resolved)
	}
	// 不存在的子路径：解析最深已存在祖先后拼回
	nested, err := ResolveDir(filepath.Join(link, "sub", "dir"))
	if err != nil {
		t.Fatal(err)
	}
	if nested.Real != filepath.Join(realResolved, "sub", "dir") || nested.Exists {
		t.Fatalf("nested = %+v", nested)
	}

	b := mustBundle(t, baseFS())
	if _, _, err := install(t, b, link, "v1.0.0", Options{}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("root symlink must stay a symlink")
	}
	if _, err := os.Stat(filepath.Join(real, StateFileName)); err != nil {
		t.Fatal("state must be written under the real path")
	}
	// 通过真实路径和链接路径看到的是同一份数据：两种入口都应判定无变化
	for _, d := range []string{link, real} {
		if got := actions(plan(t, b, d, "v1.0.0")); got["feishu-cli-alpha"] != ActionUnchanged {
			t.Fatalf("%s actions = %v", d, got)
		}
	}
}

func TestSymlinkedSkillDirIsNeverWritten(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink 需要特权")
	}
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "skills")
	external := filepath.Join(tmp, "repo", "feishu-cli-alpha")
	_ = os.MkdirAll(external, 0o755)
	_ = os.WriteFile(filepath.Join(external, "SKILL.md"), []byte("dev checkout"), 0o644)
	_ = os.MkdirAll(dir, 0o755)
	if err := os.Symlink(external, filepath.Join(dir, "feishu-cli-alpha")); err != nil {
		t.Fatal(err)
	}
	b := mustBundle(t, baseFS())
	p, _, err := install(t, b, dir, "v1.0.0", Options{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if actions(p)["feishu-cli-alpha"] != ActionExternal {
		t.Fatalf("actions = %v", actions(p))
	}
	if readFile(t, filepath.Join(external, "SKILL.md")) != "dev checkout" {
		t.Fatal("symlinked skill dir target must never be modified")
	}
	st, _ := ReadState(dir)
	if _, ok := st.Skills["feishu-cli-alpha"]; ok {
		t.Fatal("external skill must not be recorded as managed")
	}
	rep := Check(b, dir, "v1.0.0")
	if rep.Status != CheckPass || strings.Join(rep.External, ",") != "feishu-cli-alpha" {
		t.Fatalf("check = %+v", rep)
	}

	// 技能目录内部的父目录是符号链接：即使 --force 也拒绝写穿
	_ = os.RemoveAll(filepath.Join(dir, "feishu-cli-beta"))
	outside := filepath.Join(tmp, "outside")
	_ = os.MkdirAll(outside, 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "feishu-cli-beta"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "feishu-cli-beta/SKILL.md"), []byte("---\nname: feishu-cli-beta\ndescription: b\n---\nbeta v1\n"), 0o644)
	if err := os.Symlink(outside, filepath.Join(dir, "feishu-cli-beta/references")); err != nil {
		t.Fatal(err)
	}
	p = plan(t, b, dir, "v1.0.0")
	if actions(p)["feishu-cli-beta"] != ActionConflict {
		t.Fatalf("symlinked ancestor must conflict: %v", actions(p))
	}
	if _, err := Apply(b, p, Options{Force: true}); err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("force must still refuse to write through symlink, err=%v", err)
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatalf("nothing may be written outside, got %v", entries)
	}
}

func TestLegacyPruneOnlyConfirmedDirs(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "skills")
	mk := func(name, skillName string) string {
		p := filepath.Join(dir, name)
		_ = os.MkdirAll(p, 0o755)
		if skillName != "" {
			_ = os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("---\nname: "+skillName+"\ndescription: x\n---\n"), 0o644)
		}
		return p
	}
	confirmed := mk("feishu-cli-old", "feishu-cli-old")
	foreign := mk("feishu-cli-other", "someone-else")
	linkTarget := mk("elsewhere", "feishu-cli-linked")
	linked := filepath.Join(dir, "feishu-cli-linked")
	if runtime.GOOS != "windows" {
		if err := os.Symlink(linkTarget, linked); err != nil {
			t.Fatal(err)
		}
	}
	b := mustBundle(t, baseFS())

	p, res, err := install(t, b, dir, "v1.0.0", Options{})
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]LegacyStatus{}
	for _, lp := range p.Legacy {
		status[lp.Name] = lp.Status
	}
	if status["feishu-cli-old"] != LegacyConfirmed || status["feishu-cli-other"] != LegacyForeign {
		t.Fatalf("legacy status = %v", status)
	}
	if runtime.GOOS != "windows" && status["feishu-cli-linked"] != LegacySymlink {
		t.Fatalf("legacy status = %v", status)
	}
	if _, ok := status["feishu-cli-beta"]; ok {
		t.Fatal("current skill name must never be treated as legacy")
	}
	if len(res.Pruned) != 0 {
		t.Fatal("default install must only list legacy dirs")
	}
	if _, err := os.Stat(confirmed); err != nil {
		t.Fatal("legacy dir must survive without --prune-legacy")
	}
	if rep := Check(b, dir, "v1.0.0"); rep.Status != CheckWarn || strings.Join(rep.Legacy, ",") != "feishu-cli-old" {
		t.Fatalf("check = %+v", rep)
	}

	_, res, err = install(t, b, dir, "v1.0.0", Options{PruneLegacy: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Pruned, ",") != "feishu-cli-old" {
		t.Fatalf("pruned = %v", res.Pruned)
	}
	if _, err := os.Stat(confirmed); !os.IsNotExist(err) {
		t.Fatal("confirmed legacy dir should be removed")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("foreign dir must be kept")
	}
	if runtime.GOOS != "windows" {
		if _, err := os.Lstat(linked); err != nil {
			t.Fatal("symlinked legacy dir must be kept")
		}
		if _, err := os.Stat(filepath.Join(linkTarget, "SKILL.md")); err != nil {
			t.Fatal("symlink target must be kept")
		}
	}
}

func TestCheckReportsDrift(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	b := mustBundle(t, baseFS())
	if rep := Check(b, dir, "v1.0.0"); rep.Status != CheckSkip {
		t.Fatalf("missing dir should skip: %+v", rep)
	}
	_ = os.MkdirAll(dir, 0o755)
	if rep := Check(b, dir, "v1.0.0"); rep.Status != CheckSkip || !strings.Contains(rep.Hint, "skills install") {
		t.Fatalf("empty dir should skip with install hint: %+v", rep)
	}
	if _, _, err := install(t, b, dir, "v1.0.0", Options{}); err != nil {
		t.Fatal(err)
	}
	if rep := Check(b, dir, "v1.0.0"); rep.Status != CheckPass {
		t.Fatalf("fresh install should pass: %+v", rep)
	}

	// 升级 CLI 后内嵌内容变化 → warn + 修复建议
	fsys := baseFS()
	fsys["feishu-cli-alpha/SKILL.md"] = &fstest.MapFile{Data: []byte("---\nname: feishu-cli-alpha\ndescription: a\n---\nalpha v2\n")}
	b2 := mustBundle(t, fsys)
	rep := Check(b2, dir, "v1.1.0")
	if rep.Status != CheckWarn || strings.Join(rep.Stale, ",") != "feishu-cli-alpha" || rep.InstalledVersion != "v1.0.0" {
		t.Fatalf("stale check = %+v", rep)
	}
	if !strings.Contains(rep.Message, "v1.0.0") || !strings.Contains(rep.Message, "v1.1.0") || !strings.Contains(rep.Hint, "feishu-cli skills install --dir") {
		t.Fatalf("stale message/hint = %q / %q", rep.Message, rep.Hint)
	}

	// 版本号不同但内容一致 → pass
	if rep := Check(b, dir, "v1.0.1"); rep.Status != CheckPass || !strings.Contains(rep.Message, "安装时 CLI v1.0.0") {
		t.Fatalf("same content new version = %+v", rep)
	}

	// 本地修改 → warn
	_ = os.WriteFile(filepath.Join(dir, "feishu-cli-beta/SKILL.md"), []byte("edited"), 0o644)
	if rep := Check(b, dir, "v1.0.0"); rep.Status != CheckWarn || strings.Join(rep.Modified, ",") != "feishu-cli-beta" {
		t.Fatalf("modified check = %+v", rep)
	}

	// 状态文件损坏 → warn
	_ = os.WriteFile(filepath.Join(dir, StateFileName), []byte("{broken"), 0o644)
	if rep := Check(b, dir, "v1.0.0"); rep.Status != CheckWarn || !strings.Contains(rep.Message, "损坏") {
		t.Fatalf("corrupt state check = %+v", rep)
	}
}

func TestDefaultDirEnvOverride(t *testing.T) {
	t.Setenv(EnvSkillsDir, "/tmp/custom-skills")
	if got, _ := DefaultDir(); got != "/tmp/custom-skills" {
		t.Fatalf("DefaultDir = %s", got)
	}
	t.Setenv(EnvSkillsDir, "")
	home, _ := os.UserHomeDir()
	if got, _ := DefaultDir(); got != filepath.Join(home, ".claude", "skills") {
		t.Fatalf("DefaultDir = %s", got)
	}
}

func TestStateFileIsValidJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	b := mustBundle(t, baseFS())
	if _, _, err := install(t, b, dir, "v1.0.0", Options{}); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, StateFileName))), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema", "tool", "cli_version", "installed_at", "bundle_hash", "skills"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("state missing %s", key)
		}
	}
}
