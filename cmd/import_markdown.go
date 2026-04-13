package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/converter"
	"github.com/spf13/cobra"
)

// printMu 保护并发 goroutine 的日志输出不交叉
var printMu sync.Mutex

// syncPrintf 线程安全的 Printf，用于并发阶段的日志输出
func syncPrintf(format string, a ...any) {
	printMu.Lock()
	defer printMu.Unlock()
	fmt.Printf(format, a...)
}

// segment 表示 Markdown 中的一个片段
type segment struct {
	kind    string // "markdown"、"mermaid"、"plantuml" 或 "equation"
	content string
}

// parseMarkdownSegments 将 Markdown 解析为片段，分离出 mermaid 和 plantuml 代码块
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

		// 检查块级公式 $$ 开始
		if trimmed == "$$" {
			// 先保存之前的普通内容
			if len(buf) > 0 {
				segments = append(segments, segment{kind: "markdown", content: strings.Join(buf, "\n")})
				buf = nil
			}

			// 收集公式内容
			i++
			var equationLines []string
			for i < len(lines) && strings.TrimSpace(lines[i]) != "$$" {
				equationLines = append(equationLines, lines[i])
				i++
			}
			// 跳过结束的 $$
			if i < len(lines) {
				i++
			}

			if len(equationLines) > 0 {
				segments = append(segments, segment{kind: "equation", content: strings.Join(equationLines, "\n")})
			}
			continue
		}

		// 不在围栏内：检查是否是图表代码块开始（恰好 3 个反引号 + mermaid/plantuml/puml）
		var diagramKind string
		if backticks == 3 {
			if strings.HasPrefix(trimmed, "```mermaid") {
				diagramKind = "mermaid"
			} else if strings.HasPrefix(trimmed, "```plantuml") || strings.HasPrefix(trimmed, "```puml") {
				diagramKind = "plantuml"
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
	if syntax == "plantuml" {
		return "PlantUML"
	}
	return "Mermaid"
}

// countDiagramBlocks 统计图表代码块数量（Mermaid + PlantUML）
// 使用与 parseMarkdownSegments 相同的嵌套围栏逻辑，避免将示例代码块中的图表标记误计
func countDiagramBlocks(markdown string) (mermaidCount, plantumlCount int) {
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

// importStats 记录导入统计信息
type importStats struct {
	mu              sync.Mutex
	totalBlocks     int
	diagramTotal    int
	diagramSuccess  int
	diagramFailed   int
	mermaidCount    int // Mermaid 图表数（用于分类统计）
	plantumlCount   int // PlantUML 图表数（用于分类统计）
	tableTotal      int
	tableSuccess    int
	tableFailed     int
	imageTotal      int
	imageSuccess    int
	imageFailed     int
	imageSkipped    int
	fallbackSuccess int
	fallbackFailed  int
	phase1Duration  time.Duration
	phase2Duration  time.Duration
	phase3Duration  time.Duration
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

		basePath := filepath.Dir(filePath)
		markdownText := string(content)

		// 统计图表数量
		mermaidCount, plantumlCount := countDiagramBlocks(markdownText)
		diagramCount := mermaidCount + plantumlCount
		if verbose && diagramCount > 0 {
			var parts []string
			if mermaidCount > 0 {
				parts = append(parts, fmt.Sprintf("%d 个 Mermaid", mermaidCount))
			}
			if plantumlCount > 0 {
				parts = append(parts, fmt.Sprintf("%d 个 PlantUML", plantumlCount))
			}
			fmt.Printf("[信息] 检测到 %s 图表\n", strings.Join(parts, ", "))
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

			doc, err := client.CreateDocument(title, folder)
			if err != nil {
				return fmt.Errorf("创建文档失败: %w", err)
			}
			if doc.DocumentId == nil {
				return fmt.Errorf("文档已创建但未返回ID")
			}
			documentID = *doc.DocumentId
			fmt.Printf("已创建文档: %s\n", documentID)
			fmt.Printf("链接: https://feishu.cn/docx/%s\n\n", documentID)
		}

		// 解析 Markdown 为片段
		segments := parseMarkdownSegments(markdownText)

		stats := &importStats{
			diagramTotal:  diagramCount,
			mermaidCount:  mermaidCount,
			plantumlCount: plantumlCount,
		}

		// === 阶段 1/3: 顺序创建文档块 ===
		fmt.Println("=== 阶段 1/3: 创建文档块 ===")
		phase1Start := time.Now()

		dTasks, tTasks, iTasks, err := phase1CreateBlocks(documentID, segments, uploadImages, basePath, stats, verbose)
		if err != nil {
			return err
		}

		stats.phase1Duration = time.Since(phase1Start)
		stats.tableTotal = len(tTasks)
		stats.imageTotal = stats.imageSkipped + len(iTasks)
		phase1Summary := fmt.Sprintf("[阶段1] 完成 (%.1fs), 块: %d, 待填表格: %d, 待导入图表: %d",
			stats.phase1Duration.Seconds(), stats.totalBlocks, len(tTasks), len(dTasks))
		if len(iTasks) > 0 {
			phase1Summary += fmt.Sprintf(", 待上传图片: %d", len(iTasks))
		}
		fmt.Println(phase1Summary + "\n")

		// === 阶段 2/3: 并发处理 ===
		if len(dTasks) > 0 || len(tTasks) > 0 || len(iTasks) > 0 {
			// 阶段 1 大量 API 调用后等待配额恢复，避免阶段 2 立即触发频率限制
			if stats.totalBlocks > 30 {
				cooldown := 5 * time.Second
				if verbose {
					fmt.Printf("等待 API 配额恢复 (%.0fs)...\n", cooldown.Seconds())
				}
				time.Sleep(cooldown)
			}
			phase2Header := fmt.Sprintf("=== 阶段 2/3: 并发处理 (图表×%d, 表格×%d", diagramWorkers, tableWorkers)
			if len(iTasks) > 0 {
				phase2Header += fmt.Sprintf(", 图片×%d", imageWorkers)
			}
			phase2Header += ") ==="
			fmt.Println(phase2Header)
			phase2Start := time.Now()

			failedDiagrams := phase2ConcurrentProcess(documentID, dTasks, tTasks, iTasks, diagramWorkers, tableWorkers, imageWorkers, diagramRetries, stats, verbose)

			stats.phase2Duration = time.Since(phase2Start)
			imageUploadTotal := stats.imageTotal - stats.imageSkipped
			var imageInfo string
			if imageUploadTotal > 0 {
				imageInfo = fmt.Sprintf(", 图片: %d/%d", stats.imageSuccess, imageUploadTotal)
			}
			fmt.Printf("[阶段2] 完成 (%.1fs), 图表: %d/%d, 表格: %d/%d%s\n\n",
				stats.phase2Duration.Seconds(),
				stats.diagramSuccess, stats.diagramTotal,
				stats.tableSuccess, stats.tableTotal,
				imageInfo)

			// === 阶段 3/3: 降级处理 ===
			if len(failedDiagrams) > 0 {
				fmt.Printf("=== 阶段 3/3: 降级处理 (%d 个) ===\n", len(failedDiagrams))
				phase3Start := time.Now()

				phase3HandleFallbacks(documentID, failedDiagrams, stats, verbose)

				stats.phase3Duration = time.Since(phase3Start)
				fmt.Printf("[阶段3] 完成 (%.1fs), 降级成功: %d/%d\n\n",
					stats.phase3Duration.Seconds(),
					stats.fallbackSuccess, stats.fallbackSuccess+stats.fallbackFailed)
			}
		}

		// === 输出结果 ===
		totalDuration := stats.phase1Duration + stats.phase2Duration + stats.phase3Duration

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			if err := printJSON(map[string]any{
				"document_id":      documentID,
				"blocks":           stats.totalBlocks,
				"diagram_total":    stats.diagramTotal,
				"diagram_success":  stats.diagramSuccess,
				"diagram_failed":   stats.diagramFailed,
				"mermaid_count":    stats.mermaidCount,
				"plantuml_count":   stats.plantumlCount,
				"diagram_fallback": stats.fallbackSuccess,
				"table_total":      stats.tableTotal,
				"table_success":    stats.tableSuccess,
				"table_failed":     stats.tableFailed,
				"image_total":      stats.imageTotal,
				"image_success":    stats.imageSuccess,
				"image_failed":     stats.imageFailed,
				"image_skipped":    stats.imageSkipped,
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
) ([]diagramTask, []tableTask, []imageTask, error) {
	var dTasks []diagramTask
	var tTasks []tableTask
	var iTasks []imageTask
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
				return nil, nil, nil, fmt.Errorf("转换 Markdown 失败 (段落 %d): %w", segIdx+1, err)
			}

			// 累加图片统计
			stats.imageSkipped += result.ImageStats.Skipped

			if len(result.BlockNodes) == 0 {
				continue
			}

			// 提取顶层块，记录带有嵌套子块的节点
			var topLevelBlocks []*larkdocx.Block
			nodeChildrenMap := map[int][]*converter.BlockNode{} // 顶层索引 → 嵌套子节点

			for i, node := range result.BlockNodes {
				topLevelBlocks = append(topLevelBlocks, node.Block)
				if len(node.Children) > 0 {
					nodeChildrenMap[i] = node.Children
				}
			}

			// 记录表格块和图片块的索引
			var tableIndices []int
			var imageIndices []int
			for i, block := range topLevelBlocks {
				if block.BlockType != nil {
					switch *block.BlockType {
					case int(converter.BlockTypeTable):
						tableIndices = append(tableIndices, i)
					case int(converter.BlockTypeImage):
						imageIndices = append(imageIndices, i)
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
					return client.CreateBlock(documentID, documentID, batch, -1)
				}, client.RetryConfig{
					MaxRetries:       5,
					RetryOnRateLimit: true,
				})
				if createResult.Err != nil {
					return nil, nil, nil, fmt.Errorf("添加内容失败 (段落 %d): %w", segIdx+1, createResult.Err)
				}
				stats.totalBlocks += len(createResult.Value)

				for _, block := range createResult.Value {
					if block.BlockId != nil {
						createdBlockIDs = append(createdBlockIDs, *block.BlockId)
					}
				}
			}

			// 递归创建嵌套子块（如嵌套列表）
			for idx, children := range nodeChildrenMap {
				if idx < len(createdBlockIDs) {
					nestedCount, nestedErr := createNestedChildren(documentID, createdBlockIDs[idx], children)
					if nestedErr != nil {
						if verbose {
							syncPrintf("  ⚠ 段落 %d 嵌套子块创建失败: %v\n", segIdx+1, nestedErr)
						}
					}
					stats.totalBlocks += nestedCount
				}
			}

			// QuoteContainer / Callout：遍历所有顶层节点清理飞书 API 异步生成的空子块。
			// 在所有 createNestedChildren 完成后执行，覆盖有子块和无子块的容器节点。
			for i, node := range result.BlockNodes {
				if i >= len(createdBlockIDs) {
					break
				}
				if node.Block.BlockType == nil {
					continue
				}
				switch *node.Block.BlockType {
				case int(converter.BlockTypeQuoteContainer):
					deleteContainerAutoEmptyBlock(documentID, createdBlockIDs[i], "QuoteContainer")
				case int(converter.BlockTypeCallout):
					deleteContainerAutoEmptyBlock(documentID, createdBlockIDs[i], "Callout")
				}
			}

			if verbose {
				fmt.Printf("  [段落 %d] 创建 %d 个块, %d 个表格\n", segIdx+1, len(createdBlockIDs), len(tableIndices))
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

			// 收集图片任务（不立即上传）
			imageSourceIdx := 0
			for _, imgIdx := range imageIndices {
				if imgIdx >= len(createdBlockIDs) || imageSourceIdx >= len(result.ImageSources) {
					continue
				}

				iTasks = append(iTasks, imageTask{
					index:        len(iTasks) + 1,
					imageBlockID: createdBlockIDs[imgIdx],
					source:       result.ImageSources[imageSourceIdx],
					basePath:     basePath,
				})
				imageSourceIdx++
			}

		} else if seg.kind == "equation" {
			// 块级公式：飞书 API 不支持创建 Equation 块（type=16），
			// 降级为包含行内 Equation 元素的 Text 块，保留公式语义
			textBlockType := 2 // BlockTypeText
			equationContent := seg.content
			equationBlocks := []*larkdocx.Block{
				{
					BlockType: &textBlockType,
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{
								Equation: &larkdocx.Equation{
									Content: &equationContent,
								},
							},
						},
					},
				},
			}

			createdBlocks, _, err := client.CreateBlock(documentID, documentID, equationBlocks, -1)
			if err != nil {
				if verbose {
					fmt.Printf("  ⚠ 公式块创建失败: %v\n", err)
				}
			} else {
				stats.totalBlocks += len(createdBlocks)
				if verbose {
					fmt.Printf("  [公式] 创建 %d 个块（行内公式）\n", len(createdBlocks))
				}
			}

		} else if seg.kind == "mermaid" || seg.kind == "plantuml" {
			diagramIdx++
			syntaxLabel := diagramSyntaxLabel(seg.kind)

			if verbose {
				fmt.Printf("  [%s %d] 创建画板占位块...\n", syntaxLabel, diagramIdx)
			}

			// 只创建画板占位块，不导入图表
			createResult := client.DoWithRetry(func() (*client.AddBoardResult, http.Header, error) {
				return client.AddBoard(documentID, "", -1)
			}, client.RetryConfig{
				MaxRetries:       5,
				RetryOnRateLimit: true,
				OnRetry: func(attempt int, err error, wait time.Duration) {
					if verbose {
						fmt.Printf("  ⚠ %s %d 创建画板重试 %d/5 (等待 %.1fs): %v\n",
							syntaxLabel, diagramIdx, attempt, wait.Seconds(), err)
					}
				},
			})
			if createResult.Err != nil {
				fmt.Printf("  ✗ %s %d 创建画板失败: %v\n", syntaxLabel, diagramIdx, createResult.Err)
				stats.diagramFailed++
				continue
			}
			boardResult := createResult.Value

			if boardResult.WhiteboardID == "" {
				fmt.Printf("  ✗ %s %d 未返回画板 ID\n", syntaxLabel, diagramIdx)
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
				fmt.Printf("  [%s %d] 画板已创建: %s\n", syntaxLabel, diagramIdx, boardResult.WhiteboardID)
			}
		}
	}

	return dTasks, tTasks, iTasks, nil
}

