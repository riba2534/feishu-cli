package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/converter"
	"github.com/spf13/cobra"
)

// printMu 保护并发 goroutine 的日志输出不交叉
var printMu sync.Mutex

// maxInlineVideoSize 视频通过 drive_media 接口直传的大小上限（字节）。
// 飞书该接口暂不支持分块上传，超过此值的视频会被拒绝；后续支持分块上传后可放宽。
const maxInlineVideoSize = 20 * 1024 * 1024

// syncPrintf 线程安全的 Printf，用于并发阶段的日志输出
func syncPrintf(format string, a ...any) {
	printMu.Lock()
	defer printMu.Unlock()
	fmt.Fprintf(os.Stderr, format, a...)
}

// segment 表示 Markdown 中的一个片段
type segment struct {
	kind    string // "markdown"、"mermaid"、"plantuml" 或 "svg"
	content string
}

// parseMarkdownSegments 将 Markdown 解析为片段，分离出 mermaid、plantuml 和 svg 代码块
// countLeadingBackticks 返回行首反引号数量（去除前导空格后）
func countLeadingBackticks(line string) int {
	trimmed := strings.TrimSpace(line)
	count := 0
	for _, ch := range trimmed {
		if ch == '`' {
			count++
		} else {
			break
		}
	}
	return count
}

func parseMarkdownSegments(markdown string) []segment {
	var segments []segment
	lines := strings.Split(markdown, "\n")
	var buf []string
	i := 0

	// 跟踪外层代码围栏状态，避免将嵌套代码围栏内的 ```mermaid 误识别
	inFence := false
	fenceBackticks := 0

	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		backticks := countLeadingBackticks(line)

		// 如果当前在外层代码围栏内，检查是否到达围栏结束
		if inFence {
			if backticks >= fenceBackticks && strings.TrimSpace(strings.TrimLeft(trimmed, "`")) == "" {
				// 围栏结束（只有反引号，没有其他内容）
				inFence = false
				fenceBackticks = 0
			}
			buf = append(buf, line)
			i++
			continue
		}

		// 不在围栏内：检查是否是图表代码块开始（恰好 3 个反引号 + mermaid/plantuml/puml/svg）
		var diagramKind string
		if backticks == 3 {
			if strings.HasPrefix(trimmed, "```mermaid") {
				diagramKind = "mermaid"
			} else if strings.HasPrefix(trimmed, "```plantuml") || strings.HasPrefix(trimmed, "```puml") {
				diagramKind = "plantuml"
			} else if strings.HasPrefix(trimmed, "```svg") {
				diagramKind = "svg"
			}
		}

		if diagramKind != "" {
			// 先保存之前的普通内容
			if len(buf) > 0 {
				segments = append(segments, segment{kind: "markdown", content: strings.Join(buf, "\n")})
				buf = nil
			}

			// 收集图表代码块内容
			i++
			var diagramLines []string
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				diagramLines = append(diagramLines, lines[i])
				i++
			}
			// 跳过结束的 ```
			if i < len(lines) {
				i++
			}

			if len(diagramLines) > 0 {
				segments = append(segments, segment{kind: diagramKind, content: strings.Join(diagramLines, "\n")})
			}
		} else {
			// 检查是否进入非图表代码围栏（4+ 反引号，或 3 反引号 + 非图表语言）
			if backticks >= 4 {
				inFence = true
				fenceBackticks = backticks
			} else if backticks == 3 && trimmed != "```" {
				// 3 反引号 + 语言标识（非图表），进入普通代码围栏
				inFence = true
				fenceBackticks = 3
			}
			buf = append(buf, line)
			i++
		}
	}

	// 保存剩余的普通内容
	if len(buf) > 0 {
		segments = append(segments, segment{kind: "markdown", content: strings.Join(buf, "\n")})
	}

	return segments
}

// diagramSyntaxLabel 返回图表语法的显示标签
func diagramSyntaxLabel(syntax string) string {
	switch syntax {
	case "plantuml":
		return "PlantUML"
	case "svg":
		return "SVG"
	default:
		return "Mermaid"
	}
}

// countDiagramBlocks 统计图表代码块数量（Mermaid + PlantUML + SVG）
// 使用与 parseMarkdownSegments 相同的嵌套围栏逻辑，避免将示例代码块中的图表标记误计
func countDiagramBlocks(markdown string) (mermaidCount, plantumlCount, svgCount int) {
	lines := strings.Split(markdown, "\n")
	inFence := false
	fenceBackticks := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		backticks := countLeadingBackticks(line)

		if inFence {
			if backticks >= fenceBackticks && strings.TrimSpace(strings.TrimLeft(trimmed, "`")) == "" {
				inFence = false
				fenceBackticks = 0
			}
			continue
		}

		if backticks == 3 {
			if strings.HasPrefix(trimmed, "```mermaid") {
				mermaidCount++
			} else if strings.HasPrefix(trimmed, "```plantuml") || strings.HasPrefix(trimmed, "```puml") {
				plantumlCount++
			} else if strings.HasPrefix(trimmed, "```svg") {
				svgCount++
			} else if trimmed != "```" {
				// 非图表的 3 反引号代码块
				inFence = true
				fenceBackticks = 3
			}
		} else if backticks >= 4 {
			inFence = true
			fenceBackticks = backticks
		}
	}
	return
}

// --- 三阶段流水线数据结构 ---

// diagramTask 表示一个待导入的图表任务（Mermaid 或 PlantUML）
type diagramTask struct {
	index        int    // 序号 (1-based)
	content      string // 图表源码
	syntax       string // "mermaid" 或 "plantuml"
	boardBlockID string // 画板块 ID
	whiteboardID string // 画板 token
}

// diagramResult 表示图表导入的结果
type diagramResult struct {
	task    diagramTask
	success bool
	err     error
	retries int
}

// tableTask 表示一个待填充的表格任务
type tableTask struct {
	index        int // 序号 (1-based)
	tableBlockID string
	tableData    *converter.TableData
}

// tableResult 表示表格填充的结果
type tableResult struct {
	task    tableTask
	success bool
	err     error
}

// imageTask 表示一个待上传的图片任务
type imageTask struct {
	index        int    // 序号 (1-based)
	imageBlockID string // Image Block ID
	source       string // 图片来源（本地路径或 URL）
	basePath     string // Markdown 文件所在目录，用于解析相对路径
}

// imageResult 表示图片上传的结果
type imageResult struct {
	task    imageTask
	success bool
	err     error
}

// videoTask 表示一个待上传的视频任务（底层使用 File Block）
type videoTask struct {
	index       int
	fileBlockID string
	source      string
	basePath    string
}

// videoResult 表示视频上传结果
type videoResult struct {
	task    videoTask
	success bool
	err     error
}

