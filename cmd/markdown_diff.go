package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// markdownDiffCmd 比对 Drive 上的 Markdown 内容并打印 unified diff。
//
// 注意：本命令不是单纯的 API 调用——它先把远端 Markdown 内容下载下来（最新版或指定历史版本），
// 在本地计算 unified diff 并打印，不会修改远端文件。
//
// 三种比对模式（互斥）：
//   - 远端最新 vs 本地文件：--file-token + --file
//   - 远端某版本 vs 远端最新：--file-token + --from-version
//   - 远端版本 A vs 版本 B：  --file-token + --from-version + --to-version
var markdownDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "比对 Markdown 内容（远端版本之间，或远端 vs 本地文件），输出 unified diff",
	Long: `下载远端 Markdown 内容并在本地计算 unified diff，不修改远端文件。

三种比对模式（互斥，由参数组合决定）:
  1. 远端最新 vs 本地文件:   --file-token <token> --file ./local.md
  2. 远端某版本 vs 远端最新:  --file-token <token> --from-version <v>
  3. 远端版本 A vs 版本 B:    --file-token <token> --from-version <a> --to-version <b>

注：模式 2/3（远端版本对比）需该文件具备版本历史快照——普通 .md（Drive 原生 Markdown）
覆盖为原地替换、无数字版本，?version=N 会返回 404；版本对比主要适用 docx/sheet/bitable
等有版本管理的文档。模式 1（远端最新 vs 本地文件）适用任意可下载的 .md。

可选:
  --context-lines  diff 每个 hunk 上下保留的未变更上下文行数（默认 3）
  --dry-run        只打印将要执行的比对计划，不下载/不比对
  --format         json|pretty|table|ndjson|csv —— 输出结构化 hunk（缺省打印 unified diff 文本）
  --jq             用内置 gojq 过滤结构化输出（如 --jq '.added_lines'）
  --output, -o     [兼容] -o json 等价 --format json

权限:
  - User Access Token
  - drive:file:download（或 drive:drive）

示例:
  feishu-cli markdown diff --file-token boxcnxxx --file ./local.md
  feishu-cli markdown diff --file-token boxcnxxx --from-version 3
  feishu-cli markdown diff --file-token boxcnxxx --from-version 2 --to-version 5 --context-lines 1`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken, _ := cmd.Flags().GetString("file-token")
		localFile, _ := cmd.Flags().GetString("file")
		fromVersion, _ := cmd.Flags().GetString("from-version")
		toVersion, _ := cmd.Flags().GetString("to-version")
		contextLines, _ := cmd.Flags().GetInt("context-lines")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		fileToken = strings.TrimSpace(fileToken)
		localFile = strings.TrimSpace(localFile)
		fromVersion = strings.TrimSpace(fromVersion)
		toVersion = strings.TrimSpace(toVersion)

		if fileToken == "" {
			return fmt.Errorf("--file-token 必填")
		}
		if contextLines < 0 {
			return fmt.Errorf("--context-lines 不能为负（当前 %d）", contextLines)
		}

		mode, err := resolveMarkdownDiffMode(localFile, fromVersion, toVersion)
		if err != nil {
			return err
		}

		if dryRun {
			plan := map[string]any{
				"detection":     mode,
				"file_token":    fileToken,
				"context_lines": contextLines,
			}
			switch mode {
			case "local_vs_remote":
				plan["local_file"] = localFile
				plan["remote"] = "latest"
			case "remote_vs_remote":
				plan["from_version"] = fromVersion
				if toVersion != "" {
					plan["to_version"] = toVersion
				} else {
					plan["to_version"] = "latest"
				}
			}
			// dry-run 计划本就是结构化数据，默认 JSON 输出；--format/--jq 也尊重。
			o, _, oerr := resolveMarkdownDiffOutput(cmd)
			if oerr != nil {
				return oerr
			}
			if o == nil {
				o, _ = output.NewOptions(output.FormatJSON, "")
			}
			return output.Render(o, plan)
		}

		token, err := requireUserToken(cmd, "markdown diff")
		if err != nil {
			return err
		}

		var (
			fromName, toName   string
			fromBytes, toBytes []byte
		)

		switch mode {
		case "local_vs_remote":
			// base = 远端最新，target = 本地文件
			fromName = "remote (latest)"
			fromBytes, err = client.FetchFileContent(fileToken, token)
			if err != nil {
				return fmt.Errorf("下载远端内容失败: %w", err)
			}
			toName = "local: " + localFile
			toBytes, err = os.ReadFile(localFile)
			if err != nil {
				return fmt.Errorf("读取本地文件失败: %w", err)
			}
		case "remote_vs_remote":
			fromName = "remote@version=" + fromVersion
			fromBytes, err = fetchMarkdownVersionContent(fileToken, fromVersion, token)
			if err != nil {
				return fmt.Errorf("下载 from-version 内容失败（版本对比需该文件有版本历史快照；普通 .md 无数字版本，?version 会 404，仅 docx/sheet/bitable 等支持）: %w", err)
			}
			if toVersion != "" {
				toName = "remote@version=" + toVersion
				toBytes, err = fetchMarkdownVersionContent(fileToken, toVersion, token)
				if err != nil {
					return fmt.Errorf("下载 to-version 内容失败（版本对比需该文件有版本历史快照；普通 .md 无数字版本，?version 会 404）: %w", err)
				}
			} else {
				toName = "remote (latest)"
				toBytes, err = client.FetchFileContent(fileToken, token)
				if err != nil {
					return fmt.Errorf("下载远端最新内容失败: %w", err)
				}
			}
		}

		if err := checkDiffSize(string(fromBytes), string(toBytes)); err != nil {
			return err
		}

		hunks := unifiedDiff(string(fromBytes), string(toBytes), contextLines)

		// 用户显式要结构化输出（--format / --jq / 兼容的 -o json）时走统一渲染；否则输出 unified diff 文本。
		o, structured, oerr := resolveMarkdownDiffOutput(cmd)
		if oerr != nil {
			return oerr
		}
		if structured {
			added, removed := countDiffLines(hunks)
			return output.Render(o, map[string]any{
				"detection":         mode,
				"from":              fromName,
				"to":                toName,
				"size_bytes_before": len(fromBytes),
				"size_bytes_after":  len(toBytes),
				"identical":         len(hunks) == 0,
				"added_lines":       added,
				"removed_lines":     removed,
				"hunks":             hunks,
			})
		}

		if len(hunks) == 0 {
			fmt.Println("No differences.")
			return nil
		}
		fmt.Printf("--- %s\n", fromName)
		fmt.Printf("+++ %s\n", toName)
		fmt.Print(renderUnifiedDiff(hunks))
		return nil
	},
}

