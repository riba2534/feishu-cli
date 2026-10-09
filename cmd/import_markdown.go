package cmd

import (
	"errors"
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
	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/converter"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/spf13/cobra"
)

// printMu 保护并发 goroutine 的日志输出不交叉
var printMu sync.Mutex

// maxInlineVideoSize 视频走 medias/upload_all 单次直传的大小上限（字节）。
// 超过此值的视频改走 upload_prepare / upload_part / upload_finish 分片上传（client.UploadDocMedia），
// 不再拒绝（此前注释称"飞书该接口不支持分块上传"有误，官方 doc_media_upload 即用分片上传大素材）。
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
	cellMap      map[string]string // 文档级 cellID->textBlockID 映射，多个 worker 共享只读
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
	parentID     string // Image Block 的父块（失败占位用；空表示文档根）
	source       string // 图片来源（本地路径、URL 或 feishu://media/<token>）
	basePath     string // Markdown 文件所在目录，用于解析相对路径
	label        string // 消息前缀，空默认"图片"；单元格路径为"单元格图片"（编号空间不同，避免误标）
	width        int    // 显示宽度（<image width>），0 表示按原图像素
	height       int    // 显示高度
	align        int    // 对齐（1 左 2 中 3 右），0 表示默认
	reuseToken   string // 非空表示复用已有素材 token（失败时把空块替换为占位文本）
}

// kindLabel 返回进度/告警消息里的图片类别前缀。
func (t imageTask) kindLabel() string {
	if t.label != "" {
		return t.label
	}
	return "图片"
}

// imageResult 表示图片上传的结果
type imageResult struct {
	task    imageTask
	success bool
	err     error
}

// videoTask 表示一个待上传的附件/视频任务（底层使用 File Block，服务端外包一层 View 块）
type videoTask struct {
	index       int
	fileBlockID string // File 块 ID（素材上传到这里）
	viewBlockID string // 外层 View 块 ID（失败占位时删除它；空时退回 fileBlockID）
	parentID    string // View 块的父块（空表示文档根）
	source      string // 本地路径、URL 或 feishu://media/<token>
	basePath    string
	name        string // 上传文件名（空时取来源文件名）
	video       bool   // 是否为视频（统计口径：video_* / file_*）
	reuseToken  string // 非空表示复用已有素材 token
}

// kindLabel 返回附件任务的类别名。
func (t videoTask) kindLabel() string {
	if t.video {
		return "视频"
	}
	return "附件"
}

// failureKind 返回 failures 中的 kind。
func (t videoTask) failureKind() string {
	if t.video {
		return "video"
	}
	return "file"
}

// videoResult 表示视频上传结果
type videoResult struct {
	task    videoTask
	success bool
	err     error
}

// importStats 记录导入统计信息
type importStats struct {
	mu               sync.Mutex
	progress         io.Writer
	totalBlocks      int
	diagramTotal     int
	diagramSuccess   int
	diagramFailed    int
	mermaidCount     int // Mermaid 图表数（用于分类统计）
	plantumlCount    int // PlantUML 图表数（用于分类统计）
	svgCount         int // SVG 图表数（用于分类统计）
	tableTotal       int
	tableSuccess     int
	tableFailed      int
	imageTotal       int
	imageSuccess     int
	imageFailed      int
	imageSkipped     int
	videoTotal       int
	videoSuccess     int
	videoFailed      int
	videoSkipped     int
	fileTotal        int // 附件（<file token> 复用）
	fileSuccess      int
	fileFailed       int
	fileSkipped      int
	boardCopyTotal   int // 带 token 的画板（复制源画板节点）
	boardCopySuccess int
	boardCopyFailed  int
	boardCopyAsImage int // 无法复制为可编辑画板、已降级为源画板图片
	blocksFailed     int // 建块被拒而跳过的块数（阶段一隔离）
	aborted          bool
	cellImageTotal   int // 表格单元格内待嵌入图片总数（issue #164，阶段 2.5）
	cellImageSuccess int
	cellImageFailed  int
	fallbackSuccess  int
	fallbackFailed   int
	phase1Duration   time.Duration
	phase2Duration   time.Duration
	phase3Duration   time.Duration
	failures         []importFailure // 最终未能写入的内容明细（降级成功的图表不算）
}

// importFailure 一项导入失败明细，JSON 输出在 failures 数组中。
type importFailure struct {
	Kind   string `json:"kind"`             // blocks / image / table / video / file / whiteboard / sheet / bitable / cell_image / diagram / nested_blocks
	Index  int    `json:"index"`            // 该类内容的序号（1 起）；嵌套子块为段落序号
	Source string `json:"source,omitempty"` // 图片/视频来源等
	Error  string `json:"error"`
}

// addFailure 记录一项失败明细（自带加锁，调用方不要持有 stats.mu）。
func (s *importStats) addFailure(kind string, index int, source string, err error) {
	msg := "未知错误"
	if err != nil {
		msg = err.Error()
	}
	s.mu.Lock()
	s.failures = append(s.failures, importFailure{Kind: kind, Index: index, Source: source, Error: msg})
	s.mu.Unlock()
}