// importStats 记录导入统计信息
type importStats struct {
	mu              sync.Mutex
	progress        io.Writer
	totalBlocks     int
	diagramTotal    int
	diagramSuccess  int
	diagramFailed   int
	mermaidCount    int // Mermaid 图表数（用于分类统计）
	plantumlCount   int // PlantUML 图表数（用于分类统计）
	svgCount        int // SVG 图表数（用于分类统计）
	tableTotal      int
	tableSuccess    int
	tableFailed     int
	imageTotal      int
	imageSuccess    int
	imageFailed     int
	imageSkipped    int
	videoTotal      int
	videoSuccess    int
	videoFailed     int
	videoSkipped    int
	fallbackSuccess int
	fallbackFailed  int
	phase1Duration  time.Duration
	phase2Duration  time.Duration
	phase3Duration  time.Duration
}

func (s *importStats) progressf(format string, a ...any) {
	w := s.progress
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w, format, a...)
}

var importMarkdownCmd = &cobra.Command{
	Use:   "import <file.md>",
	Short: "从 Markdown 导入创建/更新文档",
	Long: `从 Markdown 文件导入内容，创建新的飞书文档或更新已有文档。

特性:
  - 三阶段流水线: 顺序创建 → 并发处理 → 降级容错
  - Mermaid/PlantUML 图表自动转换为飞书画板 (重试+失败降级为代码块)
  - 表格并发填充，大表格自动拆分
  - 详细进度和耗时统计

示例:
  feishu-cli doc import doc.md --title "我的文档"
  feishu-cli doc import doc.md --document-id ABC123def456
  feishu-cli doc import doc.md --title "我的文档" --verbose
  feishu-cli doc import doc.md --title "测试" --diagram-workers 5 --table-workers 8`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		filePath := args[0]
		title, _ := cmd.Flags().GetString("title")
		documentID, _ := cmd.Flags().GetString("document-id")
		uploadImages, _ := cmd.Flags().GetBool("upload-images")
		folder, _ := cmd.Flags().GetString("folder")
		verbose, _ := cmd.Flags().GetBool("verbose")
		diagramWorkers, _ := cmd.Flags().GetInt("diagram-workers")
		tableWorkers, _ := cmd.Flags().GetInt("table-workers")
		imageWorkers, _ := cmd.Flags().GetInt("image-workers")
		diagramRetries, _ := cmd.Flags().GetInt("diagram-retries")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)
		var progressOut io.Writer = os.Stdout
		if output == "json" {
			progressOut = cmd.ErrOrStderr()
		}

		// 向后兼容: 如果用户使用了旧的 --mermaid-workers/--mermaid-retries，覆盖新值
		if cmd.Flags().Changed("mermaid-workers") {
			diagramWorkers, _ = cmd.Flags().GetInt("mermaid-workers")
		}
		if cmd.Flags().Changed("mermaid-retries") {
			diagramRetries, _ = cmd.Flags().GetInt("mermaid-retries")
		}
		if err := validateWorkerCount("diagram-workers", diagramWorkers); err != nil {
			return err
		}
		if err := validateWorkerCount("table-workers", tableWorkers); err != nil {
			return err
		}
		if err := validateWorkerCount("image-workers", imageWorkers); err != nil {
			return err
		}

		// 检查文件大小限制（100MB）
		const maxFileSize = 100 * 1024 * 1024
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("获取文件信息失败: %w", err)
		}
		if fileInfo.Size() > maxFileSize {
			return fmt.Errorf("文件超过最大限制 %d MB", maxFileSize/(1024*1024))
		}

		// Read markdown file
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("读取文件失败: %w", err)
		}
		if err := validateMarkdownEncoding(content); err != nil {
			return err
		}

		basePath := filepath.Dir(filePath)
		markdownText := string(content)

		// 统计图表数量
		mermaidCount, plantumlCount, svgCount := countDiagramBlocks(markdownText)
		diagramCount := mermaidCount + plantumlCount + svgCount
		if verbose && diagramCount > 0 {
			var parts []string
			if mermaidCount > 0 {
				parts = append(parts, fmt.Sprintf("%d 个 Mermaid", mermaidCount))
			}
			if plantumlCount > 0 {
				parts = append(parts, fmt.Sprintf("%d 个 PlantUML", plantumlCount))
			}
			if svgCount > 0 {
				parts = append(parts, fmt.Sprintf("%d 个 SVG", svgCount))
			}
			fmt.Fprintf(progressOut, "[信息] 检测到 %s 图表\n", strings.Join(parts, ", "))
		}

		// If no document ID, create new document
		if documentID == "" {
			if title == "" {
				// Use filename as title
				title = filepath.Base(filePath)
				ext := filepath.Ext(title)
				if len(ext) < len(title) {
					title = title[:len(title)-len(ext)]
				}
				if title == "" {
					title = "无标题文档"
				}
			}

			doc, err := client.CreateDocument(title, folder, userAccessToken)
			if err != nil {
				return fmt.Errorf("创建文档失败: %w", err)
			}
			if doc.DocumentId == nil {
				return fmt.Errorf("文档已创建但未返回ID")
			}
			documentID = *doc.DocumentId
			fmt.Fprintf(progressOut, "已创建文档: %s\n", documentID)
			fmt.Fprintf(progressOut, "链接: https://feishu.cn/docx/%s\n\n", documentID)
		}

		// 解析 Markdown 为片段
		segments := parseMarkdownSegments(markdownText)

		stats := &importStats{
			diagramTotal:  diagramCount,
			mermaidCount:  mermaidCount,
			plantumlCount: plantumlCount,
			svgCount:      svgCount,
			progress:      progressOut,
		}

		// === 阶段 1/3: 顺序创建文档块 ===
		fmt.Fprintln(progressOut, "=== 阶段 1/3: 创建文档块 ===")
		phase1Start := time.Now()

		dTasks, tTasks, iTasks, vTasks, err := phase1CreateBlocks(documentID, segments, uploadImages, basePath, stats, verbose, userAccessToken)
		if err != nil {
			return err
		}

		stats.phase1Duration = time.Since(phase1Start)
		stats.tableTotal = len(tTasks)
		stats.imageTotal = stats.imageSkipped + len(iTasks)
		stats.videoTotal = stats.videoSkipped + len(vTasks)
		phase1Summary := fmt.Sprintf("[阶段1] 完成 (%.1fs), 块: %d, 待填表格: %d, 待导入图表: %d",
			stats.phase1Duration.Seconds(), stats.totalBlocks, len(tTasks), len(dTasks))
		if len(iTasks) > 0 {
			phase1Summary += fmt.Sprintf(", 待上传图片: %d", len(iTasks))
		}
		if len(vTasks) > 0 {
			phase1Summary += fmt.Sprintf(", 待上传视频: %d", len(vTasks))
		}
		fmt.Fprintln(progressOut, phase1Summary+"\n")

		// === 阶段 2/3: 并发处理 ===
		if len(dTasks) > 0 || len(tTasks) > 0 || len(iTasks) > 0 || len(vTasks) > 0 {
			// 阶段 1 大量 API 调用后等待配额恢复，避免阶段 2 立即触发频率限制
			if stats.totalBlocks > 30 {
				cooldown := 5 * time.Second
				if verbose {
					fmt.Fprintf(progressOut, "等待 API 配额恢复 (%.0fs)...\n", cooldown.Seconds())
				}
				time.Sleep(cooldown)
			}
			phase2Header := fmt.Sprintf("=== 阶段 2/3: 并发处理 (图表×%d, 表格×%d", diagramWorkers, tableWorkers)
			if len(iTasks) > 0 && len(vTasks) > 0 {
				phase2Header += fmt.Sprintf(", 图片+视频×%d", imageWorkers)
			} else if len(iTasks) > 0 {
				phase2Header += fmt.Sprintf(", 图片×%d", imageWorkers)
			} else if len(vTasks) > 0 {
				phase2Header += fmt.Sprintf(", 视频×%d", imageWorkers)
			}
			phase2Header += ") ==="
			fmt.Fprintln(progressOut, phase2Header)
			phase2Start := time.Now()

			failedDiagrams := phase2ConcurrentProcess(documentID, dTasks, tTasks, iTasks, vTasks, diagramWorkers, tableWorkers, imageWorkers, diagramRetries, stats, verbose, userAccessToken)

			stats.phase2Duration = time.Since(phase2Start)
			imageUploadTotal := stats.imageTotal - stats.imageSkipped
			videoUploadTotal := stats.videoTotal - stats.videoSkipped
			var mediaInfo string
			if imageUploadTotal > 0 {
				mediaInfo = fmt.Sprintf(", 图片: %d/%d", stats.imageSuccess, imageUploadTotal)
			}
			if videoUploadTotal > 0 {
				mediaInfo += fmt.Sprintf(", 视频: %d/%d", stats.videoSuccess, videoUploadTotal)
			}
			fmt.Fprintf(progressOut, "[阶段2] 完成 (%.1fs), 图表: %d/%d, 表格: %d/%d%s\n\n",
				stats.phase2Duration.Seconds(),
				stats.diagramSuccess, stats.diagramTotal,
				stats.tableSuccess, stats.tableTotal,
				mediaInfo)

			// === 阶段 3/3: 降级处理 ===
			if len(failedDiagrams) > 0 {
				fmt.Fprintf(progressOut, "=== 阶段 3/3: 降级处理 (%d 个) ===\n", len(failedDiagrams))
				phase3Start := time.Now()

				phase3HandleFallbacks(documentID, failedDiagrams, stats, verbose, userAccessToken)

				stats.phase3Duration = time.Since(phase3Start)
				fmt.Fprintf(progressOut, "[阶段3] 完成 (%.1fs), 降级成功: %d/%d\n\n",
					stats.phase3Duration.Seconds(),
					stats.fallbackSuccess, stats.fallbackSuccess+stats.fallbackFailed)
			}
		}

		// === 输出结果 ===
		totalDuration := stats.phase1Duration + stats.phase2Duration + stats.phase3Duration

		if output == "json" {
			if err := printJSON(map[string]any{
				"document_id":      documentID,
				"blocks":           stats.totalBlocks,
				"diagram_total":    stats.diagramTotal,
				"diagram_success":  stats.diagramSuccess,
				"diagram_failed":   stats.diagramFailed,
				"mermaid_count":    stats.mermaidCount,
				"plantuml_count":   stats.plantumlCount,
				"svg_count":        stats.svgCount,
				"diagram_fallback": stats.fallbackSuccess,
				"table_total":      stats.tableTotal,
				"table_success":    stats.tableSuccess,
				"table_failed":     stats.tableFailed,
				"image_total":      stats.imageTotal,
				"image_success":    stats.imageSuccess,
				"image_failed":     stats.imageFailed,
				"image_skipped":    stats.imageSkipped,
				"video_total":      stats.videoTotal,
				"video_success":    stats.videoSuccess,
				"video_failed":     stats.videoFailed,
				"video_skipped":    stats.videoSkipped,
				"duration_seconds": totalDuration.Seconds(),
				"phase1_seconds":   stats.phase1Duration.Seconds(),
				"phase2_seconds":   stats.phase2Duration.Seconds(),
				"phase3_seconds":   stats.phase3Duration.Seconds(),
			}); err != nil {
				return err
			}
		} else {
			fmt.Println("导入完成!")
			fmt.Printf("  文档ID: %s\n", documentID)
			fmt.Printf("  添加块数: %d\n", stats.totalBlocks)
			if stats.imageTotal > 0 {
				if stats.imageSkipped == stats.imageTotal {
					fmt.Printf("  图片: %d 张 (已创建占位块，feishu:// 引用需手动上传)\n", stats.imageSkipped)
				} else if stats.imageFailed > 0 {
					fmt.Printf("  图片: %d/%d 成功 (%d 跳过, %d 失败)\n",
						stats.imageSuccess, stats.imageTotal, stats.imageSkipped, stats.imageFailed)
				} else if stats.imageSkipped > 0 {
					fmt.Printf("  图片: %d/%d 成功 (%d 跳过)\n",
						stats.imageSuccess, stats.imageTotal, stats.imageSkipped)
				} else {
					fmt.Printf("  图片: %d/%d 成功\n", stats.imageSuccess, stats.imageTotal)
				}
			}
			if stats.videoTotal > 0 {
				if stats.videoSkipped == stats.videoTotal {
					fmt.Printf("  视频: %d 个 (已创建占位块，资源需手动处理)\n", stats.videoSkipped)
				} else if stats.videoFailed > 0 {
					fmt.Printf("  视频: %d/%d 成功 (%d 跳过, %d 失败)\n",
						stats.videoSuccess, stats.videoTotal, stats.videoSkipped, stats.videoFailed)
				} else if stats.videoSkipped > 0 {
					fmt.Printf("  视频: %d/%d 成功 (%d 跳过)\n",
						stats.videoSuccess, stats.videoTotal, stats.videoSkipped)
				} else {
					fmt.Printf("  视频: %d/%d 成功\n", stats.videoSuccess, stats.videoTotal)
				}
			}
			if stats.tableTotal > 0 {
				fmt.Printf("  表格: %d/%d 成功\n", stats.tableSuccess, stats.tableTotal)
			}
			if stats.diagramTotal > 0 {
				var diagramDetail string
				if stats.mermaidCount > 0 && stats.plantumlCount > 0 {
					diagramDetail = fmt.Sprintf(" (Mermaid: %d, PlantUML: %d)", stats.mermaidCount, stats.plantumlCount)
				}
				if stats.fallbackSuccess > 0 {
					fmt.Printf("  图表: %d/%d 成功%s (%d 降级为代码块)\n",
						stats.diagramSuccess, stats.diagramTotal, diagramDetail, stats.fallbackSuccess)
				} else {
					fmt.Printf("  图表: %d/%d 成功%s\n", stats.diagramSuccess, stats.diagramTotal, diagramDetail)
				}
			}
			fmt.Printf("  总耗时: %.1fs\n", totalDuration.Seconds())
			fmt.Printf("  链接: https://feishu.cn/docx/%s\n", documentID)
		}

		return nil
	},
}

