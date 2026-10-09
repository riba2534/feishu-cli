// Package skillinstall 把二进制内嵌的技能安装到本地 Agent 技能目录，并维护安装状态文件。
//
// 安全约束：
//   - 写入前解析目标目录的符号链接，所有写入发生在真实路径下；
//   - 只覆盖由本工具记录过且未被本地修改的文件；检测到本地修改时，不带 --force 整体拒绝；
//   - 技能目录本身是符号链接时视为外部管理，任何情况下都不写入、不删除；
//   - 旧技能目录仅在显式 --prune-legacy 且 SKILL.md 的 name 与旧技能名一致时删除。
package skillinstall

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/skillbundle"
)

const (
	// StateFileName 是写在技能目录根下的安装状态文件。
	StateFileName = ".feishu-cli-skills.json"
	// EnvSkillsDir 覆盖默认技能目录。
	EnvSkillsDir = "FEISHU_CLI_SKILLS_DIR"
	stateSchema  = 1
	stateTool    = "feishu-cli"
)

// ErrConflict 表示存在本地修改，未带 --force 时拒绝写入。
var ErrConflict = errors.New("技能目录存在本地修改，或没有安装记录且内容与内嵌版本不同")

// DefaultDir 返回默认技能目录：FEISHU_CLI_SKILLS_DIR > ~/.claude/skills。
func DefaultDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(EnvSkillsDir)); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法确定用户主目录: %w；请用 --dir 指定技能目录", err)
	}
	return filepath.Join(home, ".claude", "skills"), nil
}

// ResolvedDir 描述目标目录及其符号链接解析结果。
type ResolvedDir struct {
	Input  string `json:"input"`
	Abs    string `json:"abs"`
	Real   string `json:"real"`
	Exists bool   `json:"exists"`
}

// Symlinked 报告目标路径是否经过符号链接解析。
func (r ResolvedDir) Symlinked() bool { return r.Abs != r.Real }

// ResolveDir 把目录展开 ~、转绝对路径并解析符号链接。目录不存在时解析最深的已存在祖先，
// 再拼回剩余部分，保证后续写入发生在真实路径下。
func ResolveDir(dir string) (ResolvedDir, error) {
	input := dir
	if dir == "" {
		return ResolvedDir{}, errors.New("技能目录不能为空")
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ResolvedDir{}, fmt.Errorf("展开 ~ 失败: %w", err)
		}
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ResolvedDir{}, fmt.Errorf("解析技能目录 %s 失败: %w", input, err)
	}
	res := ResolvedDir{Input: input, Abs: abs}
	existing := abs
	var rest []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		rest = append([]string{filepath.Base(existing)}, rest...)
		existing = parent
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return ResolvedDir{}, fmt.Errorf("解析符号链接 %s 失败: %w", existing, err)
	}
	res.Real = filepath.Join(append([]string{real}, rest...)...)
	if info, err := os.Stat(res.Real); err == nil {
		if !info.IsDir() {
			return ResolvedDir{}, fmt.Errorf("技能目录 %s 不是目录", res.Real)
		}
		res.Exists = true
	}
	return res, nil
}

// SkillState 记录一个技能安装时的文件哈希。
type SkillState struct {
	Hash  string            `json:"hash"`
	Files map[string]string `json:"files"`
}

// State 是 .feishu-cli-skills.json 的内容。
type State struct {
	Schema      int                   `json:"schema"`
	Tool        string                `json:"tool"`
	CLIVersion  string                `json:"cli_version"`
	InstalledAt string                `json:"installed_at"`
	BundleHash  string                `json:"bundle_hash"`
	Skills      map[string]SkillState `json:"skills"`
}

