package cmd

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var driveStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "本地目录 ↔ 云盘文件夹 SHA-256 内容对照",
	Long: `递归列举 --folder-token 下的所有 type=file 条目，遍历 --local-dir 的所有 regular file，
按 SHA-256 内容哈希对照得到四个桶：

  - new_local：仅本地存在
  - new_remote：仅远端存在
  - modified：双方都有但内容不同
  - unchanged：双方都有且哈希一致

仅 type=file 参与对照；docx/sheet/bitable/mindnote/slides 等在线文档没有可哈希的本地等价文件，跳过。
只对两边都存在的文件计算哈希（本地与远端均流式计算，内存占用恒定）。

--quick：只比较本地 mtime 与远端 modified_time（不下载远端内容，速度快但为尽力而为的近似结果，
输出 detection=quick）；远端时间不可解析或两者不一致时归入 modified。

必填:
  --folder-token   云盘根文件夹 token
  --local-dir      本地根目录（必须在当前工作目录的子树内）

可选:
  --quick          只按修改时间比较，不下载远端内容
  --as             bot | user | auto（不传时保持旧行为：User 优先，不可用时告警回退 Bot）
  --output / -o    输出格式（json，默认人读）
  --user-access-token  覆盖登录态

权限:
  - User Access Token 或 Tenant Token
  - drive:drive.metadata:readonly
  - drive:file:download（--quick 不需要）

示例:
  feishu-cli drive status --folder-token fldxxx --local-dir ./mirror
  feishu-cli drive status --folder-token fldxxx --local-dir ./mirror --quick
  feishu-cli drive status --folder-token fldxxx --local-dir ./mirror -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		folderToken, _ := cmd.Flags().GetString("folder-token")
		localDir, _ := cmd.Flags().GetString("local-dir")
		output, _ := cmd.Flags().GetString("output")
		quick, _ := cmd.Flags().GetBool("quick")
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
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		safeRoot, _, err := resolveSafeLocalDir(localDir)
		if err != nil {
			return err
		}

		userToken, err := resolveMirrorIdentity(cmd, false, "drive status")
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "扫描本地: %s\n", safeRoot)
		localFiles, err := walkLocalRegularFiles(safeRoot)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "列举云盘文件夹: %s\n", folderToken)
		entries, err := client.ListFolderEntries(folderToken, userToken)
		if err != nil {
			return err
		}
		view, err := buildDriveMirrorView(entries, driveDuplicateRemoteFail)
		if err != nil {
			return err
		}
		remoteFiles := view.Files

		// 合并 path 集合
		paths := map[string]struct{}{}
		for p := range localFiles {
			paths[p] = struct{}{}
		}
		for p := range remoteFiles {
			paths[p] = struct{}{}
		}
		sortedPaths := make([]string, 0, len(paths))
		for p := range paths {
			sortedPaths = append(sortedPaths, p)
		}
		sort.Strings(sortedPaths)

		type entry struct {
			RelPath   string `json:"rel_path"`
			FileToken string `json:"file_token,omitempty"`
		}
		var newLocal, newRemote, modified, unchanged []entry

		// 只有两边都存在的文件才需要比较；先收集，再按模式比较
		var bothPaths []string
		for _, rel := range sortedPaths {
			_, hasLocal := localFiles[rel]
			_, hasRemote := remoteFiles[rel]
			if hasLocal && hasRemote {
				bothPaths = append(bothPaths, rel)
			}
		}

		same := make(map[string]bool, len(bothPaths))
		if quick {
			for _, rel := range bothPaths {
				info, statErr := os.Stat(localFiles[rel])
				if statErr != nil {
					return fmt.Errorf("读取本地文件信息失败 (%s): %w", rel, statErr)
				}
				c, ok := compareRemoteModifiedToLocal(remoteFiles[rel].ModifiedTime, info.ModTime())
				same[rel] = ok && c == 0
			}
		} else {
			bothLocal := make(map[string]string, len(bothPaths))
			for _, rel := range bothPaths {
				bothLocal[rel] = localFiles[rel]
			}
			// 本地 hash CPU bound，并发计算；只哈希两边都有的文件
			localHashes, err := concurrentHashLocal(bothLocal, workers)
			if err != nil {
				return err
			}
			remoteTokens := make(map[string]string, len(bothPaths))
			for _, rel := range bothPaths {
				remoteTokens[rel] = remoteFiles[rel].FileToken
			}
			remoteHashes, err := concurrentHashRemote(bothPaths, remoteTokens, userToken, workers)
			if err != nil {
				return err
			}
			for _, rel := range bothPaths {
				same[rel] = localHashes[rel] == remoteHashes[rel]
			}
		}

		for _, rel := range sortedPaths {
			_, hasLocal := localFiles[rel]
			remote, hasRemote := remoteFiles[rel]
			switch {
			case hasLocal && !hasRemote:
				newLocal = append(newLocal, entry{RelPath: rel})
			case !hasLocal && hasRemote:
				newRemote = append(newRemote, entry{RelPath: rel, FileToken: remote.FileToken})
			default:
				if same[rel] {
					unchanged = append(unchanged, entry{RelPath: rel, FileToken: remote.FileToken})
				} else {
					modified = append(modified, entry{RelPath: rel, FileToken: remote.FileToken})
				}
			}
		}

		detection := "exact"
		if quick {
			detection = "quick"
		}
		result := map[string]any{
			"detection":  detection,
			"new_local":  emptyOrSlice(newLocal),
			"new_remote": emptyOrSlice(newRemote),
			"modified":   emptyOrSlice(modified),
			"unchanged":  emptyOrSlice(unchanged),
		}

		if output == "json" {
			return printJSON(result)
		}

		printBucket := func(label string, items []entry) {
			fmt.Printf("[%s] %d 项\n", label, len(items))
			for _, it := range items {
				fmt.Printf("  %s", it.RelPath)
				if it.FileToken != "" {
					fmt.Printf("  (token=%s)", it.FileToken)
				}
				fmt.Println()
			}
		}
		if quick {
			fmt.Println("（--quick：按修改时间比较，结果为近似值）")
		}
		printBucket("仅本地 new_local", newLocal)
		printBucket("仅远端 new_remote", newRemote)
		printBucket("内容不同 modified", modified)
		fmt.Printf("[内容一致 unchanged] %d 项\n", len(unchanged))
		return nil
	},
}

// emptyOrSlice 把 nil 切片转为空切片，避免 JSON 出现 null。
func emptyOrSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// concurrentHashLocal 并发计算本地文件 SHA-256，限并发 workers。
func concurrentHashLocal(files map[string]string, workers int) (map[string]string, error) {
	out := make(map[string]string, len(files))
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for rel, abs := range files {
		rel, abs := rel, abs
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			h, err := client.HashLocalFile(abs)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("计算本地哈希失败 (%s): %w", rel, err)
				}
				return
			}
			out[rel] = h
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// concurrentHashRemote 并发拉取远端文件 SHA-256，限并发 workers。
func concurrentHashRemote(rels []string, remoteFiles map[string]string, userToken string, workers int) (map[string]string, error) {
	out := make(map[string]string, len(rels))
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for _, rel := range rels {
		rel := rel
		token := remoteFiles[rel]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			h, err := client.HashRemoteFile(token, userToken)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("计算远端哈希失败 (%s): %w", rel, err)
				}
				return
			}
			out[rel] = h
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func init() {
	driveCmd.AddCommand(driveStatusCmd)
	driveStatusCmd.Flags().String("folder-token", "", "云盘根文件夹 token（必填）")
	driveStatusCmd.Flags().String("local-dir", "", "本地根目录（必填，必须在 cwd 子树内）")
	driveStatusCmd.Flags().Int("workers", 4, "并发 hash worker 数（本地+远端）")
	driveStatusCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	driveStatusCmd.Flags().Bool("quick", false, "只比较本地 mtime 与远端 modified_time（不下载远端内容，结果为近似值）")
	driveStatusCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addLegacyAsFlag(driveStatusCmd, "保持旧行为（User 优先，不可用时告警回退 Bot）")
	mustMarkFlagRequired(driveStatusCmd, "folder-token")
	mustMarkFlagRequired(driveStatusCmd, "local-dir")
}