// phase1CreateBlocks 顺序创建所有文档块，收集待处理的图表、表格和图片任务
func phase1CreateBlocks(
	documentID string,
	segments []segment,
	uploadImages bool,
	basePath string,
	stats *importStats,
	verbose bool,
	userAccessToken string,
) ([]diagramTask, []tableTask, []imageTask, []videoTask, error) {
	var dTasks []diagramTask
	var tTasks []tableTask
	var iTasks []imageTask
	var vTasks []videoTask
	diagramIdx := 0

	for segIdx, seg := range segments {
		if seg.kind == "markdown" {
			if strings.TrimSpace(seg.content) == "" {
				continue
			}

			options := converter.ConvertOptions{
				UploadImages: uploadImages,
				DocumentID:   documentID,
			}

			conv := converter.NewMarkdownToBlock([]byte(seg.content), options, basePath)
			result, err := conv.ConvertWithTableData()
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("转换 Markdown 失败 (段落 %d): %w", segIdx+1, err)
			}

			// 累加图片统计
			stats.imageSkipped += result.ImageStats.Skipped
			stats.videoSkipped += result.VideoStats.Skipped

			if len(result.BlockNodes) == 0 {
				continue
			}

			// 提取顶层块，嵌套子块稍后按 BlockNode 树顺序创建
			var topLevelBlocks []*larkdocx.Block

			for _, node := range result.BlockNodes {
				topLevelBlocks = append(topLevelBlocks, node.Block)
			}

			// 记录表格块的索引
			var tableIndices []int
			for i, block := range topLevelBlocks {
				if block.BlockType != nil {
					switch *block.BlockType {
					case int(converter.BlockTypeTable):
						tableIndices = append(tableIndices, i)
					}
				}
			}

			// 批量添加顶层块（飞书 API 限制每次最多 50 个块）
			const batchSize = 50
			var createdBlockIDs []string
			for i := 0; i < len(topLevelBlocks); i += batchSize {
				end := i + batchSize
				if end > len(topLevelBlocks) {
					end = len(topLevelBlocks)
				}
				batch := topLevelBlocks[i:end]

				createResult := client.DoWithRetry(func() ([]*larkdocx.Block, http.Header, error) {
					return client.CreateBlock(documentID, documentID, batch, -1, userAccessToken)
				}, client.RetryConfig{
					MaxRetries:       5,
					RetryOnRateLimit: true,
				})
				if createResult.Err != nil {
					return nil, nil, nil, nil, fmt.Errorf("添加内容失败 (段落 %d): %w", segIdx+1, createResult.Err)
				}
				stats.totalBlocks += len(createResult.Value)

				for _, block := range createResult.Value {
					if block.BlockId != nil {
						createdBlockIDs = append(createdBlockIDs, *block.BlockId)
					}
				}
			}

			nestedCreatedByTop := map[int][]createdBlockNode{}

			// 递归创建嵌套子块（如嵌套列表）
			for idx, node := range result.BlockNodes {
				if idx >= len(createdBlockIDs) || len(node.Children) == 0 {
					continue
				}
				parentID := createdBlockIDs[idx]

				nestedCount, nestedCreated, nestedErr := createNestedChildren(documentID, parentID, node.Children, userAccessToken)
				if nestedErr != nil {
					if verbose {
						syncPrintf("  ⚠ 段落 %d 嵌套子块创建失败: %v\n", segIdx+1, nestedErr)
					}
				}
				stats.totalBlocks += nestedCount
				nestedCreatedByTop[idx] = nestedCreated
			}

			// QuoteContainer / Callout：遍历所有顶层节点清理飞书 API 异步生成的空子块。
			// 在所有 createNestedChildren 完成后执行，覆盖有子块和无子块的容器节点。
			for i, node := range result.BlockNodes {
				if i >= len(createdBlockIDs) {
					break
				}
				if node.Block.BlockType != nil {
					deleteContainerAutoEmptyBlock(documentID, createdBlockIDs[i], *node.Block.BlockType, userAccessToken)
				}
			}

			if verbose {
				stats.progressf("  [段落 %d] 创建 %d 个块, %d 个表格\n", segIdx+1, len(createdBlockIDs), len(tableIndices))
			}

			// 收集表格任务（不立即填充）
			tableDataIdx := 0
			for _, tableIdx := range tableIndices {
				if tableIdx >= len(createdBlockIDs) || tableDataIdx >= len(result.TableDatas) {
					continue
				}

				tTasks = append(tTasks, tableTask{
					index:        len(tTasks) + 1,
					tableBlockID: createdBlockIDs[tableIdx],
					tableData:    result.TableDatas[tableDataIdx],
				})
				tableDataIdx++
			}

			iTasks = appendImageTasks(iTasks, result.BlockNodes, createdBlockIDs, nestedCreatedByTop, result.ImageSources, basePath)
			vTasks = appendVideoTasks(vTasks, result.BlockNodes, createdBlockIDs, nestedCreatedByTop, result.VideoSources, basePath)

		} else if seg.kind == "mermaid" || seg.kind == "plantuml" || seg.kind == "svg" {
			diagramIdx++
			syntaxLabel := diagramSyntaxLabel(seg.kind)

			if verbose {
				stats.progressf("  [%s %d] 创建画板占位块...\n", syntaxLabel, diagramIdx)
			}

			// 只创建画板占位块，不导入图表
			createResult := client.DoWithRetry(func() (*client.AddBoardResult, http.Header, error) {
				return client.AddBoard(documentID, "", -1, userAccessToken)
			}, client.RetryConfig{
				MaxRetries:       5,
				RetryOnRateLimit: true,
				OnRetry: func(attempt int, err error, wait time.Duration) {
					if verbose {
						stats.progressf("  ⚠ %s %d 创建画板重试 %d/5 (等待 %.1fs): %v\n",
							syntaxLabel, diagramIdx, attempt, wait.Seconds(), err)
					}
				},
			})
			if createResult.Err != nil {
				stats.progressf("  ✗ %s %d 创建画板失败: %v\n", syntaxLabel, diagramIdx, createResult.Err)
				stats.diagramFailed++
				continue
			}
			boardResult := createResult.Value

			if boardResult.WhiteboardID == "" {
				stats.progressf("  ✗ %s %d 未返回画板 ID\n", syntaxLabel, diagramIdx)
				stats.diagramFailed++
				continue
			}

			stats.totalBlocks++

			dTasks = append(dTasks, diagramTask{
				index:        diagramIdx,
				content:      seg.content,
				syntax:       seg.kind,
				boardBlockID: boardResult.BlockID,
				whiteboardID: boardResult.WhiteboardID,
			})

			if verbose {
				stats.progressf("  [%s %d] 画板已创建: %s\n", syntaxLabel, diagramIdx, boardResult.WhiteboardID)
			}
		}
	}

	return dTasks, tTasks, iTasks, vTasks, nil
}

