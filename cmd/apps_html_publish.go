package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/output"
	"github.com/riba2534/feishu-cli/v2/internal/runctx"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/spf13/cobra"
)

// 客户端侧尺寸上限（用 var 便于单测调小覆盖拦截路径）。
//
//	maxAppsRawBytes     —— tar+gzip 进入内存前的「未压缩」总大小上限，防解压炸弹/OOM。
//	maxAppsTarballBytes —— 打包后 tar.gz 上限，对齐 OAPI「本期接口上限 20MB」约束。
var (
	maxAppsRawBytes        int64 = 200 * 1024 * 1024
	maxAppsTarballBytes    int64 = 20 * 1024 * 1024
	maxAppsSingleHTMLBytes int64 = 10 * 1024 * 1024 // 单个 .html 文件上限，对齐妙搭服务端 10MB 约束
)

// maxAppsSensitiveListInError 控制校验错误里最多内联列出多少个凭证文件命中。
const maxAppsSensitiveListInError = 5

var appsHTMLPublishCmd = &cobra.Command{
	Use:   "html-publish",
	Short: "把 HTML 文件/目录打包发布到妙搭应用（--wait 等待发布完成并返回 online_url）",
	Long: `把 --path（单个 HTML 文件或整个目录）打包成 tar.gz，按官方三段协议发布：
  GET /apps/{id}/pre_release 解析 upload_url / tos_path →
  对预签名 URL PUT tar.gz（不携带飞书 Authorization）→
  POST /apps/{id}/releases（body.tos_path）返回 release_id。

要求:
  - 目标应用 app_type 必须是 html 或 modern_html（实跑时 GET 应用校验）
  - 目录形态：根目录下必须有 index.html（妙搭以它作为应用入口）
  - 单文件形态：文件名必须就是 index.html
  - 未压缩总大小 ≤ 200MB；打包后 tar.gz ≤ 20MB；单个 .html 文件 ≤ 10MB
  - 默认拦截凭证文件（.env / .npmrc / .netrc / .git-credentials / .aws/credentials /
    .docker/config.json / .kube/config），用 --allow-sensitive 显式放行
  - 目录形态自动跳过 .git 目录与 .git 文件（不会把仓库历史发布到公网）
  - --app-id 必须是 app_ 开头的应用 ID（meta_token 先用 apps get 换出 app_id）
  - --dry-run 只展示计划（三段 endpoint + 打包清单），不获取 token、不访问网络、不上传

发布结果:
  发布是异步的，默认只返回 release_id；加 --wait 会每 20 秒查询一次 release 状态（默认最多 5 分钟），
  finished 输出 online_url，failed 输出 error_logs 并非零退出；进入人工审批时停止等待并提示审批链接。
  不加 --wait 时用 apps release get --app-id <id> --release-id <release_id> 查询。

权限: User Access Token + spark:app:read + spark:app:write

示例:
  feishu-cli apps html-publish --app-id app_xxx --path ./index.html
  feishu-cli apps html-publish --app-id app_xxx --path ./dist --wait     # 等待发布完成拿 online_url
  feishu-cli apps html-publish --app-id app_xxx --path ./dist --dry-run   # 只看计划与打包清单`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		appID := strings.TrimSpace(flagString(cmd, "app-id"))
		if appID == "" {
			return clierr.Usagef("--app-id 不能为空")
		}
		if err := validateRealAppID(appID); err != nil {
			return err
		}
		pathArg := strings.TrimSpace(flagString(cmd, "path"))
		if pathArg == "" {
			return fmt.Errorf("--path 不能为空")
		}
		allowSensitive, _ := cmd.Flags().GetBool("allow-sensitive")
		dry, _ := cmd.Flags().GetBool("dry-run")
		// 发布内容会公开到公网：--path 落在 ~/.ssh、~/.feishu-cli、/etc 等敏感目录时直接拒绝（不受 --allow-sensitive 影响）
		if err := safefile.ValidateInputPath(pathArg); err != nil {
			return err
		}

		candidates, walkErr := appsWalkCandidates(pathArg)
		// 目录形态（如 --path ~）可能把敏感目录整棵带进来：逐个文件按同一拒绝名单校验，dry-run 同样非零退出
		for _, c := range candidates {
			if err := safefile.ValidateInputPath(c.AbsPath); err != nil {
				return err
			}
		}
		// --path 是目录还是单文件，决定凭证扫描如何回填缺失的父目录上下文（见 appsIsSensitiveCandidate）。
		pathIsDir := false
		if fi, statErr := os.Stat(pathArg); statErr == nil {
			pathIsDir = fi.IsDir()
		}

		// 凭证文件拦截：dry-run 和实跑共用同一道闸门（命中且未加 --allow-sensitive 时两条路径都非零退出）。walk 失败时跳过，
		// 交给下面的分支用各自更丰富的报错呈现。
		if walkErr == nil && !allowSensitive {
			var hits []string
			for _, c := range candidates {
				if appsIsSensitiveCandidate(pathArg, pathIsDir, c) {
					hits = append(hits, c.RelPath)
				}
			}
			if len(hits) > 0 {
				return appsSensitiveError(hits)
			}
		}

		if dry {
			return appsHTMLPublishDryRun(cmd, appID, pathArg, pathIsDir, candidates, walkErr, allowSensitive)
		}

		if walkErr != nil {
			return fmt.Errorf("扫描 --path %s 失败: %w", pathArg, walkErr)
		}
		if err := appsEnsureIndexHTML(candidates); err != nil {
			return err
		}
		if oversize := appsOversizeHTMLFiles(candidates); len(oversize) > 0 {
			return appsOversizeHTMLFilesError(oversize)
		}

		var rawTotal int64
		for _, c := range candidates {
			rawTotal += c.Size
		}
		if rawTotal > maxAppsRawBytes {
			return fmt.Errorf("--path 未压缩总大小 %d 字节超过 %d 字节上限（tar+gzip 进内存前拦截，避免 OOM）；精简 --path 内容或选更小的子目录", rawTotal, maxAppsRawBytes)
		}

		tarball, err := appsBuildTarball(candidates)
		if err != nil {
			return fmt.Errorf("打包失败: %w", err)
		}
		if int64(len(tarball)) > maxAppsTarballBytes {
			return fmt.Errorf("打包后 tar.gz 大小 %d 字节超过 %d 字节上限；精简 --path 目录（去掉无关大文件/压缩资源）后重试，本期接口上限 20MB", len(tarball), maxAppsTarballBytes)
		}

		token, err := requireUserToken(cmd, "apps html-publish")
		if err != nil {
			return err
		}
		data, err := client.SparkHTMLPublish(appID, tarball, token)
		if err != nil {
			return err
		}
		if wait, _ := cmd.Flags().GetBool("wait"); wait {
			timeout, _ := cmd.Flags().GetDuration("wait-timeout")
			return appsWaitRelease(cmd, appID, sparkStringValue(data["release_id"]), token, timeout)
		}
		return renderAppsResult(cmd, data)
	},
}

