package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/riba2534/feishu-cli/internal/selfupdate"
	"github.com/spf13/cobra"
)

var (
	updateCheck  bool
	updateDryRun bool
	updateForce  bool
	updateTarget string
	updateJSON   bool
	// newUpdateClient 便于测试注入本地 HTTP 服务。
	newUpdateClient = selfupdate.New
	updateGOOS      = runtime.GOOS
	updateGOARCH    = runtime.GOARCH
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "检查并更新 feishu-cli 到 GitHub 最新 release",
	Long: `检查 GitHub 最新 release，并把当前二进制原子替换为对应平台的新版本。

流程:
  1. 通过 github.com/riba2534/feishu-cli/releases/latest 的 302 跳转获取最新 tag（不消耗 API 配额；失败回退 GitHub API，可设 GITHUB_TOKEN）
  2. 下载 feishu-cli_<版本>_<os>-<arch>.tar.gz 与 checksums.txt，校验 sha256（release 缺少校验文件时拒绝更新）
  3. 解压到目标二进制所在目录的临时文件，运行其 --version 确认版本
  4. 原子替换目标二进制（默认当前运行的二进制，解析符号链接后的真实路径）

只在显式运行时联网；CLI 不会在其他命令中后台检查版本。
目标目录无写权限时给出提示，不会自动 sudo。更新后运行 feishu-cli skills install 同步本地技能。`,
	Example: `  feishu-cli update --check
  feishu-cli update --dry-run
  feishu-cli update
  feishu-cli update --target /opt/tools/feishu-cli`,
	Args:        cobra.NoArgs,
	Annotations: map[string]string{skipConfigInitAnnotation: "1"},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Minute)
		defer cancel()
		return runUpdate(ctx, cmd.OutOrStdout(), cmd.ErrOrStderr())
	},
}

type updateStatus struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	CurrentKnown    bool   `json:"current_version_known"`
	Target          string `json:"target"`
	Asset           string `json:"asset,omitempty"`
	DownloadURL     string `json:"download_url,omitempty"`
	ChecksumsURL    string `json:"checksums_url,omitempty"`
	DryRun          bool   `json:"dry_run,omitempty"`
	Updated         bool   `json:"updated"`
	Note            string `json:"note,omitempty"`
}