// resolveMarkdownDiffOutput 判定 markdown diff 是否走统一结构化输出：
// 用户未传 --format/--jq 且 -o 不为 json 时返回 (nil, false)，由调用方输出 unified diff 文本（diff 命令的自然形态）；
// 否则返回 (*output.Options, true) 走 output 包渲染（支持 --format/--jq）。
// -o json 作为旧式兼容别名映射到 --format json（未发版前已文档化，保留兼容）。
func resolveMarkdownDiffOutput(cmd *cobra.Command) (*output.Options, bool, error) {
	oldOut, _ := cmd.Flags().GetString("output")
	jqExpr, _ := cmd.Flags().GetString("jq")
	formatFlag, _ := cmd.Flags().GetString("format")
	changedFormat := cmd.Flags().Changed("format")
	changedJQ := cmd.Flags().Changed("jq")
	if !changedFormat && !changedJQ && oldOut != "json" {
		return nil, false, nil
	}
	format := formatFlag
	if !changedFormat {
		format = output.FormatJSON // 仅 --jq 或 -o json 时默认 JSON
	}
	o, err := output.NewOptions(format, jqExpr)
	return o, true, err
}

// resolveMarkdownDiffMode 根据参数组合判定比对模式，并校验互斥/缺省。
func resolveMarkdownDiffMode(localFile, fromVersion, toVersion string) (string, error) {
	hasLocal := localFile != ""
	hasFrom := fromVersion != ""
	hasTo := toVersion != ""

	if hasTo && !hasFrom {
		return "", fmt.Errorf("--to-version 需要同时指定 --from-version")
	}
	if hasLocal && (hasFrom || hasTo) {
		return "", fmt.Errorf("--file 与 --from-version/--to-version 互斥：本地比对 vs 远端版本比对只能选一种")
	}
	if hasLocal {
		return "local_vs_remote", nil
	}
	if hasFrom {
		return "remote_vs_remote", nil
	}
	return "", fmt.Errorf("请指定 --from-version（远端版本比对），或 --from-version+--to-version，或 --file（远端最新 vs 本地文件）")
}

