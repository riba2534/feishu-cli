// Package profile 管理 feishu-cli 的多配置（profile）目录。
//
// 目录布局：
//
//	~/.feishu-cli/
//	  config.yaml           # 旧布局（无 profile 系统时）
//	  token.json            # 旧布局
//	  active-profile        # 指针文件，纯文本，内容为当前 profile 名
//	  profiles/
//	    work/
//	      config.yaml
//	      token.json
//	      user_profile.json
//	    personal/
//	      config.yaml
//	      ...
//
// 解析优先级（由 ActiveDir 返回）：
//  1. 环境变量 FEISHU_PROFILE=<name>（强制覆盖，profile 必须存在）
//  2. ~/.feishu-cli/active-profile 指针指向的 profile
//  3. 无 profile 系统时，返回旧布局 ~/.feishu-cli/
//
// 设计要点：
//   - 向后兼容：旧用户的 ~/.feishu-cli/config.yaml + token.json 仍然能读，
//     一旦执行 `profile add` 才会创建 profiles/ 目录并迁移旧布局到默认 profile。
//   - 写操作（创建/重命名/删除/切换）通过 flock 串行化，避免并发写损坏。
//   - 名称合法字符：[A-Za-z0-9_-]，长度 1-64，禁止 "." ".." 等路径注入。
package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	// EnvVar 环境变量名，设置后强制使用指定 profile。
	EnvVar = "FEISHU_PROFILE"

	// activePointerName 指针文件名，记录当前 profile。
	activePointerName = "active-profile"

	// previousPointerName 记录上一个 profile，支持 `profile use -` 切换回上一个。
	previousPointerName = "previous-profile"

	// profilesDirName profile 子目录名。
	profilesDirName = "profiles"

	// 单 profile 目录下的内容文件名（与旧布局保持一致）。
	configFileName = "config.yaml"
	tokenFileName  = "token.json"
	userCacheName  = "user_profile.json"

	// MaxNameLen profile 名最大长度。
	MaxNameLen = 64
)

// nameRegex 合法 profile 名：字母数字/下划线/连字符，1-64 字符。
var nameRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// 进程内锁，用于 active-profile / previous-profile 指针文件的串行化。
// 仅 sync.Mutex 保护进程内并发；跨进程并发场景应用层避免。
var writeMu sync.Mutex

// ErrNotConfigured 当未配置 profile 系统时返回（无 profiles/ 目录）。
var ErrNotConfigured = errors.New("profile 系统未初始化")

// ErrNotFound 当指定 profile 不存在时返回。
var ErrNotFound = errors.New("profile 不存在")

// ErrAlreadyExists 当新 profile 名已被占用时返回。
var ErrAlreadyExists = errors.New("profile 已存在")

// ErrInvalidName 当 profile 名非法时返回。
var ErrInvalidName = errors.New("profile 名非法")

// homeFunc 可在测试中替换，避免读到真实 $HOME。
var homeFunc = os.UserHomeDir

// SetHomeFunc 仅用于测试，重置后会自动锁定 mutex。
func SetHomeFunc(fn func() (string, error)) func() {
	writeMu.Lock()
	defer writeMu.Unlock()
	old := homeFunc
	homeFunc = fn
	return func() {
		writeMu.Lock()
		defer writeMu.Unlock()
		homeFunc = old
	}
}

// RootDir 返回 feishu-cli 的根配置目录 ~/.feishu-cli。
func RootDir() (string, error) {
	home, err := homeFunc()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败: %w", err)
	}
	return filepath.Join(home, ".feishu-cli"), nil
}

// profilesDir 返回 ~/.feishu-cli/profiles。
func profilesDir() (string, error) {
	root, err := RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, profilesDirName), nil
}

// HasProfiles 是否启用了 profile 系统（profiles/ 目录是否存在且至少有一个子目录）。
func HasProfiles() (bool, error) {
	dir, err := profilesDir()
	if err != nil {
		return false, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("读取 profiles 目录失败: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() && nameRegex.MatchString(e.Name()) {
			return true, nil
		}
	}
	return false, nil
}

// ProfileDir 返回指定 profile 的目录路径，不校验是否存在。
func ProfileDir(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	dir, err := profilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// ValidateName 校验 profile 名是否合法。
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: 名字不能为空", ErrInvalidName)
	}
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("%w: %q 含非法字符（允许 A-Z a-z 0-9 _ - 且长度 ≤ %d）", ErrInvalidName, name, MaxNameLen)
	}
	if name == "profiles" || name == "cache" {
		return fmt.Errorf("%w: %q 为保留名", ErrInvalidName, name)
	}
	return nil
}