// importFailureKindLabel 失败类型的中文名。
func importFailureKindLabel(kind string) string {
	switch kind {
	case "image":
		return "图片"
	case "table":
		return "表格"
	case "video":
		return "视频"
	case "cell_image":
		return "单元格图片"
	case "diagram":
		return "图表"
	case "nested_blocks":
		return "嵌套子块"
	case "blocks":
		return "内容块"
	case "file":
		return "附件"
	case "whiteboard":
		return "画板"
	case "sheet":
		return "电子表格"
	case "bitable":
		return "多维表格"
	}
	return kind
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
  - doc export 的 <image token>/<file token>/<video src="feishu://media/..."> 复用原素材
    （下载后重新上传），<whiteboard token> 复制源画板节点；做不到时降级为占位文本并计入 failures
  - 被服务端拒绝的单个块会被隔离跳过（计入 failures），不再让整篇导入中止
  - 详细进度和耗时统计

部分内容失败时仍输出文档链接与统计（-o json 含 partial_failure / failures），并以退出码 1 结束。

新建文档且以 Bot 身份执行时，创建后自动给当前 CLI 登录用户授予 full_access，
JSON 输出 permission_grant；导入到已有文档（--document-id）或 User 身份时不触发。

示例:
  feishu-cli doc import doc.md --title "我的文档"
  feishu-cli doc import doc.md --document-id ABC123def456
  feishu-cli doc import doc.md --document-id https://xxx.feishu.cn/wiki/wikcnXXXXXX
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
		colWidthRaw, _ := cmd.Flags().GetString("table-column-width")
		colWidthMode, colWidthValues, err := parseTableColumnWidthFlag(colWidthRaw)
		if err != nil {
			return err
		}
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

		// 检查文件大小限制（100MB）；敏感目录、不存在、是目录、无权限读取均为用法错误（退出码 2）
		const maxFileSize = 100 * 1024 * 1024
		fileInfo, err := safefile.StatInputFile(filePath)
		if err != nil {
			return err
		}
		if fileInfo.Size() > maxFileSize {
			return clierr.Usagef("文件超过最大限制 %d MB", maxFileSize/(1024*1024))
		}

		// Read markdown file
		content, err := safefile.ReadInputFile(filePath)
		if err != nil {
			return err
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

		// 新建文档且以 Bot 身份执行时，创建后立即给当前用户授予 full_access（导入中途失败也能打开文档排查）
		var grant *client.PermissionGrantResult

		// --document-id 接受裸 ID、/docx/ URL 与 /wiki/ URL（wiki 自动解包为底层 docx）
		if strings.TrimSpace(documentID) != "" {
			documentID, err = resolveDocxArg(documentID, "--document-id", userAccessToken)
			if err != nil {
				return err
			}
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
			fmt.Fprintf(progressOut, "链接: %s\n\n", client.BuildResourceURL(client.ResourceTypeDocx, documentID))
			grant = autoGrantCurrentUser(userAccessToken, documentID, client.ResourceTypeDocx)
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

		// 阶段一不再因单个块被拒而中止整篇导入：被拒的块逐块隔离并计入 failures（kind=blocks），
		// 只有权限/网络等致命错误才停止后续建块；无论哪种情况都继续输出结果（含 JSON）并以非零退出。
		dTasks, tTasks, media := phase1CreateBlocks(documentID, segments, uploadImages, basePath, stats, verbose, userAccessToken, colWidthMode, colWidthValues)
		iTasks, vTasks := media.images, media.files

		stats.phase1Duration = time.Since(phase1Start)
		stats.tableTotal = len(tTasks)
		stats.imageTotal = stats.imageSkipped + len(iTasks)
		stats.videoTotal = stats.videoSkipped + media.videoCount()
		stats.fileTotal = stats.fileSkipped + len(vTasks) - media.videoCount()
		stats.boardCopyTotal = len(media.boards)
		phase1Summary := fmt.Sprintf("[阶段1] 完成 (%.1fs), 块: %d, 待填表格: %d, 待导入图表: %d",
			stats.phase1Duration.Seconds(), stats.totalBlocks, len(tTasks), len(dTasks))
		if len(iTasks) > 0 {
			phase1Summary += fmt.Sprintf(", 待上传图片: %d", len(iTasks))
		}
		if len(vTasks) > 0 {
			phase1Summary += fmt.Sprintf(", 待上传附件/视频: %d", len(vTasks))
		}
		if len(media.boards) > 0 {
			phase1Summary += fmt.Sprintf(", 待复制画板: %d", len(media.boards))
		}
		if stats.aborted {
			phase1Summary += "（建块中途因致命错误停止，见失败明细）"
		}
		fmt.Fprintln(progressOut, phase1Summary+"\n")

		// === 阶段 2/3: 并发处理 ===
		if len(dTasks) > 0 || len(tTasks) > 0 || len(iTasks) > 0 || len(vTasks) > 0 || len(media.boards) > 0 {
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
				phase2Header += fmt.Sprintf(", 图片+附件×%d", imageWorkers)
			} else if len(iTasks) > 0 {
				phase2Header += fmt.Sprintf(", 图片×%d", imageWorkers)
			} else if len(vTasks) > 0 {
				phase2Header += fmt.Sprintf(", 附件×%d", imageWorkers)
			}
			phase2Header += ") ==="
			fmt.Fprintln(progressOut, phase2Header)
			phase2Start := time.Now()

			failedDiagrams := phase2ConcurrentProcess(documentID, dTasks, tTasks, iTasks, vTasks, diagramWorkers, tableWorkers, imageWorkers, diagramRetries, stats, verbose, userAccessToken)
			processBoardCopies(documentID, media.boards, stats, verbose, userAccessToken)

			stats.phase2Duration = time.Since(phase2Start)
			imageUploadTotal := stats.imageTotal - stats.imageSkipped
			videoUploadTotal := stats.videoTotal - stats.videoSkipped
			fileUploadTotal := stats.fileTotal - stats.fileSkipped
			var mediaInfo string
			if imageUploadTotal > 0 {
				mediaInfo = fmt.Sprintf(", 图片: %d/%d", stats.imageSuccess, imageUploadTotal)
			}
			if videoUploadTotal > 0 {
				mediaInfo += fmt.Sprintf(", 视频: %d/%d", stats.videoSuccess, videoUploadTotal)
			}
			if fileUploadTotal > 0 {
				mediaInfo += fmt.Sprintf(", 附件: %d/%d", stats.fileSuccess, fileUploadTotal)
			}
			if stats.boardCopyTotal > 0 {
				mediaInfo += fmt.Sprintf(", 画板复制: %d/%d", stats.boardCopySuccess+stats.boardCopyAsImage, stats.boardCopyTotal)
			}
			fmt.Fprintf(progressOut, "[阶段2] 完成 (%.1fs), 图表: %d/%d, 表格: %d/%d%s\n\n",
				stats.phase2Duration.Seconds(),
				stats.diagramSuccess, stats.diagramTotal,
				stats.tableSuccess, stats.tableTotal,
				mediaInfo)

			// === 阶段 2.5: 表格单元格图片嵌入（issue #164）===
			// 表格已填充完成（含追加行），此时单元格齐全，按最终单元格顺序为带图单元格建 Image 子块并上传。
			embedTableCellImages(documentID, tTasks, basePath, imageWorkers, stats, verbose, userAccessToken)
			if stats.cellImageTotal > 0 {
				fmt.Fprintf(progressOut, "[阶段2.5] 单元格图片: %d/%d 成功\n\n", stats.cellImageSuccess, stats.cellImageTotal)
			}

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
			if err := printJSON(withPermissionGrant(map[string]any{
				"document_id":         documentID,
				"url":                 client.BuildResourceURL(client.ResourceTypeDocx, documentID),
				"blocks":              stats.totalBlocks,
				"diagram_total":       stats.diagramTotal,
				"diagram_success":     stats.diagramSuccess,
				"diagram_failed":      stats.diagramFailed,
				"mermaid_count":       stats.mermaidCount,
				"plantuml_count":      stats.plantumlCount,
				"svg_count":           stats.svgCount,
				"diagram_fallback":    stats.fallbackSuccess,
				"table_total":         stats.tableTotal,
				"table_success":       stats.tableSuccess,
				"table_failed":        stats.tableFailed,
				"image_total":         stats.imageTotal,
				"image_success":       stats.imageSuccess,
				"image_failed":        stats.imageFailed,
				"image_skipped":       stats.imageSkipped,
				"video_total":         stats.videoTotal,
				"video_success":       stats.videoSuccess,
				"video_failed":        stats.videoFailed,
				"video_skipped":       stats.videoSkipped,
				"file_total":          stats.fileTotal,
				"file_success":        stats.fileSuccess,
				"file_failed":         stats.fileFailed,
				"file_skipped":        stats.fileSkipped,
				"whiteboard_total":    stats.boardCopyTotal,
				"whiteboard_success":  stats.boardCopySuccess,
				"whiteboard_failed":   stats.boardCopyFailed,
				"whiteboard_as_image": stats.boardCopyAsImage,
				"blocks_failed":       stats.blocksFailed,
				"cell_image_total":    stats.cellImageTotal,
				"cell_image_success":  stats.cellImageSuccess,
				"cell_image_failed":   stats.cellImageFailed,
				"duration_seconds":    totalDuration.Seconds(),
				"phase1_seconds":      stats.phase1Duration.Seconds(),
				"phase2_seconds":      stats.phase2Duration.Seconds(),
				"phase3_seconds":      stats.phase3Duration.Seconds(),
				"partial_failure":     len(stats.failures) > 0,
				"failures":            importFailuresForJSON(stats.failures),
			}, grant)); err != nil {
				return err
			}
		} else {
			if len(stats.failures) > 0 {
				fmt.Println("导入结束（部分内容失败，明细见下方）")
			} else {
				fmt.Println("导入完成!")
			}
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
			if stats.fileTotal > 0 {
				fmt.Printf("  附件: %d/%d 成功", stats.fileSuccess, stats.fileTotal)
				if stats.fileSkipped > 0 {
					fmt.Printf(" (%d 跳过)", stats.fileSkipped)
				}
				fmt.Println()
			}
			if stats.boardCopyTotal > 0 {
				fmt.Printf("  画板复制: %d/%d 成功", stats.boardCopySuccess, stats.boardCopyTotal)
				if stats.boardCopyAsImage > 0 {
					fmt.Printf(" (%d 降级为图片)", stats.boardCopyAsImage)
				}
				fmt.Println()
			}
			if stats.blocksFailed > 0 {
				fmt.Printf("  被拒跳过的块: %d\n", stats.blocksFailed)
			}
			if stats.tableTotal > 0 {
				fmt.Printf("  表格: %d/%d 成功\n", stats.tableSuccess, stats.tableTotal)
			}
			if stats.cellImageTotal > 0 {
				if stats.cellImageFailed > 0 {
					fmt.Printf("  表格单元格图片: %d/%d 成功 (%d 失败)\n", stats.cellImageSuccess, stats.cellImageTotal, stats.cellImageFailed)
				} else {
					fmt.Printf("  表格单元格图片: %d/%d 成功\n", stats.cellImageSuccess, stats.cellImageTotal)
				}
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
			fmt.Printf("  链接: %s\n", client.BuildResourceURL(client.ResourceTypeDocx, documentID))
			printPermissionGrantText(os.Stdout, grant)
			printImportFailures(cmd.ErrOrStderr(), stats.failures)
		}

		// 部分内容未写入（图片/表格/视频/单元格图片失败、图表降级也失败、嵌套子块失败）时非零退出：
		// 文档已创建且链接已输出，但脚本/Agent 不能把它当成完整导入。
		return importFailureError(stats.failures, documentID)
	},
}

// importFailuresForJSON 保证 JSON 中 failures 恒为数组（无失败时为 []）。
func importFailuresForJSON(failures []importFailure) []importFailure {
	if failures == nil {
		return []importFailure{}
	}
	return failures
}

// printImportFailures 文本模式下把失败明细打到 stderr。
func printImportFailures(w io.Writer, failures []importFailure) {
	if len(failures) == 0 {
		return
	}
	fmt.Fprintf(w, "\n⚠ 部分内容未能写入文档（%d 项）：\n", len(failures))
	for _, f := range sortedImportFailures(failures) {
		src := ""
		if f.Source != "" {
			src = " (" + f.Source + ")"
		}
		fmt.Fprintf(w, "  - %s %d%s: %s\n", importFailureKindLabel(f.Kind), f.Index, src, f.Error)
	}
}

func sortedImportFailures(failures []importFailure) []importFailure {
	out := append([]importFailure(nil), failures...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Index < out[j].Index
	})
	return out
}