// ReadState 读取状态文件；不存在时返回 (nil, nil)。
func ReadState(realDir string) (*State, error) {
	data, err := os.ReadFile(filepath.Join(realDir, StateFileName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取技能安装状态失败: %w", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("技能安装状态文件 %s 已损坏: %w", filepath.Join(realDir, StateFileName), err)
	}
	if st.Tool != stateTool || st.Skills == nil {
		return nil, fmt.Errorf("技能安装状态文件 %s 不是 feishu-cli 生成的格式", filepath.Join(realDir, StateFileName))
	}
	return &st, nil
}

// Action 是单个技能的安装动作。
type Action string

const (
	ActionCreate    Action = "create"
	ActionUpdate    Action = "update"
	ActionUnchanged Action = "unchanged"
	ActionConflict  Action = "conflict"
	ActionExternal  Action = "external" // 技能目录是符号链接，外部管理，跳过
)

// SkillPlan 是单个技能的安装计划。
type SkillPlan struct {
	Name       string   `json:"name"`
	Action     Action   `json:"action"`
	Managed    bool     `json:"managed"` // 状态文件中有记录
	Write      []string `json:"write,omitempty"`
	Remove     []string `json:"remove,omitempty"`
	Modified   []string `json:"modified,omitempty"`  // 本地修改（或未受管理目录中与内嵌版本不同）的文件
	Untracked  []string `json:"untracked,omitempty"` // 非本工具管理的文件，始终保留
	LinkTarget string   `json:"link_target,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// LegacyStatus 是旧技能目录的识别结果。
type LegacyStatus string

const (
	LegacyConfirmed LegacyStatus = "confirmed" // SKILL.md name 与旧技能名一致
	LegacyForeign   LegacyStatus = "foreign"   // 同名目录但不是 feishu-cli 旧技能
	LegacySymlink   LegacyStatus = "symlink"   // 符号链接，不处理
)

// LegacyPlan 描述一个存在于目标目录中的旧技能目录。
type LegacyPlan struct {
	Name      string       `json:"name"`
	Path      string       `json:"path"`
	Status    LegacyStatus `json:"status"`
	FoundName string       `json:"found_name,omitempty"`
	Removed   bool         `json:"removed,omitempty"`
}

// Plan 是一次安装的完整计划。
type Plan struct {
	Dir        ResolvedDir  `json:"dir"`
	CLIVersion string       `json:"cli_version"`
	BundleHash string       `json:"bundle_hash"`
	State      *State       `json:"-"`
	StateError string       `json:"state_error,omitempty"`
	Skills     []SkillPlan  `json:"skills"`
	Legacy     []LegacyPlan `json:"legacy,omitempty"`
}

// Conflicts 返回需要 --force 才能继续的技能。
func (p *Plan) Conflicts() []SkillPlan {
	var out []SkillPlan
	for _, s := range p.Skills {
		if s.Action == ActionConflict {
			out = append(out, s)
		}
	}
	return out
}

// Count 统计某动作的技能数。
func (p *Plan) Count(action Action) int {
	n := 0
	for _, s := range p.Skills {
		if s.Action == action {
			n++
		}
	}
	return n
}

// BuildPlan 对比内嵌技能、目标目录现状与状态文件，生成安装计划（只读）。
func BuildPlan(b *skillbundle.Bundle, dir ResolvedDir, cliVersion string) (*Plan, error) {
	plan := &Plan{Dir: dir, CLIVersion: cliVersion, BundleHash: b.Hash}
	if dir.Exists {
		st, err := ReadState(dir.Real)
		if err != nil {
			// 状态损坏：按"无记录"处理（更保守：已有目录与内嵌版本不同即视为冲突）
			plan.StateError = err.Error()
		} else {
			plan.State = st
		}
	}
	for _, skill := range b.Skills() {
		sp, err := planSkill(skill, dir.Real, plan.State)
		if err != nil {
			return nil, err
		}
		plan.Skills = append(plan.Skills, sp)
	}
	if dir.Exists {
		legacy, err := scanLegacy(b.Manifest.LegacySkillNames(), dir.Real)
		if err != nil {
			return nil, err
		}
		plan.Legacy = legacy
	}
	return plan, nil
}

func planSkill(skill *skillbundle.Skill, root string, st *State) (SkillPlan, error) {
	sp := SkillPlan{Name: skill.Name}
	dir := filepath.Join(root, skill.Name)
	bundleFiles := skill.FileHashes()
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		sp.Action = ActionCreate
		sp.Write = sortedKeys(bundleFiles)
		return sp, nil
	case err != nil:
		return sp, fmt.Errorf("检查技能目录 %s 失败: %w", dir, err)
	case info.Mode()&os.ModeSymlink != 0:
		target, _ := os.Readlink(dir)
		sp.Action = ActionExternal
		sp.LinkTarget = target
		sp.Reason = "技能目录是符号链接（外部管理），不写入也不删除"
		return sp, nil
	case !info.IsDir():
		sp.Action = ActionConflict
		sp.Reason = "目标位置存在同名文件（不是目录）"
		return sp, nil
	}

	disk, err := hashTree(dir)
	if err != nil {
		return sp, err
	}
	var recorded map[string]string
	if st != nil {
		if rec, ok := st.Skills[skill.Name]; ok && rec.Files != nil {
			recorded = rec.Files
			sp.Managed = true
		}
	}

	for _, p := range sortedKeys(bundleFiles) {
		if disk[p] != bundleFiles[p] {
			sp.Write = append(sp.Write, p)
		}
	}
	for _, p := range sp.Write {
		got, exists := disk[p]
		if !exists {
			if link := symlinkAncestor(disk, p); link != "" {
				// 父目录是符号链接：写入会穿透到链接目标，必须人工处理
				sp.Modified = append(sp.Modified, link)
			}
			continue
		}
		if sp.Managed {
			if rec, tracked := recorded[p]; tracked && rec == got {
				continue // 受管且未修改：可安全覆盖
			}
		}
		// 无记录目录或本地修改过的文件：覆盖会丢失这些内容
		sp.Modified = append(sp.Modified, p)
	}
	for _, p := range sortedKeys(disk) {
		if _, inBundle := bundleFiles[p]; inBundle {
			continue
		}
		if rec, tracked := recorded[p]; tracked && rec == disk[p] {
			sp.Remove = append(sp.Remove, p) // 新版本已移除且本地未改 → 可安全删除
			continue
		}
		sp.Untracked = append(sp.Untracked, p) // 非本工具管理或已被修改：始终保留
	}
	sp.Modified = uniqueSorted(sp.Modified)

	switch {
	case len(sp.Modified) > 0:
		sp.Action = ActionConflict
		if sp.Managed {
			sp.Reason = fmt.Sprintf("%d 个文件在上次安装后被本地修改", len(sp.Modified))
		} else {
			sp.Reason = fmt.Sprintf("目录没有 feishu-cli 安装记录（可能来自 npx skills add 或手动复制），%d 个文件与内嵌版本不同", len(sp.Modified))
		}
	case len(sp.Write) == 0 && len(sp.Remove) == 0:
		sp.Action = ActionUnchanged
	default:
		sp.Action = ActionUpdate
	}
	return sp, nil
}

// hashTree 计算目录下全部文件（不跟随符号链接）的哈希。符号链接文件以特殊值记录，
// 保证它与任何真实内容都不相等。
func hashTree(dir string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.Type()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(p)
			out[rel] = "symlink:" + target
			return nil
		}
		if !d.Type().IsRegular() {
			out[rel] = "special"
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = skillbundle.HashBytes(data)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描技能目录 %s 失败: %w", dir, err)
	}
	return out, nil
}

func scanLegacy(names []string, root string) ([]LegacyPlan, error) {
	var out []LegacyPlan
	for _, name := range names {
		p := filepath.Join(root, name)
		info, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("检查旧技能目录 %s 失败: %w", p, err)
		}
		lp := LegacyPlan{Name: name, Path: p}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			lp.Status = LegacySymlink
		case !info.IsDir():
			lp.Status = LegacyForeign
		default:
			data, err := os.ReadFile(filepath.Join(p, "SKILL.md"))
			if err == nil {
				lp.FoundName = skillbundle.ParseFrontmatterName(data)
			}
			if lp.FoundName == name {
				lp.Status = LegacyConfirmed
			} else {
				lp.Status = LegacyForeign
			}
		}
		out = append(out, lp)
	}
	return out, nil
}

// Options 控制 Apply 行为。
type Options struct {
	Force       bool
	PruneLegacy bool
	Now         func() time.Time
}

// Result 汇总实际写入结果。
type Result struct {
	Written        int      `json:"written_files"`
	Removed        int      `json:"removed_files"`
	Pruned         []string `json:"pruned_legacy,omitempty"`
	StatePath      string   `json:"state_path"`
	StateUnchanged bool     `json:"state_unchanged,omitempty"`
}

// sameState 比较两份状态（忽略安装时间）。
func sameState(old, cur *State) bool {
	if old == nil || cur == nil || old.Schema != cur.Schema || old.CLIVersion != cur.CLIVersion ||
		old.BundleHash != cur.BundleHash || len(old.Skills) != len(cur.Skills) {
		return false
	}
	for name, s := range cur.Skills {
		o, ok := old.Skills[name]
		if !ok || o.Hash != s.Hash || len(o.Files) != len(s.Files) {
			return false
		}
		for p, h := range s.Files {
			if o.Files[p] != h {
				return false
			}
		}
	}
	return true
}

// Apply 按计划写入。存在冲突且未 Force 时不写入任何文件并返回 ErrConflict。
func Apply(b *skillbundle.Bundle, plan *Plan, opts Options) (*Result, error) {
	if conflicts := plan.Conflicts(); len(conflicts) > 0 && !opts.Force {
		names := make([]string, 0, len(conflicts))
		for _, c := range conflicts {
			names = append(names, c.Name)
		}
		return nil, fmt.Errorf("%w: %s；未写入任何文件。确认这些改动可以丢弃后加 --force 覆盖（非本工具管理的额外文件始终保留）", ErrConflict, strings.Join(names, ", "))
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	root := plan.Dir.Real
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("创建技能目录 %s 失败: %w", root, err)
	}
	res := &Result{StatePath: filepath.Join(root, StateFileName)}
	state := &State{
		Schema:      stateSchema,
		Tool:        stateTool,
		CLIVersion:  plan.CLIVersion,
		InstalledAt: now().UTC().Format(time.RFC3339),
		Skills:      map[string]SkillState{},
	}
	// 先整体校验再写入：任何一个技能无法安全写入时，不留下半套文件
	for _, sp := range plan.Skills {
		if sp.Action == ActionExternal {
			continue
		}
		dir := filepath.Join(root, sp.Name)
		if info, err := os.Lstat(dir); err == nil && !info.IsDir() {
			return res, fmt.Errorf("技能 %s 的目标位置不是普通目录，请手动处理: %s", sp.Name, dir)
		}
		for _, rel := range sp.Write {
			if err := checkNoSymlinkParents(root, sp.Name+"/"+rel); err != nil {
				return res, err
			}
		}
	}
	installedHashes := map[string]string{}
	for _, sp := range plan.Skills {
		skill, _ := b.Skill(sp.Name)
		if sp.Action == ActionExternal {
			continue
		}
		dir := filepath.Join(root, sp.Name)
		for _, rel := range sp.Write {
			_, _, data, err := b.ReadFile(sp.Name, rel)
			if err != nil {
				return res, err
			}
			if err := writeFileAtomic(filepath.Join(dir, filepath.FromSlash(rel)), data, fileMode(data)); err != nil {
				return res, fmt.Errorf("写入 %s/%s 失败: %w", sp.Name, rel, err)
			}
			res.Written++
		}
		for _, rel := range sp.Remove {
			target := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return res, fmt.Errorf("删除旧文件 %s/%s 失败: %w", sp.Name, rel, err)
			}
			res.Removed++
			removeEmptyParents(filepath.Dir(target), dir)
		}
		state.Skills[sp.Name] = SkillState{Hash: skill.Hash, Files: skill.FileHashes()}
		installedHashes[sp.Name] = skill.Hash
	}
	// 只有全部技能都由本工具写入时 bundle_hash 才等于内嵌版本；有外部管理的技能时按实际集合计算
	state.BundleHash = skillbundle.SkillsHash(installedHashes)
	if sameState(plan.State, state) {
		res.StateUnchanged = true // 完全无变化：保留原状态文件（含原安装时间），保证重复安装幂等
	} else {
		data, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return res, err
		}
		if err := writeFileAtomic(res.StatePath, append(data, '\n'), 0o644); err != nil {
			return res, fmt.Errorf("写入技能安装状态失败: %w", err)
		}
	}
	if opts.PruneLegacy {
		for i, lp := range plan.Legacy {
			if lp.Status != LegacyConfirmed {
				continue
			}
			// 删除前再确认一次：仍是普通目录且 name 未变
			info, err := os.Lstat(lp.Path)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			md, err := os.ReadFile(filepath.Join(lp.Path, "SKILL.md"))
			if err != nil || skillbundle.ParseFrontmatterName(md) != lp.Name {
				continue
			}
			if err := os.RemoveAll(lp.Path); err != nil {
				return res, fmt.Errorf("删除旧技能目录 %s 失败: %w", lp.Path, err)
			}
			plan.Legacy[i].Removed = true
			res.Pruned = append(res.Pruned, lp.Name)
		}
	}
	return res, nil
}

// checkNoSymlinkParents 确认 root 下 rel 的每一级父目录都不是符号链接，
// 防止写入穿透到技能目录之外（即使带 --force 也拒绝）。
func checkNoSymlinkParents(root, rel string) error {
	parts := strings.Split(rel, "/")
	cur := root
	for _, part := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("检查 %s 失败: %w", cur, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("拒绝写入：%s 是符号链接，写入会修改链接目标；请先手动处理该链接", cur)
		}
	}
	return nil
}

func fileMode(data []byte) os.FileMode {
	if bytes.HasPrefix(data, []byte("#!")) {
		return 0o755
	}
	return 0o644
}

func removeEmptyParents(dir, stop string) {
	for dir != stop && strings.HasPrefix(dir, stop+string(filepath.Separator)) {
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// 目标若是符号链接，rename 会替换链接本身而不是写穿到链接目标
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// symlinkAncestor 返回 rel 的某个父目录在磁盘上是符号链接时的该父目录路径。
func symlinkAncestor(disk map[string]string, rel string) string {
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		prefix := strings.Join(parts[:i], "/")
		if strings.HasPrefix(disk[prefix], "symlink:") {
			return prefix
		}
	}
	return ""
}

func uniqueSorted(list []string) []string {
	seen := map[string]bool{}
	out := list[:0]
	for _, item := range list {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