func runUpdate(ctx context.Context, out, errOut io.Writer) error {
	if updateCheck && updateDryRun {
		return errors.New("--check 与 --dry-run 不能同时使用：--check 只查询版本，--dry-run 预览更新步骤")
	}
	client := newUpdateClient()
	st := updateStatus{DryRun: updateDryRun}

	target, current, err := resolveUpdateTarget(ctx)
	if err != nil {
		return err
	}
	st.Target, st.CurrentVersion = target, current

	latest, err := client.LatestVersion(ctx)
	if err != nil {
		return err
	}
	st.LatestVersion = latest
	cur, known := selfupdate.ParseVersion(current)
	st.CurrentKnown = known
	lv, _ := selfupdate.ParseVersion(latest)
	switch {
	case !known:
		st.UpdateAvailable = true
		st.Note = fmt.Sprintf("当前版本 %q 无法识别（可能是未注入版本号的开发构建）", current)
	case selfupdate.Compare(cur, lv) < 0:
		st.UpdateAvailable = true
	case cur.Dev:
		st.Note = fmt.Sprintf("当前是基于 %d.%d.%d 的开发构建", cur.Major, cur.Minor, cur.Patch)
	}

	if updateCheck {
		if updateJSON {
			return writeJSON(out, st)
		}
		fmt.Fprintf(out, "当前版本: %s\n最新版本: %s\n", current, latest)
		if st.Note != "" {
			fmt.Fprintf(out, "说明: %s\n", st.Note)
		}
		if st.UpdateAvailable {
			fmt.Fprintln(out, "有可用更新：运行 feishu-cli update 升级（先用 --dry-run 预览）")
		} else {
			fmt.Fprintln(out, "已是最新版本")
		}
		return nil
	}

	if !st.UpdateAvailable && !updateForce {
		return finishUpdate(out, st, fmt.Sprintf("已是最新版本 %s，无需更新（--force 可强制重新安装）", latest))
	}
	if !known && !updateForce {
		return fmt.Errorf("%s；为避免覆盖开发构建，默认不更新。确认要替换为 %s 请加 --force", st.Note, latest)
	}

	asset, err := selfupdate.AssetName(latest, updateGOOS, updateGOARCH)
	if err != nil {
		return err
	}
	st.Asset = asset
	st.DownloadURL = client.DownloadURL(latest, asset)
	st.ChecksumsURL = client.DownloadURL(latest, "checksums.txt")
	dir := filepath.Dir(target)

	if updateDryRun {
		writable := checkDirWritable(dir)
		if updateJSON {
			return writeJSON(out, st)
		}
		fmt.Fprintf(out, "[dry-run] 将把 %s 从 %s 更新到 %s\n", target, current, latest)
		fmt.Fprintf(out, "  下载: %s\n  校验: %s（sha256，缺失则拒绝）\n", st.DownloadURL, st.ChecksumsURL)
		if writable != nil {
			fmt.Fprintf(out, "  ⚠️  目标目录不可写: %v\n", writable)
		} else {
			fmt.Fprintf(out, "  目标目录可写: %s\n", dir)
		}
		fmt.Fprintln(out, "未下载、未修改任何文件。")
		return nil
	}

	if err := checkDirWritable(dir); err != nil {
		return fmt.Errorf("没有权限写入 %s: %v\n建议: 以有权限的用户重新运行（如 sudo %s update），或重新执行 install.sh，或用 --target 指定可写位置的二进制", dir, err, filepath.Base(target))
	}

	fmt.Fprintf(errOut, "下载校验文件 %s\n", st.ChecksumsURL)
	sums, err := client.FetchChecksums(ctx, latest)
	if err != nil {
		if errors.Is(err, selfupdate.ErrNotFound) {
			return fmt.Errorf("release %s 未提供 checksums.txt，无法校验完整性，拒绝自动更新；请从 release 页面手动下载", latest)
		}
		return err
	}
	expected, ok := sums[asset]
	if !ok {
		return fmt.Errorf("checksums.txt 中没有 %s 的校验值，拒绝更新", asset)
	}

	tmpDir, err := os.MkdirTemp("", "feishu-cli-update-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	archive := filepath.Join(tmpDir, asset)
	fmt.Fprintf(errOut, "下载安装包 %s\n", st.DownloadURL)
	if err := downloadToFile(ctx, client, st.DownloadURL, archive); err != nil {
		return err
	}
	actual, err := selfupdate.FileSHA256(archive)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("安装包 sha256 不匹配，已终止更新（期望 %s，实际 %s）", expected, actual)
	}
	fmt.Fprintln(errOut, "sha256 校验通过")

	staged, err := os.CreateTemp(dir, ".feishu-cli.update-*")
	if err != nil {
		return fmt.Errorf("在 %s 创建临时文件失败: %w", dir, err)
	}
	stagedPath := staged.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(stagedPath)
		}
	}()
	if err := selfupdate.ExtractBinary(archive, selfupdate.BinaryName(updateGOOS), staged); err != nil {
		_ = staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return err
	}
	if updateGOOS == runtime.GOOS && updateGOARCH == runtime.GOARCH {
		got, raw, err := selfupdate.BinaryVersion(ctx, stagedPath)
		if err != nil {
			return fmt.Errorf("新二进制自检失败，未替换: %w", err)
		}
		if !selfupdate.SameVersion(got, latest) {
			return fmt.Errorf("新二进制版本 %q 与目标版本 %s 不一致，未替换（输出: %s）", got, latest, raw)
		}
	}
	oldPath, err := selfupdate.ReplaceExecutable(target, stagedPath, updateGOOS)
	if err != nil {
		return fmt.Errorf("替换 %s 失败: %w", target, err)
	}
	committed = true
	st.Updated = true
	msg := fmt.Sprintf("已更新 %s: %s → %s", target, current, latest)
	if oldPath != "" {
		msg += fmt.Sprintf("（旧版本保留为 %s，可在退出后删除）", oldPath)
	}
	msg += "\n技能内容随版本变化：运行 feishu-cli skills install 同步本地技能，或 feishu-cli doctor --only skills 检查漂移"
	return finishUpdate(out, st, msg)
}

func finishUpdate(out io.Writer, st updateStatus, msg string) error {
	if updateJSON {
		if st.Note == "" && !st.Updated {
			st.Note = msg
		}
		return writeJSON(out, st)
	}
	fmt.Fprintln(out, msg)
	return nil
}

// resolveUpdateTarget 返回要替换的二进制真实路径及其当前版本。
func resolveUpdateTarget(ctx context.Context) (string, string, error) {
	if updateTarget != "" {
		abs, err := filepath.Abs(updateTarget)
		if err != nil {
			return "", "", err
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", "", fmt.Errorf("--target %s 不存在或无法解析: %w", updateTarget, err)
		}
		current, _, err := selfupdate.BinaryVersion(ctx, real)
		if err != nil {
			return "", "", fmt.Errorf("--target 不是可运行的 feishu-cli: %w", err)
		}
		return real, current, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("无法定位当前二进制: %w；请用 --target 指定", err)
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", "", fmt.Errorf("解析当前二进制路径失败: %w", err)
	}
	return real, version, nil
}

func checkDirWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".feishu-cli.write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

func downloadToFile(ctx context.Context, client *selfupdate.Client, url, dst string) error {
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := client.Fetch(ctx, url, f, selfupdate.MaxArchiveBytes); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "只查询当前与最新版本，不下载")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "预览下载地址、校验与目标路径，不下载不替换")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "已是最新或当前版本无法识别时仍重新安装")
	updateCmd.Flags().StringVar(&updateTarget, "target", "", "要替换的 feishu-cli 二进制路径（默认当前运行的二进制）")
	updateCmd.Flags().BoolVar(&updateJSON, "json", false, "输出 JSON")
	rootCmd.AddCommand(updateCmd)
}
