package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var exportWikiTreeCmd = &cobra.Command{
	Use:   "export-tree <node_token|url>",
	Short: "递归导出整个知识库子树为本地目录",
	Long: `以指定节点为根，递归遍历整个知识库子树，按 wiki 目录结构镜像到本地。

输出布局:
  ./<output-dir>/<根节点标题>.md                     # 根节点本身
  ./<output-dir>/<子目录>/<子目录>.md                # 有子节点的节点：自身放在同名目录里
  ./<output-dir>/<子目录>/<叶子节点>.md              # 叶子节点直接放在父目录

支持的节点类型（默认仅 docx + sheet，与 wiki export 一致）:
  docx      新版文档（完整支持）
  sheet     电子表格（读取数据转为 Markdown 表格）
  其它类型（doc / bitable / mindnote / file / slides）会被跳过并计入 unsupported。

示例:
  # 导出整棵子树到当前目录
  feishu-cli wiki export-tree LHAswV4ahiqVM4kwtbZcHhvynth

  # 指定输出目录
  feishu-cli wiki export-tree LHAswV4ahiqVM4kwtbZcHhvynth -o ./backup

  # 通过 URL 导出
  feishu-cli wiki export-tree https://xxx.feishu.cn/wiki/LHAswV4ahiqVM4kwtbZcHhvynth -o ./backup

  # 限制深度（避免拉超大子树）
  feishu-cli wiki export-tree <token> -o ./backup --max-depth 3

  # 只导 docx，跳过 sheet
  feishu-cli wiki export-tree <token> -o ./backup --include-types docx

  # 增量同步：已存在的 md 跳过
  feishu-cli wiki export-tree <token> -o ./backup --skip-existing

  # 单文档失败时立即中断（默认 continue）
  feishu-cli wiki export-tree <token> -o ./backup --continue-on-error=false

  # 同时下载图片到 assets 目录
  feishu-cli wiki export-tree <token> -o ./backup --download-images --assets-dir ./backup/assets

  # 内嵌 Sheet 读取失败时保留 <sheet/> 引用
  feishu-cli wiki export-tree <token> -o ./backup --expand-sheets=false

  # 保留 @用户为可导入还原的标签
  feishu-cli wiki export-tree <token> -o ./backup --expand-mentions=false`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		rootToken, err := extractWikiToken(args[0])
		if err != nil {
			return err
		}

		outputDir, _ := cmd.Flags().GetString("output-dir")
		if outputDir == "" {
			outputDir = "./"
		}
		if err := validateOutputPath(outputDir, ""); err != nil {
			return fmt.Errorf("输出目录不安全: %w", err)
		}

		maxDepth, _ := cmd.Flags().GetInt("max-depth")
		includeTypes, _ := cmd.Flags().GetStringSlice("include-types")
		skipExisting, _ := cmd.Flags().GetBool("skip-existing")
		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		// 1. 取根节点信息（自动得到 SpaceID，无需用户传）
		fmt.Printf("正在解析根节点: %s\n", rootToken)
		root, err := client.GetWikiNode(rootToken, userAccessToken)
		if err != nil {
			return err
		}
		fmt.Printf("根节点标题: %s\n", root.Title)
		fmt.Printf("知识空间 ID: %s\n", root.SpaceID)

		// 2. 收集整棵树（决定每个节点的本地输出路径）
		fmt.Println("正在收集子树结构...")
		jobs, err := collectWikiTree(root, outputDir, maxDepth, userAccessToken)
		if err != nil {
			return fmt.Errorf("收集子树失败: %w", err)
		}
		fmt.Printf("共发现 %d 个节点，开始导出\n\n", len(jobs))

		// 3. 逐个导出
		stats := newTreeStats(len(jobs))
		for i, job := range jobs {
			progress := fmt.Sprintf("[%d/%d]", i+1, len(jobs))
			rel, _ := filepath.Rel(outputDir, job.OutputPath)
			if rel == "" {
				rel = job.OutputPath
			}

			// 类型过滤
			if !isExportableWikiType(job.Node.ObjType, includeTypes) {
				fmt.Printf("%s ⊘ %s  (skip: 不在 --include-types 列表，obj_type=%s)\n", progress, rel, job.Node.ObjType)
				stats.Unsupported++
				continue
			}

			// skip-existing
			if skipExisting && fileExistsAndNonEmpty(job.OutputPath) {
				fmt.Printf("%s ⏭  %s  (已存在，跳过)\n", progress, rel)
				stats.Skipped++
				continue
			}

			// 确保父目录存在
			if err := os.MkdirAll(filepath.Dir(job.OutputPath), 0700); err != nil {
				stats.Failed++
				stats.Failures = append(stats.Failures, treeFailure{
					NodeToken: job.Node.NodeToken,
					Title:     job.Node.Title,
					Path:      rel,
					Error:     fmt.Sprintf("创建目录失败: %v", err),
				})
				fmt.Printf("%s ✗ %s  (创建目录失败: %v)\n", progress, rel, err)
				if !continueOnError {
					return fmt.Errorf("创建目录失败: %w", err)
				}
				continue
			}

			assetsDirOverride := wikiTreeNodeAssetsDir(cmd, outputDir, job)
			markdown, err := exportWikiNodeMarkdown(job.Node, userAccessToken, cmd, assetsDirOverride)
			if err != nil {
				stats.Failed++
				stats.Failures = append(stats.Failures, treeFailure{
					NodeToken: job.Node.NodeToken,
					Title:     job.Node.Title,
					Path:      rel,
					Error:     err.Error(),
				})
				fmt.Printf("%s ✗ %s  (导出失败: %v)\n", progress, rel, err)
				if !continueOnError {
					return fmt.Errorf("%s 导出失败: %w", job.Node.Title, err)
				}
				continue
			}

			if err := os.WriteFile(job.OutputPath, []byte(markdown), 0600); err != nil {
				stats.Failed++
				stats.Failures = append(stats.Failures, treeFailure{
					NodeToken: job.Node.NodeToken,
					Title:     job.Node.Title,
					Path:      rel,
					Error:     fmt.Sprintf("写入文件失败: %v", err),
				})
				fmt.Printf("%s ✗ %s  (写入失败: %v)\n", progress, rel, err)
				if !continueOnError {
					return fmt.Errorf("写入文件失败: %w", err)
				}
				continue
			}

			stats.Success++
			fmt.Printf("%s ✓ %s\n", progress, rel)
		}

		// 4. 总结
		printTreeSummary(stats, outputDir)
		if stats.Failed > 0 {
			return fmt.Errorf("递归导出完成但有 %d 个节点失败", stats.Failed)
		}
		return nil
	},
}

