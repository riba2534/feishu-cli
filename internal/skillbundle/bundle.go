// Package skillbundle 描述随 feishu-cli 二进制一起分发的领域技能内容。
//
// 技能文件在编译期通过 go:embed 打进二进制（见仓库根 skills_embed.go），
// 运行期以 fs.FS 注入。本包负责：解析 skills/manifest.yaml、筛选可分发文件、
// 计算内容哈希、按命令路径查找归属工作流，以及安全地读取单个文件。
package skillbundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestFile 是 skills/ 根目录下的技能清单文件名（JSON 子集，YAML 兼容）。
const ManifestFile = "manifest.yaml"

// WorkflowRef 指向某个领域技能下的一个工作流。
type WorkflowRef struct {
	Skill    string `json:"skill"`
	Workflow string `json:"workflow"`
}

// Path 返回工作流入口文件相对技能目录的路径。
func (w WorkflowRef) Path() string {
	return WorkflowPath(w.Workflow)
}

// WorkflowPath 返回工作流入口文件相对技能目录的路径。
func WorkflowPath(workflow string) string {
	return "references/workflows/" + workflow + "/workflow.md"
}

// Owner 声明某个工作流负责的命令前缀。
type Owner struct {
	Skill    string     `json:"skill"`
	Workflow string     `json:"workflow"`
	Prefixes [][]string `json:"prefixes"`
}

// LegacyMapping 记录 v1.35 之前的旧技能名到新领域工作流的迁移关系。
type LegacyMapping struct {
	LegacySkill string `json:"legacy_skill"`
	Skill       string `json:"skill"`
	Workflow    string `json:"workflow"`
}

// Manifest 对应 skills/manifest.yaml。
type Manifest struct {
	Version                 int             `json:"version"`
	ExpectedTopLevelSkills  []string        `json:"expected_top_level_skills"`
	ExcludedCommandPrefixes [][]string      `json:"excluded_command_prefixes"`
	HiddenCommands          [][]string      `json:"hidden_commands"`
	NonCommandWorkflows     []WorkflowRef   `json:"non_command_workflows"`
	LegacySkillMappings     []LegacyMapping `json:"legacy_skill_mappings"`
	Owners                  []Owner         `json:"owners"`
}

// ParseManifest 解析 manifest.yaml。文件约定为 JSON 子集，因此直接用 encoding/json。
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("解析技能清单 %s 失败: %w", ManifestFile, err)
	}
	if len(m.ExpectedTopLevelSkills) == 0 {
		return nil, fmt.Errorf("技能清单 %s 缺少 expected_top_level_skills", ManifestFile)
	}
	return &m, nil
}

// LoadManifest 从技能根目录 FS 读取并解析 manifest.yaml。
func LoadManifest(fsys fs.FS) (*Manifest, error) {
	data, err := fs.ReadFile(fsys, ManifestFile)
	if err != nil {
		return nil, fmt.Errorf("读取技能清单 %s 失败: %w", ManifestFile, err)
	}
	return ParseManifest(data)
}

// OwnerOf 按最长前缀匹配返回命令路径（不含根命令名）所属的工作流。
// 同一长度存在多个不同归属时视为未归属（与 scripts/check_skills.py 的唯一归属规则一致）。
func (m *Manifest) OwnerOf(commandPath []string) (WorkflowRef, bool) {
	best := -1
	var winners []WorkflowRef
	for _, owner := range m.Owners {
		for _, prefix := range owner.Prefixes {
			if !hasPrefix(commandPath, prefix) {
				continue
			}
			ref := WorkflowRef{Skill: owner.Skill, Workflow: owner.Workflow}
			switch {
			case len(prefix) > best:
				best = len(prefix)
				winners = []WorkflowRef{ref}
			case len(prefix) == best:
				winners = appendUnique(winners, ref)
			}
		}
	}
	if len(winners) != 1 {
		return WorkflowRef{}, false
	}
	return winners[0], true
}

// IsExcludedCommand 报告命令路径是否属于清单中排除归属检查的前缀（completion/help）。
func (m *Manifest) IsExcludedCommand(commandPath []string) bool {
	for _, prefix := range m.ExcludedCommandPrefixes {
		if hasPrefix(commandPath, prefix) {
			return true
		}
	}
	return false
}