// phase2ConcurrentProcess 并发处理图表导入、表格填充和图片上传
func phase2ConcurrentProcess(
	documentID string,
	dTasks []diagramTask,
	tTasks []tableTask,
	iTasks []imageTask,
	vTasks []videoTask,
	diagramWorkers int,
	tableWorkers int,
	imageWorkers int,
	maxRetries int,
	stats *importStats,
	verbose bool,
	userAccessToken string,
) []diagramResult {
	var wg sync.WaitGroup
	diagramResults := make([]diagramResult, len(dTasks))

	// 图表信号量
	diagramSem := make(chan struct{}, diagramWorkers)
	// 表格信号量
	tableSem := make(chan struct{}, tableWorkers)

	// 启动图表工作
	for i, task := range dTasks {
		wg.Add(1)
		go func(idx int, t diagramTask) {
			defer wg.Done()
			diagramSem <- struct{}{}
			defer func() { <-diagramSem }()

			result := processDiagramTask(t, maxRetries, verbose, userAccessToken)
			diagramResults[idx] = result

			stats.mu.Lock()
			if result.success {
				stats.diagramSuccess++
			} else {
				stats.diagramFailed++
			}
			stats.mu.Unlock()
		}(i, task)
	}

	// 启动表格工作
	for _, task := range tTasks {
		wg.Add(1)
		go func(t tableTask) {
			defer wg.Done()
			tableSem <- struct{}{}
			defer func() { <-tableSem }()

			result := processTableTask(documentID, t, verbose, userAccessToken)

			stats.mu.Lock()
			if result.success {
				stats.tableSuccess++
			} else {
				stats.tableFailed++
			}
			stats.mu.Unlock()
		}(task)
	}

	// 图片和视频共用一个媒体上传信号量（飞书 drive_media 上传 API 限制约 5 QPS，
	// 必须把图片和视频放进同一个并发池，否则总并发会变为 imageWorkers*2，超出 QPS）
	var mediaSem chan struct{}
	if len(iTasks) > 0 || len(vTasks) > 0 {
		mediaSem = make(chan struct{}, imageWorkers)
	}

	// 启动图片上传工作
	if len(iTasks) > 0 {
		for _, task := range iTasks {
			wg.Add(1)
			go func(t imageTask) {
				defer wg.Done()
				mediaSem <- struct{}{}
				defer func() { <-mediaSem }()

				result := processImageTask(documentID, t, verbose, userAccessToken)

				stats.mu.Lock()
				if result.success {
					stats.imageSuccess++
				} else {
					stats.imageFailed++
				}
				stats.mu.Unlock()
			}(task)
		}
	}

	if len(vTasks) > 0 {
		for _, task := range vTasks {
			wg.Add(1)
			go func(t videoTask) {
				defer wg.Done()
				mediaSem <- struct{}{}
				defer func() { <-mediaSem }()

				result := processVideoTask(documentID, t, verbose, userAccessToken)

				stats.mu.Lock()
				if result.success {
					stats.videoSuccess++
				} else {
					stats.videoFailed++
				}
				stats.mu.Unlock()
			}(task)
		}
	}

	wg.Wait()

	// 收集失败的图表任务
	var failedDiagrams []diagramResult
	for _, r := range diagramResults {
		if !r.success {
			failedDiagrams = append(failedDiagrams, r)
		}
	}

	return failedDiagrams
}