// treeJob 一次「需要导出的节点 + 它的本地路径」的待办。
type treeJob struct {
	Node       *client.WikiNode
	OutputPath string
}

// treeFailure 描述一个失败的导出任务，用于最后汇总。
type treeFailure struct {
	NodeToken string `json:"node_token"`
	Title     string `json:"title"`
	Path      string `json:"path"`
	Error     string `json:"error"`
}

// treeStats 跟踪导出统计。
type treeStats struct {
	Total       int
	Success     int
	Skipped     int
	Unsupported int
	Failed      int
	Failures    []treeFailure
}

func newTreeStats(total int) *treeStats {
	return &treeStats{Total: total}
}

// collectWikiTree 以 root 为起点递归列出所有节点，决定每个节点的本地路径。
//
// 输出布局规则：
//   - 根节点：写到 outputDir/<sanitizedRootTitle>.md
//   - 有子节点的节点：自身写到 outputDir/.../<title>/<title>.md（即放在同名子目录中）
//   - 叶子节点：写到 outputDir/.../<title>.md
//
// 同父下出现重名子节点时，第 2 个及以后追加 _<token[:6]> 防止路径碰撞。
func collectWikiTree(root *client.WikiNode, outputDir string, maxDepth int, userAccessToken string) ([]treeJob, error) {
	var jobs []treeJob

	rootName := sanitizeWikiTitle(root.Title)
	if root.HasChild {
		// 根节点本身 + 它的子节点都挂在 outputDir 下，不为根再单独建一级目录
		// 这样 outputDir 直接就是「根节点对应的目录」，避免出现 ./out/<root>/<root>.md 这种深一层
		// 用户可以通过 -o 决定根目录的位置和名字。
		jobs = append(jobs, treeJob{Node: root, OutputPath: filepath.Join(outputDir, rootName+".md")})
		if err := walkChildren(root, outputDir, 1, maxDepth, userAccessToken, &jobs); err != nil {
			return nil, err
		}
	} else {
		jobs = append(jobs, treeJob{Node: root, OutputPath: filepath.Join(outputDir, rootName+".md")})
	}

	return jobs, nil
}

