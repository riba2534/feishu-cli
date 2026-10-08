package skillbundle

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

const testManifest = `{
  "version": 1,
  "expected_top_level_skills": ["feishu-cli-alpha", "feishu-cli-beta"],
  "excluded_command_prefixes": [["completion"], ["help"]],
  "hidden_commands": [],
  "non_command_workflows": [{"skill": "feishu-cli-beta", "workflow": "card"}],
  "legacy_skill_mappings": [
    {"legacy_skill": "feishu-cli-old", "skill": "feishu-cli-alpha", "workflow": "one"},
    {"legacy_skill": "feishu-cli-old", "skill": "feishu-cli-alpha", "workflow": "two"},
    {"legacy_skill": "feishu-cli-beta", "skill": "feishu-cli-beta", "workflow": "card"}
  ],
  "owners": [
    {"skill": "feishu-cli-alpha", "workflow": "one", "prefixes": [["doc"], ["doc", "get"]]},
    {"skill": "feishu-cli-alpha", "workflow": "two", "prefixes": [["doc", "import"]]},
    {"skill": "feishu-cli-beta", "workflow": "three", "prefixes": [["msg"]]},
    {"skill": "feishu-cli-beta", "workflow": "dup", "prefixes": [["msg", "send"]]},
    {"skill": "feishu-cli-alpha", "workflow": "dup2", "prefixes": [["msg", "send"]]}
  ]
}`

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"manifest.yaml":             {Data: []byte(testManifest)},
		"feishu-cli-alpha/SKILL.md": {Data: []byte("---\nname: feishu-cli-alpha\ndescription: >-\n  Alpha 技能\ncompatibility: Requires feishu-cli v1.0.0+\n---\n# Alpha\n")},
		"feishu-cli-alpha/references/workflows/one/workflow.md":               {Data: []byte("one")},
		"feishu-cli-alpha/references/workflows/two/workflow.md":               {Data: []byte("two")},
		"feishu-cli-alpha/references/workflows/two/scripts/run.py":            {Data: []byte("#!/usr/bin/env python3\n")},
		"feishu-cli-alpha/references/workflows/two/scripts/test_run.py":       {Data: []byte("test")},
		"feishu-cli-alpha/evals/evals.json":                                   {Data: []byte("{}")},
		"feishu-cli-beta/SKILL.md":                                            {Data: []byte("---\nname: feishu-cli-beta\ndescription: Beta\n---\n")},
		"feishu-cli-beta/references/workflows/card/workflow.md":               {Data: []byte("card")},
		"feishu-cli-beta/references/workflows/card/assets/icon.png":           {Data: []byte{0x89, 'P', 'N', 'G'}},
		"feishu-cli-beta/references/workflows/card/scripts/__pycache__/x.pyc": {Data: []byte("pyc")},
		"feishu-cli-beta/references/workflows/card/.DS_Store":                 {Data: []byte("ds")},
		"feishu-cli-beta-workspace/SKILL.md":                                  {Data: []byte("---\nname: x\n---\n")},
	}
}

func TestLoadFiltersAndHashes(t *testing.T) {
	b, err := Load(testFS())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(b.SkillNames(), ","); got != "feishu-cli-alpha,feishu-cli-beta" {
		t.Fatalf("skill names = %s", got)
	}
	alpha, _ := b.Skill("feishu-cli-alpha")
	var paths []string
	for _, f := range alpha.Files {
		paths = append(paths, f.Path)
	}
	want := "SKILL.md,references/workflows/one/workflow.md,references/workflows/two/scripts/run.py,references/workflows/two/workflow.md"
	if strings.Join(paths, ",") != want {
		t.Fatalf("alpha files = %v", paths)
	}
	if alpha.Description != "Alpha 技能" || alpha.Compatibility != "Requires feishu-cli v1.0.0+" {
		t.Fatalf("frontmatter = %+v", alpha)
	}
	if strings.Join(alpha.Workflows, ",") != "dup2,one,two" {
		t.Fatalf("workflows = %v", alpha.Workflows)
	}
	beta, _ := b.Skill("feishu-cli-beta")
	if beta.FileCount != 3 {
		t.Fatalf("beta should keep png and skip pycache/dotfiles, got %+v", beta.Files)
	}
	if !strings.HasPrefix(b.Hash, "sha256:") || alpha.Hash == beta.Hash {
		t.Fatalf("unexpected hashes: bundle=%s alpha=%s beta=%s", b.Hash, alpha.Hash, beta.Hash)
	}
	if SkillsHash(map[string]string{"feishu-cli-alpha": alpha.Hash, "feishu-cli-beta": beta.Hash}) != b.Hash {
		t.Fatal("SkillsHash must match Bundle.Hash algorithm")
	}

	// 内容变化必须改变哈希
	fsys := testFS()
	fsys["feishu-cli-alpha/references/workflows/one/workflow.md"] = &fstest.MapFile{Data: []byte("one!")}
	b2, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if b2.Hash == b.Hash {
		t.Fatal("bundle hash should change when content changes")
	}
}