// processDiagramTask 处理单个图表导入任务（Mermaid/PlantUML/SVG），带重试
func processDiagramTask(task diagramTask, maxRetries int, verbose bool, userAccessToken string) diagramResult {
	syntaxLabel := diagramSyntaxLabel(task.syntax)

	// SVG 走 create_nodes 透传，不走 plantuml endpoint
	if task.syntax == "svg" {
		nodeJSON, buildErr := buildSVGNodeJSON(task.content)
		if buildErr != nil {
			syncPrintf("  ✗ %s %d 解析失败: %v\n", syntaxLabel, task.index, buildErr)
			return diagramResult{task: task, success: false, err: buildErr}
		}
		_, createErr := client.CreateBoardNodes(task.whiteboardID, nodeJSON, client.CreateBoardNotesOptions{
			UserAccessToken: userAccessToken,
		})
		if createErr != nil {
			syncPrintf("  ✗ %s %d 创建节点失败: %v\n", syntaxLabel, task.index, createErr)
			return diagramResult{task: task, success: false, err: createErr}
		}
		if verbose {
			syncPrintf("  ✓ %s %d 成功\n", syntaxLabel, task.index)
		}
		return diagramResult{task: task, success: true}
	}

	opts := client.ImportDiagramOptions{
		SourceType:      "content",
		Syntax:          task.syntax,
		UserAccessToken: userAccessToken,
	}

	result := client.DoWithRetry(func() (*client.ImportDiagramResult, http.Header, error) {
		return client.ImportDiagram(task.whiteboardID, task.content, opts)
	}, client.RetryConfig{
		MaxRetries:       maxRetries,
		MaxTotalAttempts: maxRetries + 5,
		RetryOnRateLimit: true,
		IsPermanent:      client.IsPermanentError,
		OnRetry: func(attempt int, err error, wait time.Duration) {
			if verbose {
				syncPrintf("  ⚠ %s %d 重试 %d/%d (等待 %.1fs): %v\n",
					syntaxLabel, task.index, attempt, maxRetries, wait.Seconds(), err)
			}
		},
	})

	retries := result.Attempts - 1
	if result.Err == nil {
		if verbose {
			if retries > 0 {
				syncPrintf("  ✓ %s %d 成功 (重试 %d 次)\n", syntaxLabel, task.index, retries)
			} else {
				syncPrintf("  ✓ %s %d 成功\n", syntaxLabel, task.index)
			}
		}
		return diagramResult{task: task, success: true, retries: retries}
	}

	if client.IsPermanentError(result.Err) {
		syncPrintf("  ✗ %s %d 语法错误 (不重试): %v\n", syntaxLabel, task.index, result.Err)
	} else {
		syncPrintf("  ✗ %s %d 失败 (重试%d次): %v\n", syntaxLabel, task.index, retries, result.Err)
	}
	return diagramResult{task: task, success: false, err: result.Err, retries: retries}
}

// processTableTask 处理单个表格填充任务（带重试）
func processTableTask(documentID string, task tableTask, verbose bool, userAccessToken string) tableResult {
	extraRowCount := len(task.tableData.ExtraRowContents)
	if verbose {
		if extraRowCount > 0 {
			syncPrintf("  [表格 %d] 填充 %d×%d（+%d 追加行）...\n",
				task.index, task.tableData.Rows, task.tableData.Cols, extraRowCount)
		} else {
			syncPrintf("  [表格 %d] 填充 %d×%d...\n", task.index, task.tableData.Rows, task.tableData.Cols)
		}
	}

	const maxRetries = 5

	// 追加行数 ≥ 5 时每 5 行打一次进度，防止用户误判卡死
	var onProgress client.InsertRowProgressFunc
	if verbose {
		onProgress = tableAppendProgress(extraRowCount, 5, 5, func(appended, total int) {
			syncPrintf("    [表格 %d] 追加行 %d/%d\n", task.index, appended, total)
		})
	}

	result := client.DoVoidWithRetry(func() (http.Header, error) {
		return nil, fillTableWithExtraRows(documentID, task.tableBlockID, task.tableData, userAccessToken, onProgress)
	}, client.RetryConfig{
		MaxRetries:       maxRetries,
		RetryOnRateLimit: true,
		OnRetry: func(attempt int, err error, wait time.Duration) {
			if verbose {
				syncPrintf("  ⚠ 表格 %d 重试 %d/%d (等待 %.1fs): %v\n",
					task.index, attempt, maxRetries, wait.Seconds(), err)
			}
		},
	})

	if result.Err != nil {
		if verbose {
			syncPrintf("  ✗ 表格 %d 失败: %v\n", task.index, result.Err)
		}
		return tableResult{task: task, success: false, err: result.Err}
	}

	if verbose {
		syncPrintf("  ✓ 表格 %d 成功\n", task.index)
	}
	return tableResult{task: task, success: true}
}