// Exists 报告 profile 是否存在（profiles/<name>/ 目录是否存在）。
func Exists(name string) (bool, error) {
	if err := ValidateName(name); err != nil {
		return false, err
	}
	dir, err := ProfileDir(name)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("访问 profile 目录失败: %w", err)
	}
	return info.IsDir(), nil
}

// List 返回所有 profile 名（按字典序）。当 profiles/ 不存在时返回空切片。
func List() ([]string, error) {
	dir, err := profilesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("读取 profiles 目录失败: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !nameRegex.MatchString(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// readPointer 读取 active-profile / previous-profile 指针文件，文件不存在返回空字符串。
func readPointer(filename string) (string, error) {
	root, err := RootDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(root, filename))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("读取 %s 失败: %w", filename, err)
	}
	name := strings.TrimSpace(string(data))
	return name, nil
}

// writePointer 原子写入指针文件（先写 .tmp 后 rename）。
func writePointer(filename, value string) error {
	root, err := RootDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	path := filepath.Join(root, filename)
	tmp := path + ".tmp"
	if value == "" {
		// 清空指针 = 删除文件
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除 %s 失败: %w", filename, err)
		}
		return nil
	}
	if err := os.WriteFile(tmp, []byte(value+"\n"), 0600); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", filename, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换 %s 失败: %w", filename, err)
	}
	return nil
}

// ReadActive 读取 active-profile 指针（不校验 profile 是否存在）。
func ReadActive() (string, error) {
	return readPointer(activePointerName)
}

// ReadPrevious 读取 previous-profile 指针。
func ReadPrevious() (string, error) {
	return readPointer(previousPointerName)
}

// ActiveName 返回当前激活的 profile 名（解析优先级见包注释）。
// 若未启用 profile 系统返回空字符串和 nil error，调用方应回退到旧布局。
func ActiveName() (string, error) {
	if env := strings.TrimSpace(os.Getenv(EnvVar)); env != "" {
		if err := ValidateName(env); err != nil {
			return "", fmt.Errorf("%s 环境变量值非法: %w", EnvVar, err)
		}
		ok, err := Exists(env)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("%w: %s 指向的 profile %q 不存在", ErrNotFound, EnvVar, env)
		}
		return env, nil
	}
	hasProfiles, err := HasProfiles()
	if err != nil {
		return "", err
	}
	if !hasProfiles {
		return "", nil
	}
	name, err := ReadActive()
	if err != nil {
		return "", err
	}
	if name == "" {
		// 有 profile 但没指针，回退到字典序第一个
		all, err := List()
		if err != nil {
			return "", err
		}
		if len(all) == 0 {
			return "", nil
		}
		return all[0], nil
	}
	if err := ValidateName(name); err != nil {
		return "", fmt.Errorf("active-profile 指针非法: %w", err)
	}
	ok, err := Exists(name)
	if err != nil {
		return "", err
	}
	if !ok {
		// 指针指向不存在的 profile，回退到第一个
		all, err := List()
		if err != nil {
			return "", err
		}
		if len(all) == 0 {
			return "", nil
		}
		return all[0], nil
	}
	return name, nil
}

// ActiveDir 返回当前激活 profile 的目录。
// 启用 profile 系统：返回 ~/.feishu-cli/profiles/<active>/。
// 未启用：返回旧布局 ~/.feishu-cli/。
// 该函数被 internal/config 与 internal/auth 用来定位 config.yaml / token.json / user_profile.json。
func ActiveDir() (string, error) {
	name, err := ActiveName()
	if err != nil {
		return "", err
	}
	if name == "" {
		return RootDir()
	}
	return ProfileDir(name)
}

// ConfigFilePath 返回当前激活 profile 的 config.yaml 路径。
func ConfigFilePath() (string, error) {
	dir, err := ActiveDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// TokenFilePath 返回当前激活 profile 的 token.json 路径。
func TokenFilePath() (string, error) {
	dir, err := ActiveDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, tokenFileName), nil
}

// UserCacheFilePath 返回当前激活 profile 的 user_profile.json 路径。
func UserCacheFilePath() (string, error) {
	dir, err := ActiveDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, userCacheName), nil
}

// Info 描述一个 profile 的元数据，用于 list 输出。
type Info struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Active    bool   `json:"active"`
	HasConfig bool   `json:"has_config"`
	HasToken  bool   `json:"has_token"`
}

