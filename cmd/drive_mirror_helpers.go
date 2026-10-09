package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

// resolveSafeLocalDir 把用户传入的 --local-dir 解为「完全解析符号链接 + 限定在 cwd 子树」的绝对路径。
// 这是 drive pull/push/status 的安全前置：避免 `link/..` 这类路径在 walk 时被内核解析到 cwd 之外。
// 同时拒绝敏感目录（cwd 为家目录时 --local-dir .ssh 也在 cwd 子树内）：write=true（pull 写入/删除本地文件）
// 按输出路径校验，否则（push/status 读取本地文件）按输入路径校验。需在任何网络请求之前调用。
func resolveSafeLocalDir(localDir string, write bool) (safeAbs, cwdAbs string, err error) {
	if localDir == "" {
		return "", "", clierr.Usagef("--local-dir 不能为空")
	}
	validate := safefile.ValidateInputPath
	if write {
		validate = safefile.ValidateOutputPath
	}
	if err := validate(localDir); err != nil {
		return "", "", err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("获取 cwd 失败: %w", err)
	}
	cwdAbs, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		// EvalSymlinks 失败时退化到 cwd 本身（没有 symlink 也是合理）
		cwdAbs = cwd
	}

	// 先确保目录存在
	info, statErr := os.Stat(localDir)
	if statErr != nil {
		return "", "", clierr.Usage(fmt.Errorf("--local-dir 不存在或无法访问: %w", statErr))
	}
	if !info.IsDir() {
		return "", "", clierr.Usagef("--local-dir 不是目录: %s", localDir)
	}

	abs, err := filepath.Abs(localDir)
	if err != nil {
		return "", "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// 符号链接解析失败，退回 abs 但不允许越界
		resolved = abs
	}

	rel, err := filepath.Rel(cwdAbs, resolved)
	if err != nil || rel == ".." || (len(rel) >= 3 && rel[:3] == "../") {
		return "", "", clierr.Usagef("--local-dir 必须在当前工作目录子树内: %s", localDir)
	}
	// 解析符号链接后的真实目录再校验一次（cwd 内的链接可能指向敏感目录）
	if err := validate(resolved); err != nil {
		return "", "", err
	}
	return resolved, cwdAbs, nil
}

// walkLocalRegularFiles 走 root，返回 rel_path（用 / 分隔，相对 root）→ 绝对路径 的映射。
// 仅收 regular file，不跟随子级 symlink。
func walkLocalRegularFiles(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(absPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, absPath)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = absPath
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("遍历 %s 失败: %w", root, err)
	}
	return files, nil
}

// walkLocalDirs 返回 root 下的所有子目录（rel_path，不含 root 本身）。
// 用于 push 阶段镜像目录结构。
func walkLocalDirs(root string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(absPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() || absPath == root {
			return nil
		}
		rel, err := filepath.Rel(root, absPath)
		if err != nil {
			return err
		}
		dirs = append(dirs, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("遍历 %s 失败: %w", root, err)
	}
	return dirs, nil
}

// remoteFilesOnly 从全量 entry map 提取 type=file 的子集（ rel_path → file_token）。
func remoteFilesOnly(entries map[string]client.DriveRemoteEntry) map[string]string {
	out := make(map[string]string, len(entries))
	for rel, e := range entries {
		if e.Type == "file" {
			out[rel] = e.FileToken
		}
	}
	return out
}

// safeMirrorTarget 把远端相对路径（"/" 分隔）映射为 root 下的本地路径。
//
// 按路径段判断而不是子串：report..v2.pdf、a..b/c.txt 合法；"..", ".", 空段、含反斜杠或 NUL 的段拒绝
// （远端名字理论上不会出现这些，但镜像必须防御被构造的名字把文件写出 --local-dir）。
// 还会解析已存在部分的符号链接，拒绝经本地符号链接目录逃逸到 root 之外的目标。
func safeMirrorTarget(root, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("远端相对路径为空")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, "\\\x00") {
			return "", fmt.Errorf("远端路径 %q 含非法路径段 %q，已拒绝写入本地（防止越出 --local-dir）", rel, seg)
		}
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	if !safefile.IsWithin(target, root) {
		return "", fmt.Errorf("远端路径 %q 解析后越出 --local-dir，已拒绝", rel)
	}
	within, err := safefile.IsWithinResolved(target, root)
	if err != nil {
		return "", fmt.Errorf("校验本地目标路径失败 (%s): %w", rel, err)
	}
	if !within {
		return "", fmt.Errorf("本地目标 %q 经符号链接指向 --local-dir 之外，已拒绝写入", rel)
	}
	return target, nil
}

// resolveMirrorIdentity 解析 drive pull/push/status 的身份：
//   - 显式 --as：走 resolveIdentityToken（auto 已配置 User 但不可用时 fail-closed）；
//   - 未传 --as 且为破坏性模式（--delete-local / --delete-remote）：fail-closed，拒绝静默降级 Bot；
//   - 未传 --as 的普通模式：保持旧行为，User 优先、不可用时 stderr 告警后回退 Bot。
func resolveMirrorIdentity(cmd *cobra.Command, destructive bool, opName string) (string, error) {
	return resolveIdentityWithLegacyDefault(cmd, func(c *cobra.Command) (string, error) {
		if destructive {
			return resolveOptionalUserTokenForDestructive(c, opName)
		}
		return resolveOptionalUserTokenWithFallback(c), nil
	})
}