// appsReleasePollInterval 轮询发布状态的间隔（官方建议约 20 秒；var 便于测试调小）。
var appsReleasePollInterval = 20 * time.Second

// appsWaitRelease 轮询 release get 直到终态并输出 online_url / error_logs（对齐官方 release-get 的 Agent 规则）：
//   - finished：输出 online_url（服务端未返回时不编造）
//   - failed：输出 error_logs 并以非零退出
//   - 尚未终态且 current_node_info.current_status=PENDING：等待审批负责人处理，立即停止轮询（不是失败）
//   - 顶层 pending 但节点不明确、未知状态：停止轮询原样报告
//   - publishing 超过 --wait-timeout：停止轮询，报告 release_id 与当前状态（发布仍在进行，不要重新发布）
func appsWaitRelease(cmd *cobra.Command, appID, releaseID, token string, timeout time.Duration) error {
	if releaseID == "" {
		return fmt.Errorf("html-publish 未返回 release_id，无法等待发布结果")
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx := runctx.Root()
	deadline := time.Now().Add(timeout)
	path := client.SparkReleaseGetPath(appID, releaseID)
	polls := 0
	for {
		polls++
		data, err := client.SparkCall("GET", path, nil, nil, token)
		if err != nil {
			return appsWithHint(err, fmt.Sprintf("发布已提交（release_id=%s），只是查询状态失败；稍后用 `feishu-cli apps release get --app-id %s --release-id %s` 继续查询，不要重新发布", releaseID, appID, releaseID))
		}
		rel := projectSparkRelease(data)
		if sparkStringValue(rel["release_id"]) == "" {
			rel["release_id"] = releaseID
		}
		status := sparkStringValue(rel["status"])
		outcome := ""
		switch {
		case status == sparkReleaseFinished:
			outcome = "finished"
		case status == sparkReleaseFailed:
			outcome = "failed"
		case sparkReleasePendingApproval(rel):
			outcome = "pending_approval"
		case status == sparkReleasePublishing:
			if time.Now().Add(appsReleasePollInterval).After(deadline) {
				outcome = "timeout"
			}
		default:
			// 顶层 pending 但没有明确 PENDING 节点、或未知状态：不自行判定结果
			outcome = "stopped"
		}
		if outcome == "" {
			select {
			case <-ctx.Done():
				return fmt.Errorf("等待发布结果被中断（release_id=%s）: %w", releaseID, ctx.Err())
			case <-time.After(appsReleasePollInterval):
			}
			continue
		}

		rel["wait"] = map[string]any{"outcome": outcome, "polls": polls}
		switch outcome {
		case "finished":
			if sparkStringValue(rel["online_url"]) != "" {
				fmt.Fprintln(os.Stderr, "发布完成；online_url 默认仅创建者可见，交付他人前按需执行 apps access-scope-set")
			}
		case "pending_approval":
			msg := "发布已进入人工审批，正在等待审批负责人处理（不是失败）。"
			if u := sparkApprovalURL(rel); u != "" {
				msg += "\n审批链接：" + u + "（打开前核对域名）"
			} else {
				msg += "\n服务端未返回有效审批链接。"
			}
			fmt.Fprintf(os.Stderr, "%s\n审批处理后用 `feishu-cli apps release get --app-id %s --release-id %s` 继续查询，不要重新发布\n", msg, appID, releaseID)
		case "timeout":
			fmt.Fprintf(os.Stderr, "等待 %s 后发布仍在进行（status=publishing）；稍后用 `feishu-cli apps release get --app-id %s --release-id %s` 继续查询，不要重新发布\n", timeout, appID, releaseID)
		case "stopped":
			fmt.Fprintf(os.Stderr, "发布状态为 %q，停止自动轮询；用 `feishu-cli apps release get --app-id %s --release-id %s` 查看详情\n", status, appID, releaseID)
		}
		if err := renderAppsResult(cmd, rel); err != nil {
			return err
		}
		if outcome == "failed" {
			return fmt.Errorf("妙搭发布失败（release_id=%s），失败步骤见输出中的 error_logs", releaseID)
		}
		return nil
	}
}

// appsHTMLPublishDryRun 打印打包清单预览（文件列表/总大小/缺 index.html 提示/放行的凭证文件）。
// dry-run 预览同样尊重 --format/--jq（对齐实调路径与 bitable dry-run），避免 help 列了却静默失效。
func appsHTMLPublishDryRun(cmd *cobra.Command, appID, pathArg string, pathIsDir bool, candidates []appsCandidate, walkErr error, allowSensitive bool) error {
	o, err := output.ParseOptions(cmd)
	if err != nil {
		return err
	}
	m := map[string]any{
		"dry_run": true,
		"plan":    "Pack tar.gz → GET pre_release → PUT tar.gz to TOS（不携带飞书 Authorization）→ POST release-create（body.tos_path）；返回 release_id",
		"steps": []map[string]any{
			{
				"method":   "GET",
				"endpoint": appsAppPath(appID, "/pre_release"),
			},
			{
				"method":        "PUT",
				"endpoint":      "<presigned_upload_url>",
				"content_type":  "application/gzip",
				"authorization": false,
			},
			{
				"method":   "POST",
				"endpoint": appsAppPath(appID, "/releases"),
				"body":     map[string]string{"tos_path": "<from pre_release response>"},
			},
		},
	}
	if wait, _ := cmd.Flags().GetBool("wait"); wait {
		timeout, _ := cmd.Flags().GetDuration("wait-timeout")
		m["steps"] = append(m["steps"].([]map[string]any), map[string]any{
			"method":   "GET",
			"endpoint": appsAppPath(appID, "/releases/<release_id>"),
			"desc":     fmt.Sprintf("--wait：每 %s 查询一次发布状态直到 finished/failed/待审批，最长 %s", appsReleasePollInterval, timeout),
		})
	}
	if walkErr != nil {
		m["path_error"] = walkErr.Error()
		return output.Render(o, m)
	}
	// 缺 index.html / 单 .html 超限在 dry-run 里以字段呈现（仍 0 退出，符合 dry-run「预览」语义）。
	// 同时聚合到统一的 would_block / block_reasons：实跑会因这些原因被拒，调用方据此单字段判断是否可发布，
	// 不必分别解析 validation_error / oversize_html 等细分键。
	var blockReasons []string
	if err := appsEnsureIndexHTML(candidates); err != nil {
		m["validation_error"] = err.Error()
		blockReasons = append(blockReasons, err.Error())
	}
	if oversize := appsOversizeHTMLFiles(candidates); len(oversize) > 0 {
		m["oversize_html"] = appsOversizeHTMLSummary(oversize)
		blockReasons = append(blockReasons, appsOversizeHTMLFilesError(oversize).Error())
	}
	var total int64
	names := make([]string, 0, len(candidates))
	for _, c := range candidates {
		total += c.Size
		names = append(names, c.RelPath)
	}
	m["file_count"] = len(candidates)
	m["total_size_bytes"] = total
	m["files"] = names
	if allowSensitive {
		var waived []string
		for _, c := range candidates {
			if appsIsSensitiveCandidate(pathArg, pathIsDir, c) {
				waived = append(waived, c.RelPath)
			}
		}
		if len(waived) > 0 {
			m["sensitive_waived"] = waived
			m["sensitive_waived_summary"] = fmt.Sprintf("%d 个凭证文件因 --allow-sensitive 被放行", len(waived))
		}
	}
	m["would_block"] = len(blockReasons) > 0
	if len(blockReasons) > 0 {
		m["block_reasons"] = blockReasons
	}
	return output.Render(o, m)
}

type appsCandidate struct {
	RelPath string // tar 内的相对路径（forward-slash）
	AbsPath string // 磁盘绝对/相对路径
	Size    int64
}

// appsWalkCandidates 遍历 rootPath，返回每个 regular file。单文件形态返回一条
// （RelPath = basename）；目录形态用 filepath.WalkDir 收集所有 regular file
// （symlink/device/pipe/socket 跳过）。
func appsWalkCandidates(rootPath string) ([]appsCandidate, error) {
	stat, err := os.Stat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("读取 --path %s 信息失败: %w", rootPath, err)
	}
	if !stat.IsDir() {
		return []appsCandidate{{
			RelPath: filepath.Base(rootPath),
			AbsPath: rootPath,
			Size:    stat.Size(),
		}}, nil
	}

	var out []appsCandidate
	err = filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// 跳过 git 仓库元数据（对齐官方 walk_html_publish_candidates）：.git 目录整棵子树不打包，
		// .git 文件（submodule / worktree 的 gitdir 指针）也跳过，避免把仓库历史发布到公网。
		// 只按名字精确匹配 .git，.gitignore / .github 等普通文件照常打包。
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// 只接受 regular file —— symlink 不跟随（避免 loop + 越界引用）。
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if appsIsUnsafeRelPath(relSlash) {
			return fmt.Errorf("遍历产生了不安全的相对路径 %q（%s）", relSlash, path)
		}
		out = append(out, appsCandidate{RelPath: relSlash, AbsPath: path, Size: info.Size()})
		return nil
	})
	return out, err
}