// walkChildren 递归处理 parent 节点的所有子节点，把它们的路径计算好后塞进 jobs。
func walkChildren(parent *client.WikiNode, parentDir string, depth, maxDepth int, userAccessToken string, jobs *[]treeJob) error {
	if maxDepth > 0 && depth > maxDepth {
		return nil
	}

	children, err := listAllWikiChildren(parent.SpaceID, parent.NodeToken, userAccessToken)
	if err != nil {
		return fmt.Errorf("列出 %q 的子节点失败: %w", parent.Title, err)
	}

	usedNames := make(map[string]bool)
	for _, child := range children {
		baseName := sanitizeWikiTitle(child.Title)
		uniqueName := dedupeSiblingName(baseName, child.NodeToken, usedNames)
		usedNames[uniqueName] = true

		if child.HasChild {
			mirrorDir := filepath.Join(parentDir, uniqueName)
			*jobs = append(*jobs, treeJob{
				Node:       child,
				OutputPath: filepath.Join(mirrorDir, uniqueName+".md"),
			})
			if err := walkChildren(child, mirrorDir, depth+1, maxDepth, userAccessToken, jobs); err != nil {
				return err
			}
		} else {
			*jobs = append(*jobs, treeJob{
				Node:       child,
				OutputPath: filepath.Join(parentDir, uniqueName+".md"),
			})
		}
	}
	return nil
}

// listAllWikiChildren 翻页列出某个父节点下的全部直接子节点。
func listAllWikiChildren(spaceID, parentToken, userAccessToken string) ([]*client.WikiNode, error) {
	var all []*client.WikiNode
	pageToken := ""
	seenPageTokens := make(map[string]bool)
	for {
		nodes, next, hasMore, err := client.ListWikiNodes(spaceID, parentToken, 50, pageToken, userAccessToken)
		if err != nil {
			return nil, err
		}
		all = append(all, nodes...)
		if !hasMore {
			break
		}
		if next == "" {
			return nil, fmt.Errorf("分页返回 has_more=true 但 page_token 为空")
		}
		if seenPageTokens[next] {
			return nil, fmt.Errorf("分页返回重复 page_token: %s", next)
		}
		seenPageTokens[next] = true
		pageToken = next
	}
	return all, nil
}

// sanitizeWikiTitle 把 wiki 节点标题转成可作为文件/目录名的安全字符串。
// 复用 cmd/utils.go 里的 safeOutputPath（只做字符替换 + 长度截断），
// 然后处理空标题 / 单点目录等极端情况。
func sanitizeWikiTitle(title string) string {
	cleaned := safeOutputPath(strings.TrimSpace(title), "")
	cleaned = strings.Trim(cleaned, ". ")
	if cleaned == "" {
		return "untitled"
	}
	return cleaned
}

// dedupeSiblingName 在同父目录下，如果 baseName 已被同级 sibling 用过，
// 追加 _<token[:6]> 后缀避免文件/目录碰撞。
func dedupeSiblingName(baseName, token string, used map[string]bool) string {
	if !used[baseName] {
		return baseName
	}
	suffix := token
	if len(suffix) > 6 {
		suffix = suffix[:6]
	}
	candidate := baseName + "_" + suffix
	// 万一连带 token 后缀都撞了（极小概率），再追加序号
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s_%s_%d", baseName, suffix, i)
	}
	return candidate
}

// isExportableWikiType 判断节点类型是否在用户指定的导出白名单内。
// 同时排除掉当前 wiki export 不支持转 Markdown 的类型（doc/bitable/mindnote/file/slides）。
func isExportableWikiType(objType string, allowed []string) bool {
	switch objType {
	case "docx", "sheet":
	default:
		return false
	}
	if len(allowed) == 0 {
		return true
	}
	for _, t := range allowed {
		if strings.EqualFold(strings.TrimSpace(t), objType) {
			return true
		}
	}
	return false
}