// fetchMarkdownVersionContent 下载远端文件指定版本的内容。
// 远端版本下载 = GET /open-apis/drive/v1/files/{file_token}/download?version=N，
// 即同一个 file_token + version 查询参数，不会产生新 token（lark dry-run 实证）。
func fetchMarkdownVersionContent(fileToken, version, userAccessToken string) ([]byte, error) {
	return client.FetchFileVersionContent(fileToken, version, userAccessToken)
}

// markdownDiffHunk unified diff 的一个 hunk。
type markdownDiffHunk struct {
	OldStart int                `json:"old_start"`
	OldLines int                `json:"old_lines"`
	NewStart int                `json:"new_start"`
	NewLines int                `json:"new_lines"`
	Lines    []markdownDiffLine `json:"lines"`
}

// markdownDiffLine hunk 内的一行，Op 为 " "(unchanged) / "-"(removed) / "+"(added)。
type markdownDiffLine struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// maxDiffLines/maxDiffCells diff 大小上限。LCS 用完整 (n+1)×(m+1) int 矩阵，
// 内存随两侧行数【乘积】增长——单侧 5 万行看似不大，但两侧各 5 万行 = 25 亿 int ≈ 10GB OOM。
// 故同时限制单侧行数与乘积单元数（两者都满足才放行）。
const (
	maxDiffLines = 20000    // 单侧行数上限
	maxDiffCells = 20000000 // LCS 矩阵 n*m 乘积上限（约 80MB int 矩阵），防 OOM
)

// checkDiffSize 在计算 LCS 之前拦截过大内容（单侧行数超限，或两侧行数乘积超限）。
func checkDiffSize(a, b string) error {
	na := len(splitDiffLines(a))
	nb := len(splitDiffLines(b))
	if na > maxDiffLines || nb > maxDiffLines {
		return fmt.Errorf("内容过大（%d 行 / %d 行），单侧超过行数上限 %d，建议用外部 diff 工具", na, nb, maxDiffLines)
	}
	if na*nb > maxDiffCells {
		return fmt.Errorf("内容过大（%d × %d 行），LCS 矩阵超过 %d 单元上限（防 OOM），建议用外部 diff 工具", na, nb, maxDiffCells)
	}
	return nil
}

// splitDiffLines 把内容按 \n 拆为行（去掉行尾换行，规整 \r\n）。
func splitDiffLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// editOp LCS 回溯出的逐行编辑操作。
type editOp struct {
	op   byte // ' ' '-' '+'
	text string
}

// diffLines 用 LCS（最长公共子序列）算法对两组行做最小编辑序列。
func diffLines(a, b []string) []editOp {
	n, m := len(a), len(b)
	// lcs[i][j] = a[i:] 与 b[j:] 的最长公共子序列长度
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var ops []editOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, editOp{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, editOp{'-', a[i]})
			i++
		default:
			ops = append(ops, editOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, editOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, editOp{'+', b[j]})
	}
	return ops
}