// appsIsUnsafeRelPath 判断一个 forward-slash 相对路径是否含越界/危险成分：
// 绝对路径前缀、.. 作为完整路径成分、或内嵌空字节。组件级判断，不会对
// 合法文件名里恰好含 ".." 子串（如 archive.tar..bak）误报。
func appsIsUnsafeRelPath(rel string) bool {
	return strings.HasPrefix(rel, "/") ||
		rel == ".." ||
		strings.HasPrefix(rel, "../") ||
		strings.Contains(rel, "/../") ||
		strings.HasSuffix(rel, "/..") ||
		strings.ContainsRune(rel, 0)
}

// appsEnsureIndexHTML 要求 candidates 里必须含 index.html（妙搭以它作为应用入口）。
func appsEnsureIndexHTML(candidates []appsCandidate) error {
	for _, c := range candidates {
		if c.RelPath == "index.html" {
			return nil
		}
	}
	return fmt.Errorf("--path 中缺少 index.html；妙搭以 index.html 作为应用入口（目录形态把首页放根目录命名 index.html，单文件形态把文件命名为 index.html）")
}

// appsOversizeHTMLFiles 返回扩展名为 .html（大小写不敏感）且超过单文件上限的候选，
// 对齐妙搭服务端单个 .html 文件 ≤10MB 约束，在客户端提前拦截并点名文件。
func appsOversizeHTMLFiles(candidates []appsCandidate) []appsCandidate {
	var oversize []appsCandidate
	for _, c := range candidates {
		if strings.EqualFold(filepath.Ext(c.RelPath), ".html") && c.Size > maxAppsSingleHTMLBytes {
			oversize = append(oversize, c)
		}
	}
	return oversize
}