// fileExistsAndNonEmpty 判断指定路径是否已经是非空文件，用于 --skip-existing。
func fileExistsAndNonEmpty(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Size() > 0
}

// exportWikiNodeMarkdown 复用现有 export_wiki.go 里的转换逻辑，根据 obj_type 分发。
func exportWikiNodeMarkdown(node *client.WikiNode, userAccessToken string, cmd *cobra.Command, assetsDirOverride string) (string, error) {
	switch node.ObjType {
	case "docx":
		return exportDocxToMarkdownWithAssets(node.ObjToken, userAccessToken, cmd, assetsDirOverride)
	case "sheet":
		return exportSheetToMarkdown(node.ObjToken, node.Title, userAccessToken)
	default:
		return "", fmt.Errorf("不支持的节点类型: %s", node.ObjType)
	}
}

func wikiTreeNodeAssetsDir(cmd *cobra.Command, outputDir string, job treeJob) string {
	downloadImages, _ := cmd.Flags().GetBool("download-images")
	if !downloadImages || job.Node.ObjType != "docx" {
		return ""
	}

	baseAssetsDir, _ := cmd.Flags().GetString("assets-dir")
	rel, err := filepath.Rel(outputDir, job.OutputPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = filepath.Base(job.OutputPath)
	}
	stem := strings.TrimSuffix(rel, filepath.Ext(rel))
	return filepath.Join(baseAssetsDir, stem)
}

// printTreeSummary 在导出结束后打印总结，便于用户一眼看清结果。
func printTreeSummary(stats *treeStats, outputDir string) {
	fmt.Println()
	fmt.Println("=== 导出完成 ===")
	fmt.Printf("  输出目录: %s\n", outputDir)
	fmt.Printf("  总节点:   %d\n", stats.Total)
	fmt.Printf("  成功:     %d\n", stats.Success)
	if stats.Skipped > 0 {
		fmt.Printf("  跳过(已存在): %d\n", stats.Skipped)
	}
	if stats.Unsupported > 0 {
		fmt.Printf("  跳过(不支持): %d\n", stats.Unsupported)
	}
	if stats.Failed > 0 {
		fmt.Printf("  失败:     %d\n", stats.Failed)
		fmt.Println()
		fmt.Println("失败明细:")
		for _, f := range stats.Failures {
			fmt.Printf("  - [%s] %s\n    %s\n", f.NodeToken, f.Path, truncate(f.Error, 200))
		}
	}
}

// truncate 截断长字符串用于错误展示。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func init() {
	wikiCmd.AddCommand(exportWikiTreeCmd)
	exportWikiTreeCmd.Flags().StringP("output-dir", "o", "./", "输出根目录")
	exportWikiTreeCmd.Flags().Int("max-depth", 0, "最大递归深度（0 表示无限）")
	exportWikiTreeCmd.Flags().StringSlice("include-types", []string{"docx", "sheet"}, "要导出的 obj_type，逗号分隔（仅 docx/sheet 实际支持转 Markdown，其它类型即使列出也会被跳过）")
	exportWikiTreeCmd.Flags().Bool("download-images", false, "下载图片到本地目录（透传给底层 export）")
	exportWikiTreeCmd.Flags().String("assets-dir", "./assets", "图片下载目录（透传给底层 export）")
	exportWikiTreeCmd.Flags().Bool("expand-sheets", true, "展开内嵌电子表格为 Markdown 表格（false 时保留 <sheet/> 引用）")
	exportWikiTreeCmd.Flags().Bool("expand-mentions", true, "展开 @用户为友好格式（false 时保留 <mention-user/> 标签以支持导入还原）")
	exportWikiTreeCmd.Flags().Bool("skip-existing", false, "已存在且非空的 md 跳过（适合增量同步）")
	exportWikiTreeCmd.Flags().Bool("continue-on-error", true, "单个节点导出失败时是否继续后续节点")
	exportWikiTreeCmd.Flags().String("user-access-token", "", "User Access Token（可选；默认优先使用 auth login 登录态，失败时回退 App Token）")
}
