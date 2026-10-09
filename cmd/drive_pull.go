package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

const (
	driveMirrorIfExistsOverwrite = "overwrite"
	driveMirrorIfExistsSkip      = "skip"
)

var drivePullCmd = &cobra.Command{
	Use:   "pull",
	Short: "把云盘文件夹镜像到本地（Drive → 本地，单向 file-level 镜像）",
	Long: `递归列举 --folder-token 下的所有 type=file 条目，下载到 --local-dir 的对应路径。
type=folder/docx/sheet/bitable/mindnote/slides/shortcut 不会作为可下载条目（在线文档没有等价本地文件）。

下载为流式（User/Bot 身份一致，无 100MB 上限）：分片级有界重试 + 断点续传、空闲超时，
写盘走临时文件 + rename，失败不会留下半截文件或破坏本地已有文件。下载完成后本地 mtime 对齐远端 modified_time。

可选 --delete-local --yes 同时清理本地不存在于远端的 regular file（高危，必须双确认）。
失败时不会触发删除阶段，避免「半同步」状态。缺 scope、无权限、限流、参数错误等"重跑同一批也不会成功"
的错误会终止整批（summary.aborted=true）并给出分类提示。

必填:
  --folder-token   云盘根文件夹 token
  --local-dir      本地根目录（必须在 cwd 子树内）

可选:
  --if-exists            overwrite（默认）/ skip / smart：本地同路径已存在时如何处理
                         smart = 本地 mtime 不早于远端 modified_time 时跳过（推荐的增量模式）
  --on-duplicate-remote  fail（默认）/ rename / newest / oldest：远端多个文件映射到同一路径时的处理
                         rename = 副本以 __lark_<哈希> 后缀另存；folder/在线文档与文件重名始终报错
  --delete-local         清理本地不存在于远端的 regular file（高危）
  --yes                  与 --delete-local 配套，确认删除
  --as                   bot | user | auto（不传时保持旧行为：User 优先，不可用时告警回退 Bot；
                         --delete-local 下已配置 User 但不可用则 fail-closed）
  --output / -o          输出格式（json）
  --user-access-token    覆盖登录态

权限:
  - User Access Token 或 Tenant Token
  - drive:drive.metadata:readonly
  - drive:file:download

示例:
  feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror
  feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --if-exists smart
  feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --as bot
  feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --delete-local --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		folderToken, _ := cmd.Flags().GetString("folder-token")
		localDir, _ := cmd.Flags().GetString("local-dir")
		ifExists, _ := cmd.Flags().GetString("if-exists")
		duplicatePolicy, _ := cmd.Flags().GetString("on-duplicate-remote")
		deleteLocal, _ := cmd.Flags().GetBool("delete-local")
		yes, _ := cmd.Flags().GetBool("yes")
		output, _ := cmd.Flags().GetString("output")
		workers, _ := cmd.Flags().GetInt("workers")
		if workers < 1 {
			workers = 1
		}

		if folderToken == "" {
			return clierr.Usagef("--folder-token 必填")
		}
		if localDir == "" {
			return clierr.Usagef("--local-dir 必填")
		}
		if ifExists == "" {
			ifExists = driveMirrorIfExistsOverwrite
		}
		if ifExists != driveMirrorIfExistsOverwrite && ifExists != driveMirrorIfExistsSkip && ifExists != driveMirrorIfExistsSmart {
			return clierr.Usagef("--if-exists 只能是 overwrite、skip 或 smart")
		}
		if duplicatePolicy == "" {
			duplicatePolicy = driveDuplicateRemoteFail
		}
		if err := validateDuplicateRemotePolicy(duplicatePolicy, true); err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		// 本地目录先于确认门禁校验：路径本身不合法（用法错误）时不该让调用方先去补 --yes
		safeRoot, _, err := resolveSafeLocalDir(localDir, true)
		if err != nil {
			return err
		}
		if deleteLocal && !yes && !confirmationBypassed(cmd) {
			return clierr.ConfirmationRequiredf("--delete-local 是高危操作，必须同时加 --yes 才执行")
		}

		// --delete-local 会删本地文件：身份意外降级到 Bot 时远端视图更小、差集更大，
		// 必须 fail-closed 而不是静默按 Bot 执行
		userToken, err := resolveMirrorIdentity(cmd, deleteLocal, "drive pull --delete-local")
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "列举云盘文件夹: %s\n", folderToken)
		entries, err := client.ListFolderEntries(folderToken, userToken)
		if err != nil {
			return err
		}
		view, err := buildDriveMirrorView(entries, duplicatePolicy)
		if err != nil {
			return err
		}
		remoteFiles := view.Files
		// remotePaths 含 folder/docx/sheet 等所有条目，用于 --delete-local 守门
		remotePaths := view.Paths

		type item struct {
			RelPath    string `json:"rel_path"`
			FileToken  string `json:"file_token,omitempty"`
			SourceID   string `json:"source_id,omitempty"`
			Action     string `json:"action"` // downloaded / skipped / failed / deleted_local / delete_failed
			Error      string `json:"error,omitempty"`
			ErrorClass string `json:"error_class,omitempty"`
			Code       int    `json:"code,omitempty"`
		}
		failItem := func(rel string, f driveMirrorFile, action string, e error) (item, driveBatchFailure) {
			d := classifyDriveBatchFailure(e, false)
			return item{RelPath: rel, FileToken: f.FileToken, SourceID: f.SourceID, Action: action, Error: e.Error(), ErrorClass: d.Class, Code: d.Code}, d
		}

		// 稳定顺序
		sortedRels := make([]string, 0, len(remoteFiles))
		for rel := range remoteFiles {
			sortedRels = append(sortedRels, rel)
		}
		sort.Strings(sortedRels)

		// 并发下载：每个 rel 写入 results[idx]，避免锁；计数器用 atomic
		results := make([]item, len(sortedRels))
		var downloadedCnt, skippedCnt, downloadFailedCnt, notAttemptedCnt int64
		var aborted atomic.Bool
		var abortOnce sync.Once
		var abortInfo driveBatchFailure
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for i, rel := range sortedRels {
			i, rel := i, rel
			f := remoteFiles[rel]
			target, pathErr := safeMirrorTarget(safeRoot, rel)
			if pathErr != nil {
				results[i], _ = failItem(rel, f, "failed", pathErr)
				results[i].ErrorClass = "unsafe_path"
				atomic.AddInt64(&downloadFailedCnt, 1)
				continue
			}

			if info, statErr := os.Stat(target); statErr == nil {
				if info.IsDir() {
					results[i] = item{RelPath: rel, FileToken: f.FileToken, SourceID: f.SourceID, Action: "failed", Error: "本地同路径是目录，远端是文件", ErrorClass: "local_conflict"}
					atomic.AddInt64(&downloadFailedCnt, 1)
					continue
				}
				if ifExists == driveMirrorIfExistsSkip || (ifExists == driveMirrorIfExistsSmart && localUpToDate(info.ModTime(), f.ModifiedTime)) {
					results[i] = item{RelPath: rel, FileToken: f.FileToken, SourceID: f.SourceID, Action: "skipped"}
					atomic.AddInt64(&skippedCnt, 1)
					continue
				}
			}

			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				if aborted.Load() {
					atomic.AddInt64(&notAttemptedCnt, 1)
					return
				}
				if mkErr := safefile.MkdirAll(filepath.Dir(target), 0755); mkErr != nil {
					results[i], _ = failItem(rel, f, "failed", mkErr)
					atomic.AddInt64(&downloadFailedCnt, 1)
					return
				}
				if dlErr := client.DownloadFileWithToken(f.FileToken, target, userToken); dlErr != nil {
					it, d := failItem(rel, f, "failed", dlErr)
					results[i] = it
					atomic.AddInt64(&downloadFailedCnt, 1)
					if d.Terminal {
						abortOnce.Do(func() { abortInfo = d })
						aborted.Store(true)
					}
					return
				}
				if mtErr := applyRemoteModifiedTime(target, f.ModifiedTime); mtErr != nil {
					fmt.Fprintf(os.Stderr, "⚠ 已下载 %s，但未能对齐远端修改时间: %v\n", rel, mtErr)
				}
				results[i] = item{RelPath: rel, FileToken: f.FileToken, SourceID: f.SourceID, Action: "downloaded"}
				atomic.AddInt64(&downloadedCnt, 1)
			}()
		}
		wg.Wait()

		items := make([]item, 0, len(results))
		for _, it := range results {
			if it.Action != "" {
				items = append(items, it)
			}
		}
		downloaded := int(downloadedCnt)
		skipped := int(skippedCnt)
		downloadFailed := int(downloadFailedCnt)
		failed := downloadFailed
		deletedLocal := 0

		// --delete-local 在下载阶段无失败时才执行，避免半同步状态
		if deleteLocal && downloadFailed == 0 && !aborted.Load() {
			localFiles, walkErr := walkLocalRegularFiles(safeRoot)
			if walkErr != nil {
				return walkErr
			}
			locals := make([]string, 0, len(localFiles))
			for rel := range localFiles {
				locals = append(locals, rel)
			}
			sort.Strings(locals)

			for _, rel := range locals {
				if _, ok := remotePaths[rel]; ok {
					// 即使 type 不是 file（如 docx 在线文档同名），也保留本地文件不删
					continue
				}
				abs := localFiles[rel]
				if rmErr := os.Remove(abs); rmErr != nil {
					items = append(items, item{RelPath: rel, Action: "delete_failed", Error: rmErr.Error()})
					failed++
					continue
				}
				items = append(items, item{RelPath: rel, Action: "deleted_local"})
				deletedLocal++
			}
		} else if deleteLocal && (downloadFailed > 0 || aborted.Load()) {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"⚠ 跳过 --delete-local：上面有 %d 个下载失败，避免半同步状态。修复后重跑。\n", downloadFailed)
		}

		summary := map[string]any{
			"downloaded":    downloaded,
			"skipped":       skipped,
			"failed":        failed,
			"deleted_local": deletedLocal,
			"aborted":       aborted.Load(),
		}
		if aborted.Load() {
			summary["not_attempted"] = int(notAttemptedCnt)
			summary["abort_reason"] = abortInfo.Class
		}
		payload := map[string]any{
			"summary": summary,
			"items":   items,
		}

		if output == "json" {
			if err := printJSON(payload); err != nil {
				return err
			}
		} else {
			fmt.Printf("下载: %d  跳过: %d  删除本地: %d  失败: %d\n",
				downloaded, skipped, deletedLocal, failed)
			for _, it := range items {
				if it.Action == "failed" || it.Action == "delete_failed" {
					fmt.Printf("  ⚠ %-15s %s -- %s\n", it.Action, it.RelPath, it.Error)
				}
			}
		}

		if aborted.Load() {
			fmt.Fprintf(cmd.ErrOrStderr(), "\n✖ 已终止整批下载（%s），%d 项未尝试。%s\n", abortInfo.Class, notAttemptedCnt, abortInfo.Hint)
			return fmt.Errorf("drive pull 因 %s 终止：%d 项失败、%d 项未尝试；%s", abortInfo.Class, failed, notAttemptedCnt, abortInfo.Hint)
		}
		if failed > 0 {
			return fmt.Errorf("有 %d 项失败，处于部分同步状态；修复后重跑", failed)
		}
		return nil
	},
}

// localUpToDate 判断 smart 模式下本地文件是否已不旧于远端（远端 modified_time 不可解析时视为需要下载）。
func localUpToDate(localMod time.Time, remoteModified string) bool {
	c, ok := compareRemoteModifiedToLocal(remoteModified, localMod)
	return ok && c <= 0
}

func init() {
	driveCmd.AddCommand(drivePullCmd)
	drivePullCmd.Flags().String("folder-token", "", "云盘根文件夹 token（必填）")
	drivePullCmd.Flags().String("local-dir", "", "本地根目录（必填）")
	drivePullCmd.Flags().String("if-exists", driveMirrorIfExistsOverwrite, "overwrite / skip / smart（本地不旧于远端时跳过）")
	drivePullCmd.Flags().String("on-duplicate-remote", driveDuplicateRemoteFail, "远端重名处理: fail / rename / newest / oldest")
	drivePullCmd.Flags().Bool("delete-local", false, "清理本地不存在于远端的文件（高危，需 --yes）")
	drivePullCmd.Flags().Bool("yes", false, "与 --delete-local 配套确认删除")
	drivePullCmd.Flags().Int("workers", 4, "并发下载 worker 数")
	drivePullCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	drivePullCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addLegacyAsFlag(drivePullCmd, "保持旧行为（User 优先，不可用时告警回退 Bot；--delete-local 时 fail-closed）")
	mustMarkFlagRequired(drivePullCmd, "folder-token")
	mustMarkFlagRequired(drivePullCmd, "local-dir")
}