// processImageTask 处理单个图片上传任务（三步法）
func processImageTask(documentID string, task imageTask, verbose bool, userAccessToken string) imageResult {
	const maxRetries = 3

	// 解析图片来源
	localPath, fileName, cleanup, err := resolveImageSource(task.source, task.basePath)
	if err != nil {
		syncPrintf("  ✗ 图片 %d 解析失败 (%s): %v\n", task.index, task.source, err)
		return imageResult{task: task, success: false, err: err}
	}
	defer cleanup()

	// 检查文件大小（≤ 20MB）
	const maxImageSize = 20 * 1024 * 1024
	fi, err := os.Stat(localPath)
	if err != nil {
		syncPrintf("  ✗ 图片 %d 文件信息获取失败: %v\n", task.index, err)
		return imageResult{task: task, success: false, err: err}
	}
	if fi.Size() > maxImageSize {
		err := fmt.Errorf("图片超过 20MB 限制 (%.1f MB)", float64(fi.Size())/(1024*1024))
		syncPrintf("  ✗ 图片 %d: %v\n", task.index, err)
		return imageResult{task: task, success: false, err: err}
	}

	extra := fmt.Sprintf(`{"drive_route_token":"%s"}`, documentID)

	// 步骤 2: 上传图片到 Image Block
	retryCfg := client.RetryConfig{
		MaxRetries:       maxRetries,
		MaxTotalAttempts: maxRetries + 3,
		RetryOnRateLimit: true,
		OnRetry: func(attempt int, err error, wait time.Duration) {
			if verbose {
				syncPrintf("  ⚠ 图片 %d 上传重试 %d/%d (等待 %.1fs): %v\n",
					task.index, attempt, maxRetries, wait.Seconds(), err)
			}
		},
	}

	uploadResult := client.DoWithRetry(func() (string, http.Header, error) {
		return client.UploadMediaWithExtra(localPath, "docx_image", task.imageBlockID, fileName, extra, userAccessToken)
	}, retryCfg)

	if uploadResult.Err != nil {
		syncPrintf("  ✗ 图片 %d 上传失败 (%s): %v\n", task.index, task.source, uploadResult.Err)
		return imageResult{task: task, success: false, err: uploadResult.Err}
	}

	fileToken := uploadResult.Value

	// 步骤 3: 替换 Image Block 的 token
	replaceResult := client.DoVoidWithRetry(func() (http.Header, error) {
		return client.ReplaceImage(documentID, task.imageBlockID, fileToken, userAccessToken)
	}, retryCfg)

	if replaceResult.Err != nil {
		syncPrintf("  ✗ 图片 %d 替换失败 (token=%s): %v\n", task.index, fileToken, replaceResult.Err)
		return imageResult{task: task, success: false, err: replaceResult.Err}
	}

	if verbose {
		syncPrintf("  ✓ 图片 %d 成功 (%s)\n", task.index, task.source)
	}
	return imageResult{task: task, success: true}
}

// processVideoTask 处理单个视频上传任务（File Block）
func processVideoTask(documentID string, task videoTask, verbose bool, userAccessToken string) videoResult {
	const maxRetries = 5

	failWith := func(reason string, err error) videoResult {
		// 视频上传失败：把阶段 1 创建的空 File 块替换为可见占位 Text 块，避免文档里留孤儿
		fileName := pathpkg.Base(task.source)
		if fileName == "" || fileName == "." || fileName == "/" {
			fileName = task.source
		}
		replaceFailedVideoBlock(documentID, task, fileName, reason, userAccessToken)
		return videoResult{task: task, success: false, err: err}
	}

	localPath, fileName, cleanup, err := resolveMediaSource(task.source, task.basePath, ".mp4")
	if err != nil {
		syncPrintf("  ✗ 视频 %d 解析失败 (%s): %v\n", task.index, task.source, err)
		return failWith(fmt.Sprintf("解析失败: %v", err), err)
	}
	defer cleanup()

	fi, err := os.Stat(localPath)
	if err != nil {
		syncPrintf("  ✗ 视频 %d 文件信息获取失败: %v\n", task.index, err)
		return failWith(fmt.Sprintf("文件信息获取失败: %v", err), err)
	}
	if fi.Size() > maxInlineVideoSize {
		sizeMB := float64(fi.Size()) / (1024 * 1024)
		err := fmt.Errorf("视频超过 %.1f MB 限制 (当前 %.1f MB)，当前上传通道暂不支持大文件分块上传",
			float64(maxInlineVideoSize)/(1024*1024), sizeMB)
		syncPrintf("  ✗ 视频 %d: %v\n", task.index, err)
		return failWith(fmt.Sprintf("超过 %.1f MB 限制", float64(maxInlineVideoSize)/(1024*1024)), err)
	}

	extra := fmt.Sprintf(`{"drive_route_token":"%s"}`, documentID)
	retryCfg := client.RetryConfig{
		MaxRetries:       maxRetries,
		MaxTotalAttempts: maxRetries + 3,
		RetryOnRateLimit: true,
		OnRetry: func(attempt int, err error, wait time.Duration) {
			if verbose {
				syncPrintf("  ⚠ 视频 %d 上传重试 %d/%d (等待 %.1fs): %v\n",
					task.index, attempt, maxRetries, wait.Seconds(), err)
			}
		},
	}

	uploadResult := client.DoWithRetry(func() (string, http.Header, error) {
		return client.UploadMediaWithExtra(localPath, "docx_file", task.fileBlockID, fileName, extra, userAccessToken)
	}, retryCfg)
	if uploadResult.Err != nil {
		syncPrintf("  ✗ 视频 %d 上传失败 (%s): %v\n", task.index, task.source, uploadResult.Err)
		return failWith(fmt.Sprintf("上传失败: %v", uploadResult.Err), uploadResult.Err)
	}

	fileToken := uploadResult.Value
	replaceResult := client.DoVoidWithRetry(func() (http.Header, error) {
		return client.UpdateBlock(documentID, task.fileBlockID, map[string]any{
			"replace_file": map[string]any{"token": fileToken},
		}, userAccessToken)
	}, retryCfg)
	if replaceResult.Err != nil {
		syncPrintf("  ✗ 视频 %d 绑定失败 (token=%s): %v\n", task.index, fileToken, replaceResult.Err)
		return failWith(fmt.Sprintf("绑定失败: %v", replaceResult.Err), replaceResult.Err)
	}

	if verbose {
		syncPrintf("  ✓ 视频 %d 成功 (%s)\n", task.index, task.source)
	}
	return videoResult{task: task, success: true}
}