// appsOversizeHTMLFilesError 构造单 .html 文件超限错误（点名文件 + 大小 + 拆分/裁剪提示）。
func appsOversizeHTMLFilesError(oversize []appsCandidate) error {
	names := make([]string, 0, len(oversize))
	for _, c := range oversize {
		names = append(names, fmt.Sprintf("%s (%d 字节)", c.RelPath, c.Size))
	}
	var sample string
	if len(names) <= maxAppsSensitiveListInError {
		sample = strings.Join(names, ", ")
	} else {
		sample = strings.Join(names[:maxAppsSensitiveListInError], ", ") +
			fmt.Sprintf("（还有 %d 个）", len(names)-maxAppsSensitiveListInError)
	}
	return fmt.Errorf("%d 个 .html 文件超过 %d 字节（10MB）单文件上限: %s\n妙搭服务端限制单个 .html 文件 ≤10MB，拆分或裁剪这些文件后重试", len(oversize), maxAppsSingleHTMLBytes, sample)
}

// appsOversizeHTMLSummary 供 dry-run 回填 oversize_html 字段。
func appsOversizeHTMLSummary(oversize []appsCandidate) []map[string]any {
	out := make([]map[string]any, 0, len(oversize))
	for _, c := range oversize {
		out = append(out, map[string]any{
			"path":  c.RelPath,
			"size":  c.Size,
			"limit": maxAppsSingleHTMLBytes,
		})
	}
	return out
}