// unifiedDiff 计算 a -> b 的 unified diff，按 contextLines 切分 hunk。
// 完全相同返回空切片。
func unifiedDiff(a, b string, contextLines int) []markdownDiffHunk {
	ops := diffLines(splitDiffLines(a), splitDiffLines(b))

	// 没有任何 +/- 即无差异
	changed := false
	for _, o := range ops {
		if o.op != ' ' {
			changed = true
			break
		}
	}
	if !changed {
		return nil
	}

	// 标记每个 op 是否属于某个 hunk（变更点及其前后 contextLines 行上下文）
	keep := make([]bool, len(ops))
	for idx, o := range ops {
		if o.op == ' ' {
			continue
		}
		lo := idx - contextLines
		if lo < 0 {
			lo = 0
		}
		hi := idx + contextLines
		if hi >= len(ops) {
			hi = len(ops) - 1
		}
		for k := lo; k <= hi; k++ {
			keep[k] = true
		}
	}

	var hunks []markdownDiffHunk
	oldLine, newLine := 1, 1 // 1-based 行号
	idx := 0
	for idx < len(ops) {
		if !keep[idx] {
			if ops[idx].op != '+' {
				oldLine++
			}
			if ops[idx].op != '-' {
				newLine++
			}
			idx++
			continue
		}
		// 收集一段连续保留区间为一个 hunk
		h := markdownDiffHunk{OldStart: oldLine, NewStart: newLine}
		for idx < len(ops) && keep[idx] {
			o := ops[idx]
			h.Lines = append(h.Lines, markdownDiffLine{Op: string(o.op), Text: o.text})
			switch o.op {
			case ' ':
				h.OldLines++
				h.NewLines++
				oldLine++
				newLine++
			case '-':
				h.OldLines++
				oldLine++
			case '+':
				h.NewLines++
				newLine++
			}
			idx++
		}
		hunks = append(hunks, h)
	}
	return hunks
}

// countDiffLines 统计所有 hunk 内新增/删除行数。
func countDiffLines(hunks []markdownDiffHunk) (added, removed int) {
	for _, h := range hunks {
		for _, l := range h.Lines {
			switch l.Op {
			case "+":
				added++
			case "-":
				removed++
			}
		}
	}
	return added, removed
}

// renderUnifiedDiff 把 hunk 渲染为标准 unified diff 文本（@@ -a,b +c,d @@ + 行）。
func renderUnifiedDiff(hunks []markdownDiffHunk) string {
	var sb strings.Builder
	for _, h := range hunks {
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		for _, l := range h.Lines {
			sb.WriteString(l.Op)
			sb.WriteString(l.Text)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func init() {
	markdownCmd.AddCommand(markdownDiffCmd)
	markdownDiffCmd.Flags().String("file-token", "", "目标 Markdown 文件 token（必填）")
	markdownDiffCmd.Flags().String("file", "", "本地 .md 文件路径（与远端最新内容比对）")
	markdownDiffCmd.Flags().String("from-version", "", "起始远端版本号")
	markdownDiffCmd.Flags().String("to-version", "", "目标远端版本号（需配合 --from-version）")
	markdownDiffCmd.Flags().Int("context-lines", 3, "diff 每个 hunk 上下保留的未变更上下文行数")
	markdownDiffCmd.Flags().Bool("dry-run", false, "只打印比对计划，不下载/不比对")
	markdownDiffCmd.Flags().StringP("output", "o", "", "[兼容] -o json 等价 --format json；缺省输出 unified diff 文本")
	output.AddFormatFlags(markdownDiffCmd) // --format json|pretty|table|ndjson|csv + --jq（与其它新命令一致）
	markdownDiffCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(markdownDiffCmd, "file-token")
}