func TestOwnerOfLongestPrefixAndAmbiguity(t *testing.T) {
	m, err := ParseManifest([]byte(testManifest))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path []string
		want WorkflowRef
		ok   bool
	}{
		{[]string{"doc"}, WorkflowRef{"feishu-cli-alpha", "one"}, true},
		{[]string{"doc", "import"}, WorkflowRef{"feishu-cli-alpha", "two"}, true},
		{[]string{"doc", "get", "x"}, WorkflowRef{"feishu-cli-alpha", "one"}, true},
		{[]string{"msg", "list"}, WorkflowRef{"feishu-cli-beta", "three"}, true},
		{[]string{"msg", "send"}, WorkflowRef{}, false}, // 同长度两个归属 → 视为未归属
		{[]string{"unknown"}, WorkflowRef{}, false},
	}
	for _, c := range cases {
		got, ok := m.OwnerOf(c.path)
		if ok != c.ok || got != c.want {
			t.Errorf("OwnerOf(%v) = %v,%v want %v,%v", c.path, got, ok, c.want, c.ok)
		}
	}
	if !m.IsExcludedCommand([]string{"completion", "bash"}) || m.IsExcludedCommand([]string{"doc"}) {
		t.Fatal("IsExcludedCommand mismatch")
	}
	if got := strings.Join(m.LegacySkillNames(), ","); got != "feishu-cli-old" {
		t.Fatalf("legacy names must dedupe and skip current skill names, got %s", got)
	}
	if WorkflowPath("one") != "references/workflows/one/workflow.md" {
		t.Fatal("WorkflowPath mismatch")
	}
}

func TestReadFileGuards(t *testing.T) {
	b, err := Load(testFS())
	if err != nil {
		t.Fatal(err)
	}
	skill, rel, data, err := b.ReadFile("feishu-cli-alpha", "")
	if err != nil || skill != "feishu-cli-alpha" || rel != "SKILL.md" || !strings.Contains(string(data), "# Alpha") {
		t.Fatalf("default read = %s %s %v", skill, rel, err)
	}
	// 跨技能相对引用（技能 Markdown 中 ../feishu-cli-xxx/ 的写法）
	skill, rel, _, err = b.ReadFile("feishu-cli-alpha", "../feishu-cli-beta/references/workflows/card/workflow.md")
	if err != nil || skill != "feishu-cli-beta" || rel != "references/workflows/card/workflow.md" {
		t.Fatalf("cross-skill read = %s %s %v", skill, rel, err)
	}
	for _, bad := range []string{"/etc/passwd", "../../etc/passwd", "../manifest.yaml", "references/workflows/two/scripts/test_run.py", "evals/evals.json", "references", "missing.md"} {
		if _, _, _, err := b.ReadFile("feishu-cli-alpha", bad); err == nil {
			t.Errorf("ReadFile(%q) should fail", bad)
		}
	}
	if _, _, _, err := b.ReadFile("feishu-cli-nope", "SKILL.md"); err == nil || !strings.Contains(err.Error(), "未知技能") {
		t.Fatalf("unknown skill err = %v", err)
	}
	files, err := b.ListFiles("feishu-cli-alpha", "references/workflows/two")
	if err != nil || len(files) != 2 {
		t.Fatalf("ListFiles = %v %v", files, err)
	}
	if _, err := b.ListFiles("feishu-cli-alpha", "../x"); err == nil {
		t.Fatal("ListFiles should reject escaping path")
	}
}

func TestLoadNilFS(t *testing.T) {
	if _, err := Load(nil); err == nil || !strings.Contains(err.Error(), "未内嵌") {
		t.Fatalf("Load(nil) err = %v", err)
	}
}

func TestParseFrontmatterName(t *testing.T) {
	if got := ParseFrontmatterName([]byte("---\r\nname: feishu-cli-msg\r\ndescription: x\r\n---\r\nbody")); got != "feishu-cli-msg" {
		t.Fatalf("name = %q", got)
	}
	if got := ParseFrontmatterName([]byte("# no frontmatter")); got != "" {
		t.Fatalf("name = %q", got)
	}
}

// TestRepositorySkillsLoad 用仓库真实 skills/ 目录做冒烟：9 个领域技能都能加载、
// 每个 owner 工作流入口都存在，且评测/测试文件不会进入分发清单。
func TestRepositorySkillsLoad(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "skills")
	b, err := Load(os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Skills()) != len(b.Manifest.ExpectedTopLevelSkills) {
		t.Fatalf("loaded %d skills, manifest expects %d", len(b.Skills()), len(b.Manifest.ExpectedTopLevelSkills))
	}
	for _, owner := range b.Manifest.Owners {
		if _, _, _, err := b.ReadFile(owner.Skill, WorkflowPath(owner.Workflow)); err != nil {
			t.Errorf("owner workflow missing: %v", err)
		}
	}
	for _, s := range b.Skills() {
		for _, f := range s.Files {
			if strings.HasPrefix(f.Path, "evals/") || strings.Contains(f.Path, "/test_") || strings.Contains(f.Path, "__pycache__") {
				t.Errorf("non-distributable file leaked: %s/%s", s.Name, f.Path)
			}
		}
	}
}