// phase2ConcurrentProcess 并发处理图表导入、表格填充和图片上传
func phase2ConcurrentProcess(
	documentID string,
	dTasks []diagramTask,
	tTasks []tableTask,
	iTasks []imageTask,
	diagramWorkers int,
	tableWorkers int,
	imageWorkers int,
	maxRetries int,
	stats *importStats,
	verbose bool,
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

			result := processDiagramTask(t, maxRetries, verbose)
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

			result := processTableTask(documentID, t, verbose)

			stats.mu.Lock()
			if result.success {
				stats.tableSuccess++
			} else {
				stats.tableFailed++
			}
			stats.mu.Unlock()
		}(task)
	}

	// 启动图片上传工作
	if len(iTasks) > 0 {
		imageSem := make(chan struct{}, imageWorkers)
		for _, task := range iTasks {
			wg.Add(1)
			go func(t imageTask) {
				defer wg.Done()
				imageSem <- struct{}{}
				defer func() { <-imageSem }()

				result := processImageTask(documentID, t, verbose)

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

// processDiagramTask 处理单个图表导入任务（Mermaid/PlantUML），带重试
func processDiagramTask(task diagramTask, maxRetries int, verbose bool) diagramResult {
	syntaxLabel := diagramSyntaxLabel(task.syntax)

	opts := client.ImportDiagramOptions{
		SourceType: "content",
		Syntax:     task.syntax,
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
func processTableTask(documentID string, task tableTask, verbose bool) tableResult {
	if verbose {
		syncPrintf("  [表格 %d] 填充 %d×%d...\n", task.index, task.tableData.Rows, task.tableData.Cols)
	}

	const maxRetries = 5

	result := client.DoVoidWithRetry(func() (http.Header, error) {
		// 获取表格单元格 ID
		cellIDs, err := client.GetTableCellIDs(documentID, task.tableBlockID)
		if err != nil {
			return nil, err
		}

		// 填充单元格内容（优先使用富文本元素以保留链接等样式）
		if len(task.tableData.CellElements) > 0 {
			return nil, client.FillTableCellsRich(documentID, cellIDs, task.tableData.CellElements, task.tableData.CellContents)
		}
		return nil, client.FillTableCells(documentID, cellIDs, task.tableData.CellContents)
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
func processImageTask(documentID string, task imageTask, verbose bool) imageResult {
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
		err := fmt.Errorf("图片超过 20MB 限制 (%d MB)", fi.Size()/(1024*1024))
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
		return client.UploadMediaWithExtra(localPath, "docx_image", task.imageBlockID, fileName, extra)
	}, retryCfg)

	if uploadResult.Err != nil {
		syncPrintf("  ✗ 图片 %d 上传失败 (%s): %v\n", task.index, task.source, uploadResult.Err)
		return imageResult{task: task, success: false, err: uploadResult.Err}
	}

	fileToken := uploadResult.Value

	// 步骤 3: 替换 Image Block 的 token
	replaceResult := client.DoVoidWithRetry(func() (http.Header, error) {
		return client.ReplaceImage(documentID, task.imageBlockID, fileToken)
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

func validateWorkerCount(flagName string, value int) error {
	if value <= 0 {
		return fmt.Errorf("--%s 必须大于 0，当前值: %d", flagName, value)
	}
	return nil
}

// resolveImageSource 解析图片来源为本地文件路径。
// 返回本地路径、上传文件名和清理函数（外部 URL 下载的临时文件需要清理）。
func resolveImageSource(source, basePath string) (string, string, func(), error) {
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
			ext = ".png"
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
func createNestedChildren(documentID string, parentBlockID string, children []*converter.BlockNode) (int, error) {
	if len(children) == 0 {
		return 0, nil
	}

	var childBlocks []*larkdocx.Block
	for _, c := range children {
		childBlocks = append(childBlocks, c.Block)
	}

	const batchSize = 50
	var createdBlockIDs []string
	totalCreated := 0

	for i := 0; i < len(childBlocks); i += batchSize {
		end := i + batchSize
		if end > len(childBlocks) {
			end = len(childBlocks)
		}
		batch := childBlocks[i:end]

		result := client.DoWithRetry(func() ([]*larkdocx.Block, http.Header, error) {
			return client.CreateBlock(documentID, parentBlockID, batch, -1)
		}, client.RetryConfig{
			MaxRetries:       5,
			RetryOnRateLimit: true,
		})
		if result.Err != nil {
			return totalCreated, fmt.Errorf("创建嵌套子块失败 (parent=%s): %w", parentBlockID, result.Err)
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
		if len(child.Children) > 0 {
			nestedCount, err := createNestedChildren(documentID, childID, child.Children)
			totalCreated += nestedCount
			if err != nil {
				return totalCreated, err
			}
		}
		// QuoteContainer / Callout 嵌套场景：无论是否有子块，均清理 API 自动生成的空块
		if child.Block.BlockType != nil {
			switch *child.Block.BlockType {
			case int(converter.BlockTypeQuoteContainer):
				deleteContainerAutoEmptyBlock(documentID, childID, "QuoteContainer")
			case int(converter.BlockTypeCallout):
				deleteContainerAutoEmptyBlock(documentID, childID, "Callout")
			}
		}
	}

	return totalCreated, nil
}

// phase3HandleFallbacks 处理失败的图表，降级为代码块
func phase3HandleFallbacks(
	documentID string,
	failedDiagrams []diagramResult,
	stats *importStats,
	verbose bool,
) {
	// 获取文档顶层子块列表
	children, err := client.GetAllBlockChildren(documentID, documentID)
	if err != nil {
		fmt.Printf("  ✗ 获取文档子块失败，无法降级: %v\n", err)
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
				fmt.Printf("  ⚠ %s %d 画板块未找到，跳过降级\n", syntaxLabel, r.task.index)
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
			fmt.Printf("  [降级] %s %d → 代码块 (位置 %d)\n", syntaxLabel, item.result.task.index, item.index)
		}

		// 1. 删除空画板块
		_, err := client.DeleteBlocks(documentID, documentID, item.index, item.index+1)
		if err != nil {
			fmt.Printf("  ✗ %s %d 删除画板失败: %v\n", syntaxLabel, item.result.task.index, err)
			stats.fallbackFailed++
			continue
		}

		// 2. 在同位置插入代码块
		codeBlock := createDiagramCodeBlock(item.result.task.syntax, item.result.task.content)
		_, _, err = client.CreateBlock(documentID, documentID, []*larkdocx.Block{codeBlock}, item.index)
		if err != nil {
			fmt.Printf("  ✗ %s %d 插入代码块失败: %v\n", syntaxLabel, item.result.task.index, err)
			stats.fallbackFailed++
			continue
		}

		stats.fallbackSuccess++
		if verbose {
			fmt.Printf("  ✓ %s %d 降级成功\n", syntaxLabel, item.result.task.index)
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
	// 向后兼容别名
	importMarkdownCmd.Flags().Int("mermaid-workers", 5, "图表并发导入数 (--diagram-workers 别名)")
	importMarkdownCmd.Flags().Int("mermaid-retries", 10, "图表最大重试次数 (--diagram-retries 别名)")
	_ = importMarkdownCmd.Flags().MarkHidden("mermaid-workers")
	_ = importMarkdownCmd.Flags().MarkHidden("mermaid-retries")
}