// appsBuildTarball 把 candidates 打包成内存中的 tar.gz。
func appsBuildTarball(candidates []appsCandidate) ([]byte, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("没有可打包的文件")
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, c := range candidates {
		if err := appsWriteTarEntry(tw, c); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return nil, fmt.Errorf("tar 打包关闭失败: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("gzip 压缩关闭失败: %w", err)
	}
	return buf.Bytes(), nil
}

func appsWriteTarEntry(tw *tar.Writer, c appsCandidate) error {
	if appsIsUnsafeRelPath(c.RelPath) {
		return fmt.Errorf("非法 tar 条目名 %q", c.RelPath)
	}
	src, err := os.Open(c.AbsPath)
	if err != nil {
		return fmt.Errorf("打开文件 %s 失败: %w", c.AbsPath, err)
	}
	defer src.Close()

	hdr := &tar.Header{
		Name:     c.RelPath,
		Size:     c.Size,
		Mode:     0o644,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("写入 tar 头 %s 失败: %w", c.RelPath, err)
	}
	if _, err := io.Copy(tw, src); err != nil {
		return fmt.Errorf("写入文件内容 %s 失败: %w", c.RelPath, err)
	}
	return nil
}

// appsSensitiveError 构造凭证文件拦截错误（命中且未加 --allow-sensitive）。
func appsSensitiveError(hits []string) error {
	var sample string
	if len(hits) <= maxAppsSensitiveListInError {
		sample = strings.Join(hits, ", ")
	} else {
		sample = strings.Join(hits[:maxAppsSensitiveListInError], ", ") +
			fmt.Sprintf("（还有 %d 个）", len(hits)-maxAppsSensitiveListInError)
	}
	return fmt.Errorf("--path 含 %d 个不应发布的凭证文件: %s\n从发布内容里移除这些文件，或确实要发布时加 --allow-sensitive", len(hits), sample)
}

func init() {
	appsCmd.AddCommand(appsHTMLPublishCmd)
	appsHTMLPublishCmd.Flags().String("app-id", "", "妙搭应用 ID（必填）")
	appsHTMLPublishCmd.Flags().String("path", "", "HTML 文件或目录路径（必填）")
	appsHTMLPublishCmd.Flags().Bool("allow-sensitive", false, "跳过凭证文件扫描（放行 .env / .npmrc / .aws/credentials 等）")
	appsHTMLPublishCmd.Flags().Bool("wait", false, "发布后轮询 release 状态直到终态，输出 online_url / error_logs（失败时非零退出）")
	appsHTMLPublishCmd.Flags().Duration("wait-timeout", 5*time.Minute, "--wait 的最长等待时间")
	addAppsWriteFlags(appsHTMLPublishCmd)
}
