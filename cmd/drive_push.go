package cmd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// driveFolderChildLimitAdvice 是命中飞书 1062507（父目录直接子节点超 1500）时的中文清理建议。
const driveFolderChildLimitAdvice = "目标父文件夹的直接子节点已达上限（1500，错误码 1062507）。" +
	"请先在该文件夹下清理/归档部分文件或子文件夹腾出空间，或在本地把文件拆分到更细的子目录以分散到多个父文件夹，再重跑 push。"

// isDriveFolderChildLimitErr 判断上传/建文件夹错误是否为 1062507（父目录直接子节点超 1500）。
// 该上限是单个父文件夹级的终态错误：对同一父目录重试必然再撞墙，应跳过其下所有条目；
// 但其余未满目录应继续镜像（见 fullParents 隔离逻辑）。
func isDriveFolderChildLimitErr(err error) bool {
	return client.HasAPICode(err, 1062507)
}

var drivePushCmd = &cobra.Command{
	Use:   "push",
	Short: "把本地目录镜像到云盘文件夹（本地 → Drive，单向 file-level 镜像）",
	Long: `递归遍历 --local-dir 下的所有 regular file，上传到 --folder-token 下的对应路径。
本地目录会通过 /open-apis/drive/v1/files/create_folder 在远端按需创建以镜像目录结构。

可选 --delete-remote --yes 同时清理远端 type=file 但本地不存在的文件（高危，必须双确认）。
docx/sheet/bitable/mindnote/slides/shortcut 等在线文档不会被作为孤儿删除。
失败时不会触发删除阶段，避免「半同步」状态。缺 scope、无权限、限流、参数错误、服务端错误等
"重跑同一批也不会成功"的错误会终止整批（summary.aborted=true）并给出分类提示；
父目录子节点超 1500（1062507）只跳过发往该目录的条目，其余目录继续镜像。

必填:
  --folder-token   云盘根文件夹 token
  --local-dir      本地根目录（必须在 cwd 子树内）

可选:
  --if-exists            skip（默认）/ overwrite / smart：远端同路径已存在时如何处理
                         overwrite = 原地覆盖：upload_all（>20MB 走 upload_prepare）携带 file_token，
                                     file_token 不变，链接/协作者/评论/历史版本保留，生成新版本；
                                     覆盖失败直接报错，绝不"先删后传"；租户未支持覆盖字段时报错，请改用 skip
                         smart     = 远端 modified_time 不早于本地 mtime 时跳过，否则按 overwrite 覆盖
  --on-duplicate-remote  fail（默认）/ newest / oldest：远端多个文件映射到同一路径时以哪个为准
  --delete-remote        清理远端不存在于本地的 file（高危）
  --yes                  与 --delete-remote 配套，确认删除
  --as                   bot | user | auto（不传时保持旧行为：User 优先，不可用时告警回退 Bot；
                         --delete-remote 下已配置 User 但不可用则 fail-closed）
  --output / -o          输出格式（json）
  --user-access-token    覆盖登录态

权限:
  - User Access Token 或 Tenant Token
  - drive:drive.metadata:readonly
  - drive:file:upload
  - 删除远端时需 space:document:delete

示例:
  feishu-cli drive push --folder-token fldxxx --local-dir ./mirror
  feishu-cli drive push --folder-token fldxxx --local-dir ./mirror --if-exists overwrite
  feishu-cli drive push --folder-token fldxxx --local-dir ./mirror --if-exists smart
  feishu-cli drive push --folder-token fldxxx --local-dir ./mirror --delete-remote --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		folderToken, _ := cmd.Flags().GetString("folder-token")
		localDir, _ := cmd.Flags().GetString("local-dir")
		ifExists, _ := cmd.Flags().GetString("if-exists")
		duplicatePolicy, _ := cmd.Flags().GetString("on-duplicate-remote")
		deleteRemote, _ := cmd.Flags().GetBool("delete-remote")
		yes, _ := cmd.Flags().GetBool("yes")
		output, _ := cmd.Flags().GetString("output")

		if folderToken == "" {
			return clierr.Usagef("--folder-token 必填")
		}
		if localDir == "" {
			return clierr.Usagef("--local-dir 必填")
		}
		if ifExists == "" {
			ifExists = driveMirrorIfExistsSkip
		}
		if ifExists != driveMirrorIfExistsOverwrite && ifExists != driveMirrorIfExistsSkip && ifExists != driveMirrorIfExistsSmart {
			return clierr.Usagef("--if-exists 只能是 overwrite、skip 或 smart")
		}
		if duplicatePolicy == "" {
			duplicatePolicy = driveDuplicateRemoteFail
		}
		if err := validateDuplicateRemotePolicy(duplicatePolicy, false); err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if deleteRemote && !yes && !confirmationBypassed(cmd) {
			return clierr.ConfirmationRequiredf("--delete-remote 是高危操作，必须同时加 --yes 才执行")
		}

		safeRoot, _, err := resolveSafeLocalDir(localDir)
		if err != nil {
			return err
		}

		// --delete-remote 会删远端文件：身份降级会让"本地不存在"的判定基于错误的远端视图，
		// 必须 fail-closed 而不是静默按 Bot 执行
		userToken, err := resolveMirrorIdentity(cmd, deleteRemote, "drive push --delete-remote")
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "扫描本地: %s\n", safeRoot)
		localFiles, err := walkLocalRegularFiles(safeRoot)
		if err != nil {
			return err
		}
		localDirs, err := walkLocalDirs(safeRoot)
		if err != nil {
			return err
		}
		// 扫描时的本地快照（大小 + mtime），上传前复核，防止把扫描后被修改的文件当作同一版本推送
		snapshots := make(map[string]os.FileInfo, len(localFiles))
		for rel, abs := range localFiles {
			info, statErr := os.Stat(abs)
			if statErr != nil {
				return fmt.Errorf("读取本地文件信息失败 (%s): %w", rel, statErr)
			}
			snapshots[rel] = info
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

		// folderCache: relDir → folder_token，root 关联到 folderToken
		folderCache := map[string]string{"": folderToken}
		for rel, tok := range view.Folders {
			folderCache[rel] = tok
		}

		type item struct {
			RelPath    string `json:"rel_path"`
			FileToken  string `json:"file_token,omitempty"`
			Version    string `json:"version,omitempty"`
			Action     string `json:"action"` // uploaded / overwritten / skipped / failed / folder_created / deleted_remote / delete_failed
			Error      string `json:"error,omitempty"`
			ErrorClass string `json:"error_class,omitempty"`
			Code       int    `json:"code,omitempty"`
		}
		var items []item
		var uploaded, overwritten, skipped, failed, deletedRemote int
		uploadFailed := false
		aborted := false
		var abortInfo driveBatchFailure
		// recordFailure 记录单项失败并按分级决定是否终止整批（1062507 走 fullParents 隔离，不终止）。
		recordFailure := func(rel, token string, e error) {
			d := classifyDriveBatchFailure(e, true)
			items = append(items, item{RelPath: rel, FileToken: token, Action: "failed", Error: e.Error(), ErrorClass: d.Class, Code: d.Code})
			failed++
			uploadFailed = true
			if d.Terminal && !aborted {
				aborted = true
				abortInfo = d
			}
		}
		// fullParents：命中 1062507 的父目录集合。1500 子节点上限是**单个父文件夹**级的，
		// 只跳过发往已满目录（及其子树）的条目，其余目录继续镜像——整体 break 会无辜
		// 放弃发往未满兄弟目录/根目录的文件，降低镜像完整度。
		fullParents := map[string]bool{}
		markParentFull := func(rel string) { fullParents[pushParentRel(rel)] = true }
		underFullParent := func(rel string) bool {
			for p := pushParentRel(rel); ; p = pushParentRel(p) {
				if fullParents[p] {
					return true
				}
				if p == "" {
					return false
				}
			}
		}

		// 先按本地目录创建远端文件夹（保证空目录也被镜像）
		sort.Strings(localDirs)
		for _, relDir := range localDirs {
			if aborted {
				break
			}
			if _, ok := folderCache[relDir]; ok {
				continue
			}
			if underFullParent(relDir) {
				items = append(items, item{RelPath: relDir, Action: "failed", Error: "父目录子节点已满（1062507），跳过", ErrorClass: "parent_sibling_limit"})
				failed++
				uploadFailed = true
				continue
			}
			tok, fErr := ensureRemoteFolder(folderToken, relDir, folderCache, userToken)
			if fErr != nil {
				recordFailure(relDir, "", fErr)
				if isDriveFolderChildLimitErr(fErr) {
					markParentFull(relDir)
				}
				continue
			}
			items = append(items, item{RelPath: relDir, FileToken: tok, Action: "folder_created"})
		}

		// 再上传文件
		localPaths := make([]string, 0, len(localFiles))
		for rel := range localFiles {
			localPaths = append(localPaths, rel)
		}
		sort.Strings(localPaths)

		notAttempted := 0
		for idx, rel := range localPaths {
			if aborted {
				notAttempted = len(localPaths) - idx
				break
			}
			if underFullParent(rel) {
				items = append(items, item{RelPath: rel, Action: "failed", Error: "父目录子节点已满（1062507），跳过", ErrorClass: "parent_sibling_limit"})
				failed++
				uploadFailed = true
				continue
			}
			abs := localFiles[rel]
			existing, has := remoteFiles[rel]

			if has {
				if ifExists == driveMirrorIfExistsSkip {
					items = append(items, item{RelPath: rel, FileToken: existing.FileToken, Action: "skipped"})
					skipped++
					continue
				}
				if ifExists == driveMirrorIfExistsSmart {
					if c, ok := compareRemoteModifiedToLocal(existing.ModifiedTime, snapshots[rel].ModTime()); ok && c >= 0 {
						items = append(items, item{RelPath: rel, FileToken: existing.FileToken, Action: "skipped"})
						skipped++
						continue
					}
				}
			}

			parent := pushParentRel(rel)
			parentToken, ensureErr := ensureRemoteFolder(folderToken, parent, folderCache, userToken)
			if ensureErr != nil {
				recordFailure(rel, existing.FileToken, ensureErr)
				if isDriveFolderChildLimitErr(ensureErr) {
					markParentFull(rel)
				}
				continue
			}
			if snapErr := verifyPushSnapshot(abs, snapshots[rel]); snapErr != nil {
				recordFailure(rel, existing.FileToken, snapErr)
				continue
			}

			if has {
				// overwrite / smart（远端更旧）：原地覆盖，file_token 不变；失败直接报错，绝不先删后传
				res, owErr := client.OverwriteDriveFileFromPath(abs, parentToken, filepath.Base(abs), existing.FileToken, userToken)
				if owErr != nil {
					recordFailure(rel, existing.FileToken, owErr)
					continue
				}
				items = append(items, item{RelPath: rel, FileToken: res.FileToken, Version: res.Version, Action: "overwritten"})
				overwritten++
				continue
			}

			// 新文件
			newToken, upErr := client.UploadFileWithToken(abs, parentToken, filepath.Base(abs), userToken)
			if upErr != nil {
				recordFailure(rel, "", upErr)
				if isDriveFolderChildLimitErr(upErr) {
					markParentFull(rel)
				}
				continue
			}
			items = append(items, item{RelPath: rel, FileToken: newToken, Action: "uploaded"})
			uploaded++
		}

		// --delete-remote 在上传阶段无失败时才执行
		if deleteRemote && !uploadFailed && !aborted {
			remotePaths := make([]string, 0, len(remoteFiles))
			for rel := range remoteFiles {
				remotePaths = append(remotePaths, rel)
			}
			sort.Strings(remotePaths)
			for _, rel := range remotePaths {
				if _, has := localFiles[rel]; has {
					continue
				}
				token := remoteFiles[rel].FileToken
				if delErr := client.DeleteDriveFileByToken(token, userToken); delErr != nil {
					d := classifyDriveBatchFailure(delErr, true)
					items = append(items, item{RelPath: rel, FileToken: token, Action: "delete_failed", Error: delErr.Error(), ErrorClass: d.Class, Code: d.Code})
					failed++
					if d.Terminal {
						aborted = true
						abortInfo = d
						break
					}
					continue
				}
				items = append(items, item{RelPath: rel, FileToken: token, Action: "deleted_remote"})
				deletedRemote++
			}
		} else if deleteRemote && (uploadFailed || aborted) {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"⚠ 跳过 --delete-remote：上面有 %d 个上传失败，避免半同步状态。修复后重跑。\n", failed)
		}

		summary := map[string]any{
			"uploaded":       uploaded + overwritten,
			"overwritten":    overwritten,
			"skipped":        skipped,
			"failed":         failed,
			"deleted_remote": deletedRemote,
			"aborted":        aborted,
		}
		if aborted {
			summary["not_attempted"] = notAttempted
			summary["abort_reason"] = abortInfo.Class
		}
		payload := map[string]any{"summary": summary, "items": items}

		if output == "json" {
			if err := printJSON(payload); err != nil {
				return err
			}
		} else {
			fmt.Printf("上传: %d（其中覆盖 %d）  跳过: %d  删除远端: %d  失败: %d\n",
				uploaded+overwritten, overwritten, skipped, deletedRemote, failed)
			for _, it := range items {
				if it.Action == "failed" || it.Action == "delete_failed" {
					fmt.Printf("  ⚠ %-15s %s -- %s\n", it.Action, it.RelPath, it.Error)
				}
			}
		}

		if aborted {
			fmt.Fprintf(cmd.ErrOrStderr(), "\n✖ 已终止整批推送（%s），%d 个文件未尝试。%s\n", abortInfo.Class, notAttempted, abortInfo.Hint)
			return fmt.Errorf("drive push 因 %s 终止：%d 项失败、%d 个文件未尝试；%s", abortInfo.Class, failed, notAttempted, abortInfo.Hint)
		}
		if len(fullParents) > 0 {
			dirs := make([]string, 0, len(fullParents))
			for d := range fullParents {
				if d == "" {
					d = "(根目录)"
				}
				dirs = append(dirs, d)
			}
			sort.Strings(dirs)
			fmt.Fprintf(cmd.ErrOrStderr(), "\n✖ 以下目录已满，其下条目被跳过（其余目录已继续镜像）: %s\n%s\n",
				strings.Join(dirs, ", "), driveFolderChildLimitAdvice)
			return fmt.Errorf("部分目录子节点已满（%s），发往这些目录的 %d 项失败/跳过；其余已完成（上传 %d，跳过 %d）",
				strings.Join(dirs, ", "), failed, uploaded+overwritten, skipped)
		}
		if failed > 0 {
			return fmt.Errorf("有 %d 项失败，处于部分同步状态；修复后重跑", failed)
		}
		return nil
	},
}

// verifyPushSnapshot 上传前复核本地文件仍与扫描时一致（仍为普通文件、大小与 mtime 未变）。
func verifyPushSnapshot(abs string, snap os.FileInfo) error {
	if snap == nil {
		return nil
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("%w：%s 已不可读: %v", errDriveLocalFileChanged, abs, err)
	}
	if !info.Mode().IsRegular() || info.Size() != snap.Size() || !info.ModTime().Equal(snap.ModTime()) {
		return fmt.Errorf("%w：%s（大小或修改时间已变化），请重新运行 push", errDriveLocalFileChanged, abs)
	}
	return nil
}

// pushParentRel 取 rel_path（用 / 分隔）的父目录 rel_path。"" 表示根。
func pushParentRel(rel string) string {
	d := path.Dir(rel)
	if d == "." {
		return ""
	}
	return d
}

// ensureRemoteFolder 保证 relDir 在远端存在，返回其 folder_token。
// folderCache 既作为已有缓存（避免重复创建），也会被本函数填充新创建的 folder。
func ensureRemoteFolder(rootToken, relDir string, folderCache map[string]string, userToken string) (string, error) {
	if relDir == "" {
		return rootToken, nil
	}
	if tok, ok := folderCache[relDir]; ok {
		return tok, nil
	}
	parentToken, err := ensureRemoteFolder(rootToken, pushParentRel(relDir), folderCache, userToken)
	if err != nil {
		return "", err
	}
	tok, _, err := client.CreateFolder(path.Base(relDir), parentToken, userToken)
	if err != nil {
		return "", err
	}
	folderCache[relDir] = tok
	return tok, nil
}

func init() {
	driveCmd.AddCommand(drivePushCmd)
	drivePushCmd.Flags().String("folder-token", "", "云盘根文件夹 token（必填）")
	drivePushCmd.Flags().String("local-dir", "", "本地根目录（必填）")
	drivePushCmd.Flags().String("if-exists", driveMirrorIfExistsSkip, "skip（默认）/ overwrite（原地覆盖，file_token 不变）/ smart（远端不旧于本地时跳过）")
	drivePushCmd.Flags().String("on-duplicate-remote", driveDuplicateRemoteFail, "远端重名处理: fail / newest / oldest")
	drivePushCmd.Flags().Bool("delete-remote", false, "清理远端不存在于本地的文件（高危，需 --yes）")
	drivePushCmd.Flags().Bool("yes", false, "与 --delete-remote 配套确认删除")
	drivePushCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	drivePushCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addLegacyAsFlag(drivePushCmd, "保持旧行为（User 优先，不可用时告警回退 Bot；--delete-remote 时 fail-closed）")
	mustMarkFlagRequired(drivePushCmd, "folder-token")
	mustMarkFlagRequired(drivePushCmd, "local-dir")
}