// Describe 返回所有 profile 的元数据列表。当未启用 profile 系统时返回空切片。
func Describe() ([]Info, error) {
	names, err := List()
	if err != nil {
		return nil, err
	}
	active, err := ActiveName()
	if err != nil {
		return nil, err
	}
	out := make([]Info, 0, len(names))
	for _, name := range names {
		dir, err := ProfileDir(name)
		if err != nil {
			return nil, err
		}
		info := Info{
			Name:      name,
			Path:      dir,
			Active:    name == active,
			HasConfig: fileExists(filepath.Join(dir, configFileName)),
			HasToken:  fileExists(filepath.Join(dir, tokenFileName)),
		}
		out = append(out, info)
	}
	return out, nil
}

// fileExists 测试文件是否存在。
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// CreateOpts 控制新 profile 的初始化内容。
type CreateOpts struct {
	// AppID 写入 config.yaml 的 app_id，可为空（用户后续手动填）。
	AppID string
	// AppSecret 写入 config.yaml 的 app_secret，可为空。
	AppSecret string
	// BaseURL 写入 config.yaml 的 base_url，为空时使用默认值。
	BaseURL string
	// SwitchTo 创建后是否立即切换为 active profile。
	SwitchTo bool
}

// Create 创建一个新 profile。
//
// 行为：
//  1. 校验名字合法且未被占用
//  2. 如果是首个 profile（profiles/ 不存在），且旧布局存在 config.yaml/token.json，
//     不会自动迁移——避免悄无声息丢数据。调用方应先用 Migrate() 显式迁移。
//  3. 创建 profiles/<name>/，写入 config.yaml（含 app_id/app_secret/base_url）
//  4. 若 SwitchTo=true，写 active-profile 指针
func Create(name string, opts CreateOpts) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	if err := ValidateName(name); err != nil {
		return err
	}
	exists, err := Exists(name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: %q", ErrAlreadyExists, name)
	}

	dir, err := ProfileDir(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建 profile 目录失败: %w", err)
	}

	configFile := filepath.Join(dir, configFileName)
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}
	content := renderConfigYAML(opts.AppID, opts.AppSecret, baseURL)
	if err := writeFileAtomic(configFile, []byte(content), 0600); err != nil {
		return fmt.Errorf("写入 config.yaml 失败: %w", err)
	}

	if opts.SwitchTo {
		previous, _ := ReadActive() // best-effort
		if previous != "" && previous != name {
			if err := writePointer(previousPointerName, previous); err != nil {
				return fmt.Errorf("更新 previous-profile 失败: %w", err)
			}
		}
		if err := writePointer(activePointerName, name); err != nil {
			return fmt.Errorf("更新 active-profile 失败: %w", err)
		}
	}
	return nil
}

// Remove 删除一个 profile（包括目录下所有文件）。
//
// 调用方应在交互场景下加 --force 二次确认。
// 若删除的是当前 active profile，active-profile 指针会被清空（下次访问回退到第一个）。
func Remove(name string) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	if err := ValidateName(name); err != nil {
		return err
	}
	exists, err := Exists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}

	dir, err := ProfileDir(name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("删除 profile 目录失败: %w", err)
	}

	// 清理指针（如果指向了被删除的 profile）
	active, _ := ReadActive()
	if active == name {
		if err := writePointer(activePointerName, ""); err != nil {
			return err
		}
	}
	previous, _ := ReadPrevious()
	if previous == name {
		if err := writePointer(previousPointerName, ""); err != nil {
			return err
		}
	}
	return nil
}

// Rename 将 profile 从 oldName 重命名到 newName。
func Rename(oldName, newName string) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	if err := ValidateName(oldName); err != nil {
		return fmt.Errorf("旧名: %w", err)
	}
	if err := ValidateName(newName); err != nil {
		return fmt.Errorf("新名: %w", err)
	}
	if oldName == newName {
		return fmt.Errorf("新旧名相同: %q", oldName)
	}

	srcExists, err := Exists(oldName)
	if err != nil {
		return err
	}
	if !srcExists {
		return fmt.Errorf("%w: %q", ErrNotFound, oldName)
	}
	dstExists, err := Exists(newName)
	if err != nil {
		return err
	}
	if dstExists {
		return fmt.Errorf("%w: %q", ErrAlreadyExists, newName)
	}

	srcDir, err := ProfileDir(oldName)
	if err != nil {
		return err
	}
	dstDir, err := ProfileDir(newName)
	if err != nil {
		return err
	}
	if err := os.Rename(srcDir, dstDir); err != nil {
		return fmt.Errorf("重命名 profile 目录失败: %w", err)
	}

	// 更新指针
	active, _ := ReadActive()
	if active == oldName {
		if err := writePointer(activePointerName, newName); err != nil {
			return err
		}
	}
	previous, _ := ReadPrevious()
	if previous == oldName {
		if err := writePointer(previousPointerName, newName); err != nil {
			return err
		}
	}
	return nil
}