// importFailureError 有失败明细时返回非零退出的错误（文档链接保留在错误信息中）。
func importFailureError(failures []importFailure, documentID string) error {
	if len(failures) == 0 {
		return nil
	}
	counts := map[string]int{}
	var kinds []string
	for _, f := range failures {
		if counts[f.Kind] == 0 {
			kinds = append(kinds, f.Kind)
		}
		counts[f.Kind]++
	}
	sort.Strings(kinds)
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		parts = append(parts, fmt.Sprintf("%s %d 项", importFailureKindLabel(k), counts[k]))
	}
	return fmt.Errorf("部分内容导入失败（%s），已写入的内容保留在文档中，失败明细见上方输出（JSON 模式见 failures 字段）；文档链接: %s",
		strings.Join(parts, "、"), client.BuildResourceURL(client.ResourceTypeDocx, documentID))
}

// phase1CreateBlocks 顺序创建所有文档块，收集待处理的图表、表格与资源补齐任务。
//
// 容错策略：某批块因内容被拒（1770001 等）时逐块隔离，被拒的块计入 failures（kind=blocks）后继续；
// 转换失败的段落计入 failures 后跳过；只有权限/网络等致命错误才停止后续建块（stats.aborted）。
// 无论哪种情况都返回已收集的任务，由调用方继续阶段二并输出结果，不再「文档已建但无任何输出」。
func phase1CreateBlocks(
	documentID string,
	segments []segment,
	uploadImages bool,
	basePath string,
	stats *importStats,
	verbose bool,
	userAccessToken string,
	colWidthMode string,
	colWidthValues []int,
) ([]diagramTask, []tableTask, mediaTaskSet) {
	var dTasks []diagramTask
	var tTasks []tableTask
	var media mediaTaskSet
	diagramIdx := 0
	degradeIdx := map[string]int{}

	for segIdx, seg := range segments {
		if stats.aborted {
			break
		}
		if seg.kind == "markdown" {
			if strings.TrimSpace(seg.content) == "" {
				continue
			}

			options := converter.ConvertOptions{
				UploadImages:     uploadImages,
				EmbedTableImages: true, // 表格单元格图片真嵌入（issue #164），由阶段 2.5 落库
				DocumentID:       documentID,
			}
			applyColumnWidthOptions(&options, colWidthMode, colWidthValues)

			conv := converter.NewMarkdownToBlock([]byte(seg.content), options, basePath)
			result, err := conv.ConvertWithTableData()
			if err != nil {
				stats.progressf("  ✗ 段落 %d 转换失败: %v\n", segIdx+1, err)
				stats.addFailure("blocks", segIdx+1, "", fmt.Errorf("转换 Markdown 失败（该段内容未写入）: %w", err))
				continue
			}

			// 累加图片统计
			stats.imageSkipped += result.ImageStats.Skipped
			stats.videoSkipped += result.VideoStats.Skipped
			stats.fileSkipped += result.FileStats.Skipped

			// 转换期降级（如带 token 的 <sheet>/<bitable> 已转为链接文本）计入失败明细，避免静默丢失
			for _, d := range result.Degradations {
				degradeIdx[d.Kind]++
				stats.addFailure(d.Kind, degradeIdx[d.Kind], d.Source, errors.New(d.Reason))
			}

			if len(result.BlockNodes) == 0 {
				continue
			}

			topLevelBlocks := make([]*larkdocx.Block, len(result.BlockNodes))
			for i, node := range result.BlockNodes {
				topLevelBlocks[i] = node.Block
			}

			// 批量添加顶层块（每批 ≤50；被拒的批次逐块隔离，见 createChildrenIsolated）
			createdTop, blockErrs, fatalErr := createChildrenIsolated(documentID, documentID, topLevelBlocks, userAccessToken)
			for _, e := range blockErrs {
				syncPrintf("  ✗ 段落 %d: %v\n", segIdx+1, e)
				stats.blocksFailed++
				stats.addFailure("blocks", segIdx+1, "", e)
			}
			if fatalErr != nil {
				syncPrintf("  ✗ 段落 %d 建块失败，停止后续写入: %v\n", segIdx+1, fatalErr)
				stats.addFailure("blocks", segIdx+1, "", fmt.Errorf("建块失败，后续内容未写入: %w", fatalErr))
				stats.aborted = true
			}

			// 按树序收集已创建节点（顶层 + 嵌套），供表格与资源补齐任务使用
			var createdAll []createdBlockNode
			createdCount := 0
			for idx, node := range result.BlockNodes {
				if idx >= len(createdTop) || createdTop[idx] == nil || createdTop[idx].BlockId == nil {
					continue
				}
				createdCount++
				stats.totalBlocks++
				top := createdBlockNode{node: node, blockID: *createdTop[idx].BlockId, parentID: documentID, created: createdTop[idx]}
				createdAll = append(createdAll, top)

				nestedCount, nestedCreated, nestedErr := createChildrenOf(documentID, top, userAccessToken)
				if nestedErr != nil {
					// 嵌套子块（如嵌套列表项、分栏列内容）创建失败意味着内容缺失：始终提示并计入失败明细
					syncPrintf("  ✗ 段落 %d 嵌套子块创建失败: %v\n", segIdx+1, nestedErr)
					stats.addFailure("nested_blocks", segIdx+1, "", nestedErr)
				}
				stats.totalBlocks += nestedCount
				createdAll = append(createdAll, nestedCreated...)

				// QuoteContainer / Callout：清理飞书 API 异步生成的空子块（在子块创建完成后执行）
				if node.Block.BlockType != nil {
					deleteContainerAutoEmptyBlock(documentID, top.blockID, *node.Block.BlockType, userAccessToken)
				}
			}

			// 收集表格任务（不立即填充）：按块指针取 TableData，嵌套在分栏等容器内的表格同样会被填充
			tableCount := 0
			for _, cn := range createdAll {
				td := result.TableDataByBlock[cn.node.Block]
				if td == nil {
					continue
				}
				tableCount++
				tTasks = append(tTasks, tableTask{
					index:        len(tTasks) + 1,
					tableBlockID: cn.blockID,
					tableData:    td,
				})
			}

			if verbose {
				stats.progressf("  [段落 %d] 创建 %d 个块, %d 个表格\n", segIdx+1, createdCount, tableCount)
			}

			collectMediaTasks(&media, result.MediaRefs, createdAll, basePath)

		} else if seg.kind == "mermaid" || seg.kind == "plantuml" || seg.kind == "svg" {
			diagramIdx++
			syntaxLabel := diagramSyntaxLabel(seg.kind)

			if verbose {
				stats.progressf("  [%s %d] 创建画板占位块...\n", syntaxLabel, diagramIdx)
			}

			// 只创建画板占位块，不导入图表
			boardClientToken := client.NewClientToken() // 重试复用同一 token，避免重复创建画板块
			createResult := client.DoWithRetry(func() (*client.AddBoardResult, http.Header, error) {
				return client.AddBoardWithClientToken(documentID, "", -1, boardClientToken, userAccessToken)
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
				stats.addFailure("diagram", diagramIdx, syntaxLabel, fmt.Errorf("创建画板失败（图表内容未写入）: %w", createResult.Err))
				continue
			}
			boardResult := createResult.Value

			if boardResult.WhiteboardID == "" {
				stats.progressf("  ✗ %s %d 未返回画板 ID\n", syntaxLabel, diagramIdx)
				stats.diagramFailed++
				stats.addFailure("diagram", diagramIdx, syntaxLabel, fmt.Errorf("创建画板未返回画板 ID（图表内容未写入）"))
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

	return dTasks, tTasks, media
}

// processBoardCopies 顺序复制带 token 的画板（画板节点写入有频控，顺序执行即可）。
func processBoardCopies(documentID string, tasks []boardCopyTask, stats *importStats, verbose bool, userAccessToken string) {
	for _, t := range tasks {
		how, err := processBoardCopyTask(documentID, t, verbose, userAccessToken)
		stats.mu.Lock()
		switch {
		case err != nil:
			stats.boardCopyFailed++
		case how == boardCopiedAsImage:
			stats.boardCopyAsImage++ // 设计内降级：内容以图片保留，不计入 failures
		default:
			stats.boardCopySuccess++
		}
		stats.mu.Unlock()
		if err != nil {
			stats.addFailure("whiteboard", t.index, t.sourceToken, err)
		}
	}
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

	// 表格 cellMap 预热（issue #159）：阶段一已经创建了所有表格 block 与默认空 cell，
	// 这里一次性拉全文档块树，建立 cellID(BlockType=32) -> textBlockID(children[0]) 映射，
	// 多个 table worker 只读共享，避免逐 cell 调 GetBlockChildren。
	// 失败不致命：tTasks 留空 cellMap 时，FillTableCellsRichWithMap 自动降级到旧路径。
	if len(tTasks) > 0 {
		if cellMap, err := buildCellTextBlockMap(documentID, userAccessToken); err == nil {
			for i := range tTasks {
				tTasks[i].cellMap = cellMap
			}
			if verbose {
				stats.progressf("[阶段2] 预热 cellMap: %d cells\n", len(cellMap))
			}
		} else if verbose {
			stats.progressf("[阶段2] cellMap 预热失败，降级逐 cell 路径: %v\n", err)
		}
	}

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
			if !result.success {
				stats.addFailure("table", t.index, "", result.err)
			}
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
				if !result.success {
					stats.addFailure("image", t.index, t.source, result.err)
				}
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
				switch {
				case t.video && result.success:
					stats.videoSuccess++
				case t.video:
					stats.videoFailed++
				case result.success:
					stats.fileSuccess++
				default:
					stats.fileFailed++
				}
				stats.mu.Unlock()
				if !result.success {
					stats.addFailure(t.failureKind(), t.index, t.source, result.err)
				}
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

	attempt := 0
	result := client.DoWithRetry(func() (*client.ImportDiagramResult, http.Header, error) {
		attempt++
		// 每个图表写入的都是本次导入新建的空画板；/nodes/plantuml 不认 client_token（实测重复请求会重复建图），
		// 所以重试改用 overwrite 原子覆盖：上一次请求若其实已落地（只是响应超时/5xx），重试不会叠出第二张图。
		retryOpts := opts
		retryOpts.Overwrite = attempt > 1
		return client.ImportDiagram(task.whiteboardID, task.content, retryOpts)
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

// buildCellTextBlockMap 拉一次全文档块树，把 BlockType==32（TableCell）的
// cellID -> 默认空 text 块 ID（Children[0]）建成只读映射，供 phase2 多个 table worker
// 共享。返回的 map 在 import 流程内只读，调用方不要改。
//
// 设计权衡：单次 GetAllBlocksWithToken 的成本是 ceil(blocks/500) 次 List，
// 远低于逐 cell 调 GetBlockChildren 的 N 次往返；阶段一刚跑完 CreateBlock，
// 飞书侧块树已就绪，无一致性风险。
func buildCellTextBlockMap(documentID, userAccessToken string) (map[string]string, error) {
	blocks, err := client.GetAllBlocksWithToken(documentID, userAccessToken)
	if err != nil {
		return nil, err
	}
	return cellTextBlockMapFromBlocks(blocks), nil
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
		return nil, fillTableWithExtraRows(documentID, task.tableBlockID, task.tableData, userAccessToken, onProgress, task.cellMap)
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

	// 复用已有素材 token 失败时，把空 Image 块替换为占位文本（避免留下看不出原因的空图）
	fail := func(err error) imageResult {
		if task.reuseToken != "" {
			placeholder := fmt.Sprintf("[图片未能复用: %s%s]", converter.FeishuMediaScheme, task.reuseToken)
			if phErr := replaceBlockWithText(documentID, task.parentID, task.imageBlockID, placeholder, userAccessToken); phErr != nil {
				syncPrintf("  ⚠ %s %d 占位失败: %v\n", task.kindLabel(), task.index, phErr)
			}
		}
		return imageResult{task: task, success: false, err: err}
	}

	// 解析图片来源（本地路径 / URL 下载 / feishu://media/<token> 素材复用）
	localPath, fileName, cleanup, err := resolveMediaSourceAs(task.source, task.basePath, ".png", "", userAccessToken)
	if err != nil {
		syncPrintf("  ✗ %s %d 解析失败 (%s): %v\n", task.kindLabel(), task.index, task.source, err)
		return fail(err)
	}
	defer cleanup()

	// 检查文件大小（≤ 20MB）
	const maxImageSize = 20 * 1024 * 1024
	fi, err := os.Stat(localPath)
	if err != nil {
		syncPrintf("  ✗ %s %d 文件信息获取失败: %v\n", task.kindLabel(), task.index, err)
		return fail(err)
	}
	if fi.Size() > maxImageSize {
		err := fmt.Errorf("图片超过 20MB 限制 (%.1f MB)", float64(fi.Size())/(1024*1024))
		syncPrintf("  ✗ %s %d: %v\n", task.kindLabel(), task.index, err)
		return fail(err)
	}

	extra := fmt.Sprintf(`{"drive_route_token":"%s"}`, documentID)

	// 步骤 2: 上传图片到 Image Block
	retryCfg := client.RetryConfig{
		MaxRetries:       maxRetries,
		MaxTotalAttempts: maxRetries + 3,
		RetryOnRateLimit: true,
		OnRetry: func(attempt int, err error, wait time.Duration) {
			if verbose {
				syncPrintf("  ⚠ %s %d 上传重试 %d/%d (等待 %.1fs): %v\n",
					task.kindLabel(), task.index, attempt, maxRetries, wait.Seconds(), err)
			}
		},
	}

	uploadResult := client.DoWithRetry(func() (string, http.Header, error) {
		return client.UploadMediaWithExtra(localPath, "docx_image", task.imageBlockID, fileName, extra, userAccessToken)
	}, retryCfg)

	if uploadResult.Err != nil {
		syncPrintf("  ✗ %s %d 上传失败 (%s): %v\n", task.kindLabel(), task.index, task.source, uploadResult.Err)
		return fail(uploadResult.Err)
	}

	fileToken := uploadResult.Value

	// 步骤 3: 绑定 token 并显式带上显示宽高（为何必须带宽高见 client.ReplaceImage 注释）。
	// <image width height> 给出的原显示尺寸优先（roundtrip 保持原文档的显示大小），否则用真实像素；
	// 只给一边时按原图比例补另一边；解码失败时传 0 退回服务端推断，不影响导入成功。
	pxW, pxH := decodeImagePixelSize(localPath)
	dispW, dispH := pxW, pxH
	if task.width > 0 || task.height > 0 {
		if w, h, dimErr := resolveImageDisplaySize(task.width, task.height, task.width > 0, task.height > 0, pxW, pxH); dimErr == nil {
			dispW, dispH = w, h
		}
	}
	if verbose && (dispW == 0 || dispH == 0) {
		syncPrintf("  ⚠ %s %d 无法解析像素尺寸，显示尺寸交由服务端推断 (%s)\n", task.kindLabel(), task.index, task.source)
	}
	replaceResult := client.DoVoidWithRetry(func() (http.Header, error) {
		return client.ReplaceImage(documentID, task.imageBlockID, fileToken,
			client.ReplaceImageOptions{Width: dispW, Height: dispH, Align: task.align}, userAccessToken)
	}, retryCfg)

	if replaceResult.Err != nil {
		syncPrintf("  ✗ %s %d 替换失败 (token=%s): %v\n", task.kindLabel(), task.index, fileToken, replaceResult.Err)
		return fail(replaceResult.Err)
	}

	if verbose {
		syncPrintf("  ✓ %s %d 成功 (%s)\n", task.kindLabel(), task.index, task.source)
	}
	return imageResult{task: task, success: true}
}

// embedTableCellImages 在表格填充完成后，把 Markdown 表格单元格内的图片真正嵌入为单元格内的 Image 子块
// （issue #164）。此时表格已填好（含 insert_table_row 追加的行），单元格齐全。流程：
//  1. 对每个含单元格图片的表格，用 GetTableCellIDs（block.Table.Cells，与阶段 2 填充、导出同源）取
//     有序单元格 ID；不依赖 list-blocks 是否为表格块填充 children；
//  2. 按 TableData.CellImages 的行优先索引对齐到 cellID（数量不齐则把这些图片计入失败并跳过该表，
//     既避免错位，也让非 verbose 摘要能反映丢弃、不再静默吞掉——正是 #164 要修的症状）；
//  3. 为带图单元格 CreateBlock 一个空 Image 子块，再复用 processImageTask 走「上传媒体→替换 token」三步法；
//     上传失败时删除该空块并补占位 Text，避免单元格里留孤儿空图。
//
// 并发受 imageWorkers 限；CreateBlock / ReplaceImage 均已内置 docWriteLimiter（单文档 3 QPS），无需额外限流。
func embedTableCellImages(documentID string, tTasks []tableTask, basePath string, imageWorkers int, stats *importStats, verbose bool, userAccessToken string) {
	// 统计所有"打算嵌入"的单元格图片总数（含后续可能被跳过的），确保丢弃也计入 total、可观测。
	totalIntended := 0
	for _, t := range tTasks {
		if t.tableData == nil {
			continue
		}
		for _, imgs := range t.tableData.CellImages {
			totalIntended += len(imgs)
		}
	}
	if totalIntended == 0 {
		return
	}

	stats.mu.Lock()
	stats.cellImageTotal += totalIntended
	stats.mu.Unlock()

	// 收集工作项：(单元格ID, 图片源)。单元格 ID 取自 block.Table.Cells（GetTableCellIDs）。
	type cellImgWork struct {
		cellID string
		source string
	}
	var works []cellImgWork
	for _, t := range tTasks {
		if t.tableData == nil || len(t.tableData.CellImages) == 0 {
			continue
		}
		tableImgCount := 0
		for _, imgs := range t.tableData.CellImages {
			tableImgCount += len(imgs)
		}
		if tableImgCount == 0 {
			continue
		}
		cellIDs, err := client.GetTableCellIDs(documentID, t.tableBlockID, userAccessToken)
		if err != nil {
			syncPrintf("  ✗ 表格 %d 获取单元格失败，跳过 %d 张单元格图片: %v\n", t.index, tableImgCount, err)
			stats.mu.Lock()
			stats.cellImageFailed += tableImgCount
			stats.mu.Unlock()
			stats.addFailure("cell_image", t.index, "", fmt.Errorf("表格 %d 获取单元格失败，%d 张单元格图片未写入: %w", t.index, tableImgCount, err))
			continue
		}
		if len(cellIDs) != len(t.tableData.CellImages) {
			// 单元格数与图片索引不一致（理论上不应发生），跳过该表避免把图片塞错单元格；
			// 计入失败而非静默丢弃，使非 verbose 摘要也能反映。
			syncPrintf("  ⚠ 表格 %d 单元格数(%d)与图片索引(%d)不一致，跳过 %d 张单元格图片\n",
				t.index, len(cellIDs), len(t.tableData.CellImages), tableImgCount)
			stats.mu.Lock()
			stats.cellImageFailed += tableImgCount
			stats.mu.Unlock()
			stats.addFailure("cell_image", t.index, "", fmt.Errorf("表格 %d 单元格数(%d)与图片索引(%d)不一致，%d 张单元格图片未写入",
				t.index, len(cellIDs), len(t.tableData.CellImages), tableImgCount))
			continue
		}
		for ci, imgs := range t.tableData.CellImages {
			for _, src := range imgs {
				works = append(works, cellImgWork{cellID: cellIDs[ci], source: src})
			}
		}
	}
	if len(works) == 0 {
		return
	}

	if verbose {
		stats.progressf("=== 阶段 2.5: 表格单元格图片嵌入 (%d 张, ×%d) ===\n", len(works), imageWorkers)
	}

	bt := int(converter.BlockTypeImage)
	var wg sync.WaitGroup
	var cellCleanupMu sync.Mutex // 串行化失败占位（按 index 删块），避免同格多图并发清理误删
	sem := make(chan struct{}, imageWorkers)
	for i, w := range works {
		wg.Add(1)
		go func(idx int, w cellImgWork) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// 步骤 1：在单元格内创建一个空 Image 子块
			created, _, err := client.CreateBlock(documentID, w.cellID, []*larkdocx.Block{{
				BlockType: &bt,
				Image:     &larkdocx.Image{},
			}}, -1, userAccessToken)
			if err != nil || len(created) == 0 || created[0].BlockId == nil {
				syncPrintf("  ✗ 单元格图片 %d 建块失败 (%s): %v\n", idx+1, w.source, err)
				stats.mu.Lock()
				stats.cellImageFailed++
				stats.mu.Unlock()
				if err == nil {
					err = fmt.Errorf("创建单元格图片块未返回块 ID")
				}
				stats.addFailure("cell_image", idx+1, w.source, err)
				return
			}

			// 步骤 2/3：复用图片三步法（上传媒体 → 替换 token）
			imageBlockID := client.StringVal(created[0].BlockId)
			res := processImageTask(documentID, imageTask{
				index:        idx + 1,
				imageBlockID: imageBlockID,
				source:       w.source,
				basePath:     basePath,
				label:        "单元格图片", // 与顶层"图片 N"编号空间不同，消息前缀区分
			}, verbose, userAccessToken)

			stats.mu.Lock()
			if res.success {
				stats.cellImageSuccess++
			} else {
				stats.cellImageFailed++
			}
			stats.mu.Unlock()
			if !res.success {
				stats.addFailure("cell_image", idx+1, w.source, res.err)
			}

			// 上传失败：删除步骤 1 建的空 Image 块并补占位 Text，避免单元格里留孤儿空图。
			// 串行化所有占位操作：replaceFailedCellImageBlock 内部每次重新拉子块并按 block ID 定位，
			// 串行后删除用的 index 与刚取的快照一致；步骤 1 的 CreateBlock 是末尾追加（-1），不移动已存在
			// 块的靠前 index，故无需纳入此锁。这样同一单元格多图同时失败也不会并发误删兄弟块。
			if !res.success {
				cellCleanupMu.Lock()
				replaceFailedCellImageBlock(documentID, w.cellID, imageBlockID, w.source, idx+1, userAccessToken)
				cellCleanupMu.Unlock()
			}
		}(i, w)
	}
	wg.Wait()
}

// replaceFailedCellImageBlock 把上传失败的单元格内空 Image 块替换为可见占位 Text，
// 避免阶段 2.5 建的空块在单元格里变成孤儿空图（与 EmbedTableImages=off 的占位降级、视频失败的
// replaceFailedVideoBlock 行为对齐）。按 block ID 定位再"删除+原位插 Text"，占位失败仅记日志、不影响整体导入。
func replaceFailedCellImageBlock(documentID, cellID, imageBlockID, source string, idx int, userAccessToken string) {
	children, err := client.GetAllBlockChildren(documentID, cellID, userAccessToken)
	if err != nil {
		syncPrintf("  ⚠ 单元格图片 %d 占位失败（无法获取单元格子块）: %v\n", idx, err)
		return
	}
	pos := -1
	for i, child := range children {
		if child.BlockId != nil && *child.BlockId == imageBlockID {
			pos = i
			break
		}
	}
	if pos < 0 {
		return // 空块已不在（可能被并发清理），无需处理
	}
	if _, err := client.DeleteBlocks(documentID, cellID, pos, pos+1, userAccessToken); err != nil {
		syncPrintf("  ⚠ 单元格图片 %d 占位失败（删除空 Image 块失败）: %v\n", idx, err)
		return
	}
	placeholder := fmt.Sprintf("[图片上传失败: %s]", source)
	textBlockType := int(converter.BlockTypeText)
	textBlock := &larkdocx.Block{
		BlockType: &textBlockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: &placeholder}},
			},
		},
	}
	if _, _, err := client.CreateBlock(documentID, cellID, []*larkdocx.Block{textBlock}, pos, userAccessToken); err != nil {
		syncPrintf("  ⚠ 单元格图片 %d 占位失败（插入 Text 块失败）: %v\n", idx, err)
	}
}

// processVideoTask 处理单个附件/视频上传任务（File Block）：解析来源（本地 / URL / feishu://media/ 素材复用）
// → 上传到 File 块 → replace_file。失败时把空附件块替换为可见占位文本。
func processVideoTask(documentID string, task videoTask, verbose bool, userAccessToken string) videoResult {
	const maxRetries = 5

	failWith := func(reason string, err error) videoResult {
		// 上传失败：把阶段 1 创建的空 File 块（连同外层 View 块）替换为可见占位 Text 块，避免文档里留孤儿
		fileName := task.name
		if fileName == "" {
			fileName = pathpkg.Base(task.source)
		}
		if fileName == "" || fileName == "." || fileName == "/" {
			fileName = task.source
		}
		replaceFailedVideoBlock(documentID, task, fileName, reason, userAccessToken)
		return videoResult{task: task, success: false, err: err}
	}

	defaultExt := ""
	if task.video {
		defaultExt = ".mp4"
	}
	localPath, fileName, cleanup, err := resolveMediaSourceAs(task.source, task.basePath, defaultExt, task.name, userAccessToken)
	if err != nil {
		syncPrintf("  ✗ %s %d 解析失败 (%s): %v\n", task.kindLabel(), task.index, task.source, err)
		return failWith(fmt.Sprintf("解析失败: %v", err), err)
	}
	defer cleanup()

	fi, err := os.Stat(localPath)
	if err != nil {
		syncPrintf("  ✗ %s %d 文件信息获取失败: %v\n", task.kindLabel(), task.index, err)
		return failWith(fmt.Sprintf("文件信息获取失败: %v", err), err)
	}
	extra := fmt.Sprintf(`{"drive_route_token":"%s"}`, documentID)
	retryCfg := client.RetryConfig{
		MaxRetries:       maxRetries,
		MaxTotalAttempts: maxRetries + 3,
		RetryOnRateLimit: true,
		OnRetry: func(attempt int, err error, wait time.Duration) {
			if verbose {
				syncPrintf("  ⚠ %s %d 上传重试 %d/%d (等待 %.1fs): %v\n",
					task.kindLabel(), task.index, attempt, maxRetries, wait.Seconds(), err)
			}
		},
	}

	var uploadResult client.RetryResult[string]
	if fi.Size() > maxInlineVideoSize {
		// 大文件分片上传：分片级重试在 UploadDocMedia 内部完成，外层不再整体重放
		if verbose {
			syncPrintf("  [%s %d] %.1f MB，走分片上传\n", task.kindLabel(), task.index, float64(fi.Size())/(1024*1024))
		}
		token, upErr := client.UploadDocMedia(localPath, "docx_file", task.fileBlockID, fileName, documentID, userAccessToken)
		uploadResult = client.RetryResult[string]{Value: token, Err: upErr}
	} else {
		uploadResult = client.DoWithRetry(func() (string, http.Header, error) {
			return client.UploadMediaWithExtra(localPath, "docx_file", task.fileBlockID, fileName, extra, userAccessToken)
		}, retryCfg)
	}
	if uploadResult.Err != nil {
		syncPrintf("  ✗ %s %d 上传失败 (%s): %v\n", task.kindLabel(), task.index, task.source, uploadResult.Err)
		return failWith(fmt.Sprintf("上传失败: %v", uploadResult.Err), uploadResult.Err)
	}

	fileToken := uploadResult.Value
	replaceResult := client.DoVoidWithRetry(func() (http.Header, error) {
		return client.UpdateBlock(documentID, task.fileBlockID, map[string]any{
			"replace_file": map[string]any{"token": fileToken},
		}, userAccessToken)
	}, retryCfg)
	if replaceResult.Err != nil {
		syncPrintf("  ✗ %s %d 绑定失败 (token=%s): %v\n", task.kindLabel(), task.index, fileToken, replaceResult.Err)
		return failWith(fmt.Sprintf("绑定失败: %v", replaceResult.Err), replaceResult.Err)
	}

	if verbose {
		syncPrintf("  ✓ %s %d 成功 (%s)\n", task.kindLabel(), task.index, task.source)
	}
	return videoResult{task: task, success: true}
}

// replaceFailedVideoBlock 将上传失败的附件/视频块替换为可见的占位 Text 块，
// 避免阶段 1 创建的空 File 块在文档中变成孤儿（用户看不到任何内容）。
// 飞书 PatchBlock 不支持跨类型变更，采用「删除外层 View 块 + 同位置插入 Text」；支持嵌套在列表、分栏等容器内的块。
// 占位失败仅记日志，不让整体导入崩溃。
func replaceFailedVideoBlock(documentID string, task videoTask, fileName, reason, userAccessToken string) {
	// 占位文本只保留简短原因（完整错误在 failures 里），避免把长错误写进文档正文
	placeholder := fmt.Sprintf("[%s上传失败：%s (%s)]", task.kindLabel(), fileName, truncateRunes(reason, 80))
	if task.reuseToken != "" {
		placeholder = fmt.Sprintf("[%s未能复用：%s (%s%s)]", task.kindLabel(), fileName, converter.FeishuMediaScheme, task.reuseToken)
	}
	target := task.viewBlockID
	if target == "" {
		target = task.fileBlockID
	}
	if err := replaceBlockWithText(documentID, task.parentID, target, placeholder, userAccessToken); err != nil {
		syncPrintf("  ⚠ %s %d 占位块创建失败: %v\n", task.kindLabel(), task.index, err)
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

// resolveMediaSourceAs 在 resolveMediaSource 基础上支持 feishu://media/<token>（下载已有素材用于重新上传，
// 即素材复用），并允许指定上传文件名（附件/视频沿用原文件名）。
func resolveMediaSourceAs(source, basePath, defaultExt, name, userAccessToken string) (string, string, func(), error) {
	if token, ok := strings.CutPrefix(source, converter.FeishuMediaScheme); ok {
		if token == "" {
			return "", "", nil, fmt.Errorf("素材引用缺少 token: %s", source)
		}
		return downloadFeishuMedia(token, name, defaultExt, userAccessToken)
	}
	localPath, fileName, cleanup, err := resolveMediaSource(source, basePath, defaultExt)
	if err != nil {
		return "", "", nil, err
	}
	if n := strings.TrimSpace(name); n != "" && n != "." && n != "/" {
		fileName = n
	}
	return localPath, fileName, cleanup, nil
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

	// Markdown 引用的本地文件同样是用户输入：按敏感目录拒绝名单校验（按绝对路径判断，
	// 允许 ../images/a.png 这类相对 Markdown 目录的正常引用），失败计入该图片/附件的 failures
	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return "", "", nil, fmt.Errorf("解析本地文件路径失败 %s: %w", localPath, err)
	}
	if err := safefile.ValidateInputPath(absPath); err != nil {
		return "", "", nil, err
	}
	if _, err := os.Stat(localPath); err != nil {
		return "", "", nil, fmt.Errorf("图片文件不存在: %s", localPath)
	}
	return localPath, filepath.Base(localPath), noop, nil
}

// createNestedChildren 递归创建嵌套子块（如嵌套列表的父子关系），返回创建的块总数、按树序的已创建节点
// 与汇总错误。被拒的子块逐块隔离（见 createChildrenIsolated），其余子块与更深层内容照常写入。
func createNestedChildren(documentID string, parentBlockID string, children []*converter.BlockNode, userAccessToken string) (int, []createdBlockNode, error) {
	if len(children) == 0 {
		return 0, nil, nil
	}

	blocks := make([]*larkdocx.Block, len(children))
	for i, c := range children {
		blocks[i] = c.Block
	}
	created, blockErrs, fatalErr := createChildrenIsolated(documentID, parentBlockID, blocks, userAccessToken)
	errs := append([]error(nil), blockErrs...)

	totalCreated := 0
	var createdNodes []createdBlockNode
	for i, child := range children {
		if i >= len(created) || created[i] == nil || created[i].BlockId == nil {
			continue
		}
		totalCreated++
		cn := createdBlockNode{node: child, blockID: *created[i].BlockId, parentID: parentBlockID, created: created[i]}
		createdNodes = append(createdNodes, cn)
		if len(child.Children) > 0 {
			nestedCount, nestedCreated, err := createChildrenOf(documentID, cn, userAccessToken)
			totalCreated += nestedCount
			createdNodes = append(createdNodes, nestedCreated...)
			if err != nil {
				errs = append(errs, err)
			}
		}
		// QuoteContainer / Callout 嵌套场景：无论是否有子块，均清理 API 自动生成的空块
		if child.Block.BlockType != nil {
			deleteContainerAutoEmptyBlock(documentID, cn.blockID, *child.Block.BlockType, userAccessToken)
		}
	}
	if fatalErr != nil {
		errs = append(errs, fatalErr)
	}
	if len(errs) > 0 {
		return totalCreated, createdNodes, fmt.Errorf("创建嵌套子块失败 (parent=%s): %w", parentBlockID, errors.Join(errs...))
	}
	return totalCreated, createdNodes, nil
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
		for _, r := range failedDiagrams {
			stats.addFailure("diagram", r.task.index, diagramSyntaxLabel(r.task.syntax),
				fmt.Errorf("图表导入失败（%v），且获取文档子块失败无法降级为代码块: %w", r.err, err))
		}
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
			stats.addFailure("diagram", r.task.index, syntaxLabel, fmt.Errorf("图表导入失败（%v），画板块未找到无法降级为代码块", r.err))
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
			stats.addFailure("diagram", item.result.task.index, syntaxLabel, fmt.Errorf("图表导入失败（%v），降级时删除空画板失败: %w", item.result.err, err))
			continue
		}

		// 2. 在同位置插入代码块
		codeBlock := createDiagramCodeBlock(item.result.task.syntax, item.result.task.content)
		_, _, err = client.CreateBlock(documentID, documentID, []*larkdocx.Block{codeBlock}, item.index, userAccessToken)
		if err != nil {
			stats.progressf("  ✗ %s %d 插入代码块失败: %v\n", syntaxLabel, item.result.task.index, err)
			stats.fallbackFailed++
			stats.addFailure("diagram", item.result.task.index, syntaxLabel, fmt.Errorf("图表导入失败（%v），降级插入代码块失败（图表内容未写入）: %w", item.result.err, err))
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
	importMarkdownCmd.Flags().StringP("document-id", "d", "", "已有文档 ID 或 URL（内容追加到末尾；wiki URL 自动解析为底层文档）")
	importMarkdownCmd.Flags().Bool("upload-images", true, "上传本地图片")
	importMarkdownCmd.Flags().StringP("folder", "f", "", "新文档的文件夹 Token")
	importMarkdownCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	importMarkdownCmd.Flags().BoolP("verbose", "v", false, "显示详细进度")
	importMarkdownCmd.Flags().Int("diagram-workers", 5, "图表 (Mermaid/PlantUML) 并发导入数")
	importMarkdownCmd.Flags().Int("table-workers", 3, "表格并发填充数")
	importMarkdownCmd.Flags().Int("image-workers", 2, "图片并发上传数 (API 限制 5 QPS)")
	importMarkdownCmd.Flags().Int("diagram-retries", 10, "图表最大重试次数")
	importMarkdownCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文档）")
	importMarkdownCmd.Flags().String("table-column-width", "auto",
		"Markdown 表格列宽策略：auto（按内容启发式）| fixed（按文档宽度均分）| 像素列表如 80,200,*,120（* 表示该列走 auto）")
	// 向后兼容别名
	importMarkdownCmd.Flags().Int("mermaid-workers", 5, "图表并发导入数 (--diagram-workers 别名)")
	importMarkdownCmd.Flags().Int("mermaid-retries", 10, "图表最大重试次数 (--diagram-retries 别名)")
	_ = importMarkdownCmd.Flags().MarkHidden("mermaid-workers")
	_ = importMarkdownCmd.Flags().MarkHidden("mermaid-retries")
}
