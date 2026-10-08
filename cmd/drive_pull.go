package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
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

可选 --delete-local --yes 同时清理本地不存在于远端的 regular file（高危，必须双确认）。
失败时不会触发删除阶段，避免「半同步」状态。

必填:
  --folder-token   云盘根文件夹 token
  --local-dir      本地根目录（必须在 cwd 子树内）

可选:
  --if-exists       overwrite（默认）/ skip：本地同路径已存在时如何处理
  --delete-local    清理本地不存在于远端的 regular file（高危）
  --yes             与 --delete-local 配套，确认删除
  --output / -o     输出格式（json）
  --user-access-token  覆盖登录态

权限:
  - User Access Token 或 Tenant Token
  - drive:drive.metadata:readonly
  - drive:file:download

示例:
  feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror
  feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --if-exists skip
  feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --delete-local --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		folderToken, _ := cmd.Flags().GetString("folder-token")
		localDir, _ := cmd.Flags().GetString("local-dir")
		ifExists, _ := cmd.Flags().GetString("if-exists")
		deleteLocal, _ := cmd.Flags().GetBool("delete-local")
		yes, _ := cmd.Flags().GetBool("yes")
		output, _ := cmd.Flags().GetString("output")
		workers, _ := cmd.Flags().GetInt("workers")
		if workers < 1 {
			workers = 1
		}

		if folderToken == "" {
			return fmt.Errorf("--folder-token 必填")
		}
		if localDir == "" {
			return fmt.Errorf("--local-dir 必填")
		}
		if ifExists == "" {
			ifExists = driveMirrorIfExistsOverwrite
		}
		if ifExists != driveMirrorIfExistsOverwrite && ifExists != driveMirrorIfExistsSkip {
			return fmt.Errorf("--if-exists 只能是 overwrite 或 skip")
		}
		if deleteLocal && !yes {
			return clierr.ConfirmationRequiredf("--delete-local 是高危操作，必须同时加 --yes 才执行")
		}

		safeRoot, _, err := resolveSafeLocalDir(localDir)
		if err != nil {
			return err
		}

		// --delete-local 会删本地文件：身份意外降级到 Bot 时远端视图更小、差集更大，
		// 必须 fail-closed 而不是静默按 Bot 执行
		var userToken string
		if deleteLocal {
			userToken, err = resolveOptionalUserTokenForDestructive(cmd, "drive pull --delete-local")
			if err != nil {
				return err
			}
		} else {
			userToken = resolveOptionalUserTokenWithFallback(cmd)
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "列举云盘文件夹: %s\n", folderToken)
		entries, err := client.ListFolderRecursive(folderToken, userToken)
		if err != nil {
			return err
		}
		remoteFiles := remoteFilesOnly(entries)
		// remotePaths 含 folder/docx/sheet 等所有条目，用于 --delete-local 守门
		remotePaths := make(map[string]struct{}, len(entries))
		for rel := range entries {
			remotePaths[rel] = struct{}{}
		}

		type item struct {
			RelPath   string `json:"rel_path"`
			FileToken string `json:"file_token,omitempty"`
			Action    string `json:"action"` // downloaded / skipped / failed / deleted_local / delete_failed
			Error     string `json:"error,omitempty"`
		}

		// 稳定顺序
		sortedRels := make([]string, 0, len(remoteFiles))
		for rel := range remoteFiles {
			sortedRels = append(sortedRels, rel)
		}
		sort.Strings(sortedRels)

		// 并发下载：每个 rel 写入 results[idx]，避免锁；计数器用 atomic
		results := make([]item, len(sortedRels))
		var downloadedCnt, skippedCnt, downloadFailedCnt int64
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for i, rel := range sortedRels {
			i, rel := i, rel
			token := remoteFiles[rel]
			target := filepath.Join(safeRoot, filepath.FromSlash(rel))

			if info, statErr := os.Stat(target); statErr == nil {
				if info.IsDir() {
					results[i] = item{
						RelPath:   rel,
						FileToken: token,
						Action:    "failed",
						Error:     "本地同路径是目录，远端是文件",
					}
					atomic.AddInt64(&downloadFailedCnt, 1)
					continue
				}
				if ifExists == driveMirrorIfExistsSkip {
					results[i] = item{RelPath: rel, FileToken: token, Action: "skipped"}
					atomic.AddInt64(&skippedCnt, 1)
					continue
				}
			}

			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				if mkErr := os.MkdirAll(filepath.Dir(target), 0755); mkErr != nil {
					results[i] = item{RelPath: rel, FileToken: token, Action: "failed", Error: mkErr.Error()}
					atomic.AddInt64(&downloadFailedCnt, 1)
					return
				}
				if dlErr := client.DownloadFileWithToken(token, target, userToken); dlErr != nil {
					results[i] = item{RelPath: rel, FileToken: token, Action: "failed", Error: dlErr.Error()}
					atomic.AddInt64(&downloadFailedCnt, 1)
					return
				}
				results[i] = item{RelPath: rel, FileToken: token, Action: "downloaded"}
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
		if deleteLocal && downloadFailed == 0 {
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
		} else if deleteLocal && downloadFailed > 0 {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"⚠ 跳过 --delete-local：上面有 %d 个下载失败，避免半同步状态。修复后重跑。\n", downloadFailed)
		}

		summary := map[string]any{
			"downloaded":    downloaded,
			"skipped":       skipped,
			"failed":        failed,
			"deleted_local": deletedLocal,
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

		if failed > 0 {
			return fmt.Errorf("有 %d 项失败，处于部分同步状态；修复后重跑", failed)
		}
		return nil
	},
}

func init() {
	driveCmd.AddCommand(drivePullCmd)
	drivePullCmd.Flags().String("folder-token", "", "云盘根文件夹 token（必填）")
	drivePullCmd.Flags().String("local-dir", "", "本地根目录（必填）")
	drivePullCmd.Flags().String("if-exists", driveMirrorIfExistsOverwrite, "overwrite / skip")
	drivePullCmd.Flags().Bool("delete-local", false, "清理本地不存在于远端的文件（高危，需 --yes）")
	drivePullCmd.Flags().Bool("yes", false, "与 --delete-local 配套确认删除")
	drivePullCmd.Flags().Int("workers", 4, "并发下载 worker 数")
	drivePullCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	drivePullCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(drivePullCmd, "folder-token")
	mustMarkFlagRequired(drivePullCmd, "local-dir")
}