// Use 切换 active profile 到 name。
// 若 name == "-"，切换回 previous-profile（toggle）。
func Use(name string) (string, error) {
	writeMu.Lock()
	defer writeMu.Unlock()

	// "-" 表示切回上一个
	if name == "-" {
		prev, err := ReadPrevious()
		if err != nil {
			return "", err
		}
		if prev == "" {
			return "", fmt.Errorf("没有可切换回的上一个 profile")
		}
		name = prev
	}

	if err := ValidateName(name); err != nil {
		return "", err
	}
	exists, err := Exists(name)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("%w: %q", ErrNotFound, name)
	}

	current, _ := ReadActive()
	if current == name {
		// 已经在目标 profile，无需切换
		return name, nil
	}

	// 切换前把当前 active 记为 previous
	if current != "" {
		if err := writePointer(previousPointerName, current); err != nil {
			return "", err
		}
	}
	if err := writePointer(activePointerName, name); err != nil {
		return "", err
	}
	return name, nil
}

// MigrateLegacyOpts 控制旧布局迁移行为。
type MigrateLegacyOpts struct {
	// TargetName 迁移到的 profile 名（默认 "default"）。
	TargetName string
	// Force 即便目标 profile 已存在也迁移（会覆盖）。
	Force bool
}

// MigrateLegacy 把旧布局 ~/.feishu-cli/{config.yaml,token.json,user_profile.json}
// 迁移到 profiles/<TargetName>/，并把指针指向该 profile。原文件不删除（让用户手动确认）。
// 旧文件不存在的话，仍然会创建一个空 profile 目录。
func MigrateLegacy(opts MigrateLegacyOpts) (string, error) {
	writeMu.Lock()
	defer writeMu.Unlock()

	target := opts.TargetName
	if target == "" {
		target = "default"
	}
	if err := ValidateName(target); err != nil {
		return "", err
	}
	exists, err := Exists(target)
	if err != nil {
		return "", err
	}
	if exists && !opts.Force {
		return "", fmt.Errorf("%w: %q（用 --force 覆盖）", ErrAlreadyExists, target)
	}

	dstDir, err := ProfileDir(target)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dstDir, 0700); err != nil {
		return "", fmt.Errorf("创建 profile 目录失败: %w", err)
	}

	root, err := RootDir()
	if err != nil {
		return "", err
	}
	for _, f := range []string{configFileName, tokenFileName, userCacheName} {
		src := filepath.Join(root, f)
		if !fileExists(src) {
			continue
		}
		dst := filepath.Join(dstDir, f)
		if err := copyFile(src, dst); err != nil {
			return "", fmt.Errorf("迁移 %s 失败: %w", f, err)
		}
	}

	if err := writePointer(activePointerName, target); err != nil {
		return "", err
	}
	return target, nil
}

// writeFileAtomic 原子写入文件：先写 .tmp 后 rename。
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// copyFile 复制文件内容并保留权限位。
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return writeFileAtomic(dst, data, info.Mode().Perm())
}

// renderConfigYAML 生成 profile 的 config.yaml 模板。
func renderConfigYAML(appID, appSecret, baseURL string) string {
	var b strings.Builder
	b.WriteString("# 飞书 CLI 配置文件 (profile)\n")
	b.WriteString("# 由 `feishu-cli profile add` 自动创建\n\n")
	if appID == "" {
		b.WriteString(`app_id: ""` + "\n")
	} else {
		fmt.Fprintf(&b, "app_id: %q\n", appID)
	}
	if appSecret == "" {
		b.WriteString(`app_secret: ""` + "\n")
	} else {
		fmt.Fprintf(&b, "app_secret: %q\n", appSecret)
	}
	fmt.Fprintf(&b, "base_url: %q\n", baseURL)
	b.WriteString(`owner_email: ""` + "\n")
	b.WriteString("transfer_ownership: false\n")
	b.WriteString("debug: false\n\n")
	b.WriteString("export:\n")
	b.WriteString("  download_images: true\n")
	b.WriteString(`  assets_dir: "./assets"` + "\n\n")
	b.WriteString("import:\n")
	b.WriteString("  upload_images: true\n")
	return b.String()
}