// SkillWorkflows 返回某技能声明的全部工作流名（含非命令工作流），按名称排序去重。
func (m *Manifest) SkillWorkflows(skill string) []string {
	seen := map[string]bool{}
	for _, owner := range m.Owners {
		if owner.Skill == skill {
			seen[owner.Workflow] = true
		}
	}
	for _, ref := range m.NonCommandWorkflows {
		if ref.Skill == skill {
			seen[ref.Workflow] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// LegacySkillNames 返回需要迁移的旧技能目录名（去重排序，排除与当前领域技能同名者，
// 例如 feishu-cli-mail 新旧同名，绝不能当作旧目录清理）。
func (m *Manifest) LegacySkillNames() []string {
	current := map[string]bool{}
	for _, name := range m.ExpectedTopLevelSkills {
		current[name] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, item := range m.LegacySkillMappings {
		name := item.LegacySkill
		if name == "" || current[name] || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func hasPrefix(commandPath, prefix []string) bool {
	if len(prefix) > len(commandPath) {
		return false
	}
	for i, part := range prefix {
		if commandPath[i] != part {
			return false
		}
	}
	return true
}

func appendUnique(list []WorkflowRef, ref WorkflowRef) []WorkflowRef {
	for _, existing := range list {
		if existing == ref {
			return list
		}
	}
	return append(list, ref)
}

// File 是技能内的一个可分发文件。
type File struct {
	Path   string `json:"path"` // 相对技能目录，使用 / 分隔
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Skill 是一个领域技能的元数据与文件清单。
type Skill struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Compatibility string   `json:"compatibility,omitempty"`
	Workflows     []string `json:"workflows"`
	Files         []File   `json:"-"`
	FileCount     int      `json:"file_count"`
	TotalBytes    int64    `json:"total_bytes"`
	Hash          string   `json:"hash"`
}

// FileHashes 返回 path → sha256 映射。
func (s *Skill) FileHashes() map[string]string {
	out := make(map[string]string, len(s.Files))
	for _, f := range s.Files {
		out[f.Path] = f.SHA256
	}
	return out
}

// Bundle 是一次编译内嵌的全部技能内容。
type Bundle struct {
	fsys     fs.FS
	Manifest *Manifest
	skills   []*Skill
	byName   map[string]*Skill
	Hash     string
}

// Load 读取技能根目录 FS（含 manifest.yaml 与各技能目录），计算文件清单与哈希。
func Load(fsys fs.FS) (*Bundle, error) {
	if fsys == nil {
		return nil, errors.New("当前构建未内嵌技能内容（请使用 make build 或 go build . 从仓库根编译）")
	}
	manifest, err := LoadManifest(fsys)
	if err != nil {
		return nil, err
	}
	b := &Bundle{fsys: fsys, Manifest: manifest, byName: map[string]*Skill{}}
	names := append([]string(nil), manifest.ExpectedTopLevelSkills...)
	sort.Strings(names)
	for _, name := range names {
		skill, err := loadSkill(fsys, manifest, name)
		if err != nil {
			return nil, err
		}
		b.skills = append(b.skills, skill)
		b.byName[name] = skill
	}
	b.Hash = bundleHash(b.skills)
	return b, nil
}

func loadSkill(fsys fs.FS, manifest *Manifest, name string) (*Skill, error) {
	data, err := fs.ReadFile(fsys, name+"/SKILL.md")
	if err != nil {
		return nil, fmt.Errorf("内嵌技能 %s 缺少 SKILL.md: %w", name, err)
	}
	fm := parseFrontmatter(data)
	skill := &Skill{
		Name:          name,
		Description:   fm.Description,
		Compatibility: fm.Compatibility,
		Workflows:     manifest.SkillWorkflows(name),
	}
	err = fs.WalkDir(fsys, name, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(p, name+"/")
		if d.IsDir() {
			if p != name && !Distributable(rel+"/") {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !Distributable(rel) {
			return nil
		}
		content, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		skill.Files = append(skill.Files, File{Path: rel, Size: int64(len(content)), SHA256: HashBytes(content)})
		skill.TotalBytes += int64(len(content))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("遍历内嵌技能 %s 失败: %w", name, err)
	}
	sort.Slice(skill.Files, func(i, j int) bool { return skill.Files[i].Path < skill.Files[j].Path })
	skill.FileCount = len(skill.Files)
	skill.Hash = FilesHash(skill.FileHashes())
	return skill, nil
}

// Distributable 判断技能内相对路径是否属于运行期需要分发的内容。
// 以 / 结尾表示目录。排除评测、测试脚本、Python 缓存与隐藏文件。
func Distributable(rel string) bool {
	isDir := strings.HasSuffix(rel, "/")
	rel = strings.TrimSuffix(rel, "/")
	if rel == "" {
		return true
	}
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if part == "" || strings.HasPrefix(part, ".") || part == "__pycache__" || strings.HasSuffix(part, "-workspace") {
			return false
		}
		if i == 0 && part == "evals" {
			return false
		}
	}
	if isDir {
		return true
	}
	base := parts[len(parts)-1]
	switch {
	case strings.HasSuffix(base, ".pyc"):
		return false
	case strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"):
		return false
	case strings.HasSuffix(base, "_test.py"), strings.HasSuffix(base, "_test.go"):
		return false
	}
	return true
}

// HashBytes 返回内容的 sha256 十六进制摘要。
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// FilesHash 把 path → sha256 映射规约为一个稳定摘要（路径排序后逐行拼接再哈希）。
func FilesHash(files map[string]string) string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		fmt.Fprintf(h, "%s\x00%s\n", p, files[p])
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func bundleHash(skills []*Skill) string {
	h := sha256.New()
	for _, s := range skills {
		fmt.Fprintf(h, "%s\x00%s\n", s.Name, s.Hash)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// SkillsHash 计算一组技能哈希（name → hash）的整体摘要，与 Bundle.Hash 算法一致。
func SkillsHash(hashes map[string]string) string {
	names := make([]string, 0, len(hashes))
	for name := range hashes {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		fmt.Fprintf(h, "%s\x00%s\n", name, hashes[name])
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// Skills 返回按名称排序的技能列表。
func (b *Bundle) Skills() []*Skill { return b.skills }

// Skill 按名称查找技能。
func (b *Bundle) Skill(name string) (*Skill, bool) {
	s, ok := b.byName[name]
	return s, ok
}

// SkillNames 返回全部技能名（排序）。
func (b *Bundle) SkillNames() []string {
	out := make([]string, 0, len(b.skills))
	for _, s := range b.skills {
		out = append(out, s.Name)
	}
	return out
}

// FS 返回底层技能根目录 FS。
func (b *Bundle) FS() fs.FS { return b.fsys }

// ResolvePath 把 (技能名, 相对路径) 规范化为 "skill/relative"。
// 允许 "../feishu-cli-xxx/..." 形式跨技能引用（技能 Markdown 中的写法），
// 但结果必须落在某个内嵌技能内部且是可分发文件，拒绝绝对路径与越界路径。
func (b *Bundle) ResolvePath(skill, rel string) (string, string, error) {
	if _, ok := b.byName[skill]; !ok {
		return "", "", b.unknownSkill(skill)
	}
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	if rel == "" {
		rel = "SKILL.md"
	}
	if path.IsAbs(rel) {
		return "", "", fmt.Errorf("路径 %q 不能是绝对路径；请使用技能内相对路径，如 references/workflows/<workflow>/workflow.md", rel)
	}
	joined := path.Clean(skill + "/" + rel)
	if joined == "." || strings.HasPrefix(joined, "../") || joined == ".." {
		return "", "", fmt.Errorf("路径 %q 越出技能目录", rel)
	}
	target, inner, _ := strings.Cut(joined, "/")
	if _, ok := b.byName[target]; !ok {
		return "", "", fmt.Errorf("路径 %q 指向的技能 %q 不存在；运行 `feishu-cli skills list` 查看可用技能", rel, target)
	}
	if inner == "" {
		inner = "SKILL.md"
	}
	return target, inner, nil
}

// ReadFile 读取技能内文件，返回 (实际技能名, 相对路径, 内容)。
func (b *Bundle) ReadFile(skill, rel string) (string, string, []byte, error) {
	target, inner, err := b.ResolvePath(skill, rel)
	if err != nil {
		return "", "", nil, err
	}
	if !Distributable(inner) {
		return "", "", nil, fmt.Errorf("%s/%s 不属于随 CLI 分发的技能内容", target, inner)
	}
	full := target + "/" + inner
	info, err := fs.Stat(b.fsys, full)
	if err != nil {
		return "", "", nil, fmt.Errorf("技能 %s 中不存在 %s；运行 `feishu-cli skills list %s` 查看文件", target, inner, target)
	}
	if info.IsDir() {
		return "", "", nil, fmt.Errorf("%s/%s 是目录；运行 `feishu-cli skills list %s/%s` 查看其中文件", target, inner, target, inner)
	}
	data, err := fs.ReadFile(b.fsys, full)
	if err != nil {
		return "", "", nil, fmt.Errorf("读取内嵌技能文件 %s 失败: %w", full, err)
	}
	return target, inner, data, nil
}

// ListFiles 返回技能内（可选子目录下）的可分发文件。
func (b *Bundle) ListFiles(skill, sub string) ([]File, error) {
	s, ok := b.byName[skill]
	if !ok {
		return nil, b.unknownSkill(skill)
	}
	sub = strings.Trim(strings.ReplaceAll(sub, "\\", "/"), "/")
	if sub != "" {
		cleaned := path.Clean(sub)
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(sub) {
			return nil, fmt.Errorf("路径 %q 越出技能目录", sub)
		}
		sub = cleaned
	}
	var out []File
	for _, f := range s.Files {
		if sub == "" || f.Path == sub || strings.HasPrefix(f.Path, sub+"/") {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("技能 %s 中没有路径 %q；运行 `feishu-cli skills list %s` 查看文件", skill, sub, skill)
	}
	return out, nil
}

func (b *Bundle) unknownSkill(name string) error {
	return fmt.Errorf("未知技能 %q；可用技能: %s", name, strings.Join(b.SkillNames(), ", "))
}

type frontmatter struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	Compatibility string `yaml:"compatibility"`
}

// ParseFrontmatterName 返回 SKILL.md frontmatter 中的 name 字段（解析失败返回空串）。
func ParseFrontmatterName(skillMD []byte) string {
	return parseFrontmatter(skillMD).Name
}

func parseFrontmatter(data []byte) frontmatter {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return frontmatter{}
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return frontmatter{}
	}
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return frontmatter{}
	}
	fm.Description = strings.TrimSpace(fm.Description)
	return fm
}