// replaceFailedVideoBlock 将上传失败的视频 File 块替换为可见的占位 Text 块，
// 避免阶段 1 创建的空 File 块在文档中变成孤儿（用户看不到任何内容）。
// 由于飞书 PatchBlock 不支持跨类型变更，这里采用"删除 + 同位置插入 Text"的策略，
// 模式与 phase3HandleFallbacks 一致。占位失败仅记日志，不让整体导入崩溃。
func replaceFailedVideoBlock(documentID string, task videoTask, fileName, reason, userAccessToken string) {
	placeholder := fmt.Sprintf("[视频上传失败：%s (%s)]", fileName, reason)

	// 1. 在文档顶层子块中找到该 File 块的索引
	children, err := client.GetAllBlockChildren(documentID, documentID, userAccessToken)
	if err != nil {
		syncPrintf("  ⚠ 视频 %d 占位块创建失败（无法获取子块列表）: %v\n", task.index, err)
		return
	}
	idx := -1
	for i, child := range children {
		if child.BlockId != nil && *child.BlockId == task.fileBlockID {
			idx = i
			break
		}
	}
	if idx < 0 {
		syncPrintf("  ⚠ 视频 %d 占位块创建跳过（File 块未在顶层找到，可能位于嵌套容器内）\n", task.index)
		return
	}

	// 2. 删除空 File 块
	if _, err := client.DeleteBlocks(documentID, documentID, idx, idx+1, userAccessToken); err != nil {
		syncPrintf("  ⚠ 视频 %d 占位块创建失败（删除原 File 块失败）: %v\n", task.index, err)
		return
	}

	// 3. 在原位置插入 Text 占位块
	textBlockType := 2 // Text block
	textBlock := &larkdocx.Block{
		BlockType: &textBlockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: &placeholder}},
			},
		},
	}
	if _, _, err := client.CreateBlock(documentID, documentID, []*larkdocx.Block{textBlock}, idx, userAccessToken); err != nil {
		syncPrintf("  ⚠ 视频 %d 占位块创建失败（插入 Text 块失败）: %v\n", task.index, err)
		return
	}
}

func validateWorkerCount(flagName string, value int) error {
	if value <= 0 {
		return fmt.Errorf("--%s 必须大于 0，当前值: %d", flagName, value)
	}
	return nil
}

func validateMarkdownEncoding(content []byte) error {
	if !utf8.Valid(content) {
		return fmt.Errorf("Markdown 文件不是合法 UTF-8 编码，请先转换为 UTF-8 后再导入")
	}
	return nil
}

// resolveImageSource 解析图片来源为本地文件路径。
// 返回本地路径、上传文件名和清理函数（外部 URL 下载的临时文件需要清理）。
func resolveImageSource(source, basePath string) (string, string, func(), error) {
	return resolveMediaSource(source, basePath, ".png")
}

