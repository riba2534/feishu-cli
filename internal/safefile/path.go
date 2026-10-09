// Package safefile 提供本地文件路径安全校验与原子写入。
//
// 路径策略（只做拒绝名单，不限制写到任意普通目录，避免误伤 /tmp、~/Downloads 等正常路径）：
//   - 相对路径按"路径段"拒绝 ".."（越出当前目录）；report..v2.json 这类文件名照常放行；
//   - 解析符号链接后，拒绝落在敏感目录内的路径：系统目录（/etc、/proc、/sys、/dev、/var/run）
//     与家目录下的凭证目录/文件（~/.ssh、~/.aws、~/.gnupg、~/.kube、~/.docker、~/.config/gcloud、
//     ~/.feishu-cli 等，清单对齐官方 lark-cli 的 localfileio 拒绝名单）；
//   - 包含判断带路径分隔符边界（/etc 不会误伤 /etcetera），并按文件身份（inode）兜底，
//     覆盖大小写不敏感文件系统上 ~/.SSH 这类别名。
package safefile

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// systemDenyRoots 系统敏感目录（Windows 上不适用）。
var systemDenyRoots = []string{"/etc", "/proc", "/sys", "/dev", "/var/run"}

// homeDenyNames 家目录下拒绝读写的凭证目录 / 文件（按包含关系匹配，目录覆盖其下全部内容）。
var homeDenyNames = []string{
	".ssh", ".gnupg", ".aws", ".azure", ".kube", ".docker",
	".config/gcloud", ".config/gh",
	".netrc", ".git-credentials", ".gitconfig", ".npmrc", ".pypirc",
	".gem/credentials", ".cargo/credentials", ".cargo/credentials.toml",
	".bash_history", ".zsh_history", ".sh_history", ".python_history", ".psql_history",
	".feishu-cli", ".lark-cli",
}

// devStreamAllowed 放行的伪设备：管道 / 标准流 / 黑洞，读写它们不涉及敏感数据。
func devStreamAllowed(abs string) bool {
	switch abs {
	case "/dev/null", "/dev/stdin", "/dev/stdout", "/dev/stderr":
		return true
	}
	return strings.HasPrefix(abs, "/dev/fd/")
}

type denyRoot struct {
	label    string
	literal  string
	resolved string
	info     os.FileInfo
}

// denyRoots 每次调用现算（几十次 Lstat，开销可忽略），以便测试中切换 HOME 立即生效。
func denyRoots() []denyRoot {
	var roots []denyRoot
	add := func(label, p string) {
		p = filepath.Clean(p)
		r := denyRoot{label: label, literal: p, resolved: p}
		if resolved, err := resolveNearestAncestor(p); err == nil {
			r.resolved = resolved
		}
		if fi, err := os.Stat(r.resolved); err == nil {
			r.info = fi
		}
		roots = append(roots, r)
	}
	if runtime.GOOS != "windows" {
		for _, p := range systemDenyRoots {
			add(p, p)
		}
	}
	for _, home := range candidateHomes() {
		for _, name := range homeDenyNames {
			add("~/"+name, filepath.Join(home, filepath.FromSlash(name)))
		}
	}
	return roots
}

// candidateHomes 汇总所有可能的家目录（$HOME 可能被改写，账户库里的才是真实家目录）。
func candidateHomes() []string {
	seen := map[string]bool{}
	var homes []string
	add := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" || h == "/" || seen[h] {
			return
		}
		seen[h] = true
		homes = append(homes, h)
	}
	if h, err := os.UserHomeDir(); err == nil {
		add(h)
	}
	if u, err := user.Current(); err == nil {
		add(u.HomeDir)
	}
	return homes
}

// ValidateOutputPath 校验用户指定的本地输出路径（文件或目录）。违规时返回用法错误（退出码 2）。
func ValidateOutputPath(path string) error {
	return validate(path, "输出路径", "写入")
}

// ValidateInputPath 校验用户指定的本地输入文件（--data-file、--xxx-file 等）。
// 拒绝读取敏感目录，防止把凭证当请求体发到远端。"-"（stdin）由调用方自行处理，不要传入。
func ValidateInputPath(path string) error {
	return validate(path, "输入文件", "读取")
}

func validate(path, kind, verb string) error {
	if strings.TrimSpace(path) == "" {
		return clierr.Usagef("%s不能为空", kind)
	}
	if strings.ContainsRune(path, 0) {
		return clierr.Usagef("%s %q 含非法字符", kind, path)
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) && hasParentSegment(clean) {
		return clierr.Usagef("%s %q 越出当前目录（含 '..' 路径段），请改用绝对路径或当前目录下的路径", kind, path)
	}
	if label, ok := sensitiveRootOf(clean); ok {
		return clierr.Usagef("%s %q 位于受保护的敏感目录 %s 内，已拒绝%s（防止泄露或覆盖凭证 / 系统文件），请换用其他目录", kind, path, label, verb)
	}
	return nil
}

// hasParentSegment 按路径段判断是否含 ".."（filepath.Clean 之后相对路径只可能以 ".." 段开头）。
func hasParentSegment(clean string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(clean), "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

func sensitiveRootOf(clean string) (string, bool) {
	abs, err := filepath.Abs(clean)
	if err != nil {
		return "", false
	}
	if devStreamAllowed(abs) {
		return "", false
	}
	resolved, err := resolveNearestAncestor(abs)
	if err != nil {
		resolved = abs
	}
	roots := denyRoots()
	for _, r := range roots {
		if IsWithin(abs, r.literal) || IsWithin(resolved, r.literal) || IsWithin(resolved, r.resolved) {
			return r.label, true
		}
	}
	// 名字比较无法覆盖大小写不敏感 / 规范化等别名（如 macOS 上 ~/.SSH 即 ~/.ssh），按文件身份兜底
	for p := resolved; ; {
		if fi, err := os.Stat(p); err == nil {
			for _, r := range roots {
				if r.info != nil && os.SameFile(fi, r.info) {
					return r.label, true
				}
			}
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return "", false
}

// IsWithin 判断 path 是否等于 dir 或位于 dir 之下（带路径分隔符边界：/etc 不包含 /etcetera）。
// 两者都应是已 Clean 的绝对路径；本函数不解析符号链接。
func IsWithin(path, dir string) bool {
	path, dir = filepath.Clean(path), filepath.Clean(dir)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		path, dir = strings.ToLower(path), strings.ToLower(dir)
	}
	if path == dir {
		return true
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// IsWithinResolved 在解析符号链接后判断 path 是否位于 dir 之下（用于"输出必须在某目录内"的约束）。
func IsWithinResolved(path, dir string) (bool, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("无法解析路径: %w", err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false, fmt.Errorf("无法解析目录: %w", err)
	}
	rp, err := resolveNearestAncestor(absPath)
	if err != nil {
		return false, err
	}
	rd, err := resolveNearestAncestor(absDir)
	if err != nil {
		return false, err
	}
	return IsWithin(rp, rd), nil
}

// resolveNearestAncestor 解析路径中已存在部分的符号链接，尚不存在的尾部原样拼回。
func resolveNearestAncestor(abs string) (string, error) {
	abs = filepath.Clean(abs)
	var tail []string
	cur := abs
	for {
		if _, err := os.Lstat(cur); err == nil {
			resolved, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}