func resolveMediaSource(source, basePath, defaultExt string) (string, string, func(), error) {
	noop := func() {}

	// HTTP(S) URL → 下载到临时文件
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		parsedURL, err := url.Parse(source)
		if err != nil {
			return "", "", nil, fmt.Errorf("解析图片 URL 失败: %w", err)
		}

		fileName := pathpkg.Base(parsedURL.Path)
		if fileName == "." || fileName == "/" || fileName == "" {
			fileName = "image"
		}

		ext := filepath.Ext(fileName)
		if ext == "" || len(ext) > 10 {
			ext = defaultExt
		}
		tmpFile, err := os.CreateTemp("", "feishu-img-*"+ext)
		if err != nil {
			return "", "", nil, fmt.Errorf("创建临时文件失败: %w", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		if err := client.DownloadFromURL(source, tmpPath); err != nil {
			os.Remove(tmpPath)
			return "", "", nil, fmt.Errorf("下载图片失败: %w", err)
		}
		if filepath.Ext(fileName) == "" {
			fileName += ext
		}
		return tmpPath, fileName, func() { os.Remove(tmpPath) }, nil
	}

	// 本地文件路径：相对路径基于 Markdown 文件所在目录解析
	localPath := source
	if !filepath.IsAbs(localPath) {
		localPath = filepath.Join(basePath, localPath)
	}

	if _, err := os.Stat(localPath); err != nil {
		return "", "", nil, fmt.Errorf("图片文件不存在: %s", localPath)
	}
	return localPath, filepath.Base(localPath), noop, nil
}

// createNestedChildren 递归创建嵌套子块（如嵌套列表的父子关系）
// 返回创建的块总数和可能的错误
func createNestedChildren(documentID string, parentBlockID string, children []*converter.BlockNode, userAccessToken string) (int, []createdBlockNode, error) {
	if len(children) == 0 {
		return 0, nil, nil
	}

	var childBlocks []*larkdocx.Block
	for _, c := range children {
		childBlocks = append(childBlocks, c.Block)
	}

	const batchSize = 50
	var createdBlockIDs []string
	totalCreated := 0
	var createdNodes []createdBlockNode

	for i := 0; i < len(childBlocks); i += batchSize {
		end := i + batchSize
		if end > len(childBlocks) {
			end = len(childBlocks)
		}
		batch := childBlocks[i:end]

		result := client.DoWithRetry(func() ([]*larkdocx.Block, http.Header, error) {
			return client.CreateBlock(documentID, parentBlockID, batch, -1, userAccessToken)
		}, client.RetryConfig{
			MaxRetries:       5,
			RetryOnRateLimit: true,
		})
		if result.Err != nil {
			return totalCreated, createdNodes, fmt.Errorf("创建嵌套子块失败 (parent=%s): %w", parentBlockID, result.Err)
		}
		totalCreated += len(result.Value)

		for _, block := range result.Value {
			if block.BlockId != nil {
				createdBlockIDs = append(createdBlockIDs, *block.BlockId)
			}
		}
	}

	// 递归创建更深层的子块
	for i, child := range children {
		if i >= len(createdBlockIDs) {
			continue
		}
		childID := createdBlockIDs[i]
		createdNodes = append(createdNodes, createdBlockNode{node: child, blockID: childID})
		if len(child.Children) > 0 {
			nestedCount, nestedCreated, err := createNestedChildren(documentID, childID, child.Children, userAccessToken)
			totalCreated += nestedCount
			createdNodes = append(createdNodes, nestedCreated...)
			if err != nil {
				return totalCreated, createdNodes, err
			}
		}
		// QuoteContainer / Callout 嵌套场景：无论是否有子块，均清理 API 自动生成的空块
		if child.Block.BlockType != nil {
			deleteContainerAutoEmptyBlock(documentID, childID, *child.Block.BlockType, userAccessToken)
		}
	}

	return totalCreated, createdNodes, nil
}

type createdBlockNode struct {
	node    *converter.BlockNode
	blockID string
}

func appendImageTasks(
	tasks []imageTask,
	topNodes []*converter.BlockNode,
	topBlockIDs []string,
	nestedCreatedByTop map[int][]createdBlockNode,
	imageSources []string,
	basePath string,
) []imageTask {
	sourceIdx := 0

	appendIfImage := func(node *converter.BlockNode, blockID string) {
		if sourceIdx >= len(imageSources) || !isUploadImageBlockNode(node) {
			return
		}
		tasks = append(tasks, imageTask{
			index:        len(tasks) + 1,
			imageBlockID: blockID,
			source:       imageSources[sourceIdx],
			basePath:     basePath,
		})
		sourceIdx++
	}

	for i, node := range topNodes {
		if i >= len(topBlockIDs) {
			break
		}
		appendIfImage(node, topBlockIDs[i])
		for _, nested := range nestedCreatedByTop[i] {
			appendIfImage(nested.node, nested.blockID)
		}
	}

	return tasks
}

func isUploadImageBlockNode(node *converter.BlockNode) bool {
	if node == nil || node.Block == nil || node.Block.BlockType == nil ||
		*node.Block.BlockType != int(converter.BlockTypeImage) {
		return false
	}
	if node.Block.Image == nil || node.Block.Image.Token == nil {
		return true
	}
	return *node.Block.Image.Token == ""
}

func appendVideoTasks(
	tasks []videoTask,
	topNodes []*converter.BlockNode,
	topBlockIDs []string,
	nestedCreatedByTop map[int][]createdBlockNode,
	videoSources []string,
	basePath string,
) []videoTask {
	sourceIdx := 0

	appendIfVideo := func(node *converter.BlockNode, blockID string) {
		if sourceIdx >= len(videoSources) || !isVideoBlockNode(node) {
			return
		}
		tasks = append(tasks, videoTask{
			index:       len(tasks) + 1,
			fileBlockID: blockID,
			source:      videoSources[sourceIdx],
			basePath:    basePath,
		})
		sourceIdx++
	}

	for i, node := range topNodes {
		if i >= len(topBlockIDs) {
			break
		}
		appendIfVideo(node, topBlockIDs[i])
		for _, nested := range nestedCreatedByTop[i] {
			appendIfVideo(nested.node, nested.blockID)
		}
	}

	return tasks
}

func isVideoBlockNode(node *converter.BlockNode) bool {
	if node == nil || node.Block == nil || node.Block.BlockType == nil {
		return false
	}
	if *node.Block.BlockType != int(converter.BlockTypeFile) || node.Block.File == nil || node.Block.File.Name == nil {
		return false
	}
	return converter.IsVideoFilename(*node.Block.File.Name)
}

// phase3HandleFallbacks 处理失败的图表，降级为代码块
func phase3HandleFallbacks(
	documentID string,
	failedDiagrams []diagramResult,
	stats *importStats,
	verbose bool,
	userAccessToken string,
) {
	// 获取文档顶层子块列表
	children, err := client.GetAllBlockChildren(documentID, documentID, userAccessToken)
	if err != nil {
		stats.progressf("  ✗ 获取文档子块失败，无法降级: %v\n", err)
		stats.fallbackFailed += len(failedDiagrams)
		return
	}

	// 构建 blockID → index 映射
	blockIDToIndex := make(map[string]int)
	for i, child := range children {
		if child.BlockId != nil {
			blockIDToIndex[*child.BlockId] = i
		}
	}

	// 按 index 降序排序失败列表（避免删除时索引偏移）
	type fallbackItem struct {
		result diagramResult
		index  int // 在文档中的索引
	}
	var items []fallbackItem
	for _, r := range failedDiagrams {
		if idx, ok := blockIDToIndex[r.task.boardBlockID]; ok {
			items = append(items, fallbackItem{result: r, index: idx})
		} else {
			syntaxLabel := diagramSyntaxLabel(r.task.syntax)
			if verbose {
				stats.progressf("  ⚠ %s %d 画板块未找到，跳过降级\n", syntaxLabel, r.task.index)
			}
			stats.fallbackFailed++
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].index > items[j].index // 降序
	})

	for _, item := range items {
		syntaxLabel := diagramSyntaxLabel(item.result.task.syntax)
		if verbose {
			stats.progressf("  [降级] %s %d → 代码块 (位置 %d)\n", syntaxLabel, item.result.task.index, item.index)
		}

		// 1. 删除空画板块
		_, err := client.DeleteBlocks(documentID, documentID, item.index, item.index+1, userAccessToken)
		if err != nil {
			stats.progressf("  ✗ %s %d 删除画板失败: %v\n", syntaxLabel, item.result.task.index, err)
			stats.fallbackFailed++
			continue
		}

		// 2. 在同位置插入代码块
		codeBlock := createDiagramCodeBlock(item.result.task.syntax, item.result.task.content)
		_, _, err = client.CreateBlock(documentID, documentID, []*larkdocx.Block{codeBlock}, item.index, userAccessToken)
		if err != nil {
			stats.progressf("  ✗ %s %d 插入代码块失败: %v\n", syntaxLabel, item.result.task.index, err)
			stats.fallbackFailed++
			continue
		}

		stats.fallbackSuccess++
		if verbose {
			stats.progressf("  ✓ %s %d 降级成功\n", syntaxLabel, item.result.task.index)
		}
	}
}

// createDiagramCodeBlock 创建图表代码块（用于降级）
func createDiagramCodeBlock(syntax, content string) *larkdocx.Block {
	blockType := 14 // Code block
	// Mermaid/PlantUML 没有对应的飞书语言代码，使用 plaintext(1)
	langCode := 1
	// 在代码块内容前加上语法标识注释，方便用户识别
	labeledContent := fmt.Sprintf("// %s diagram\n%s", syntax, content)
	return &larkdocx.Block{
		BlockType: &blockType,
		Code: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: &labeledContent,
					},
				},
			},
			Style: &larkdocx.TextStyle{
				Language: &langCode,
			},
		},
	}
}

func init() {
	docCmd.AddCommand(importMarkdownCmd)
	importMarkdownCmd.Flags().StringP("title", "t", "", "文档标题 (用于新建文档)")
	importMarkdownCmd.Flags().StringP("document-id", "d", "", "已有文档ID (用于更新)")
	importMarkdownCmd.Flags().Bool("upload-images", true, "上传本地图片")
	importMarkdownCmd.Flags().StringP("folder", "f", "", "新文档的文件夹 Token")
	importMarkdownCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	importMarkdownCmd.Flags().BoolP("verbose", "v", false, "显示详细进度")
	importMarkdownCmd.Flags().Int("diagram-workers", 5, "图表 (Mermaid/PlantUML) 并发导入数")
	importMarkdownCmd.Flags().Int("table-workers", 3, "表格并发填充数")
	importMarkdownCmd.Flags().Int("image-workers", 2, "图片并发上传数 (API 限制 5 QPS)")
	importMarkdownCmd.Flags().Int("diagram-retries", 10, "图表最大重试次数")
	importMarkdownCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文档）")
	// 向后兼容别名
	importMarkdownCmd.Flags().Int("mermaid-workers", 5, "图表并发导入数 (--diagram-workers 别名)")
	importMarkdownCmd.Flags().Int("mermaid-retries", 10, "图表最大重试次数 (--diagram-retries 别名)")
	_ = importMarkdownCmd.Flags().MarkHidden("mermaid-workers")
	_ = importMarkdownCmd.Flags().MarkHidden("mermaid-retries")
}
