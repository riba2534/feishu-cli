package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// Mermaid 复杂度预警阈值。实测服务端已能渲染 par、12 个 participant、3 层嵌套 alt，
// 这里只对明显超出实测范围的规模给出"可能失败"提示，不再把 par 当作不支持。
const (
	mermaidWarnParticipants = 20 // participant / actor 声明数
	mermaidWarnAltBlocks    = 6  // alt 块数
	mermaidWarnLongLines    = 50 // 超过 60 字节的长行数
)

// mermaidComplexityHint 服务端渲染可能失败时的建议：只建议本地引擎（节点仍可编辑），
// 不建议 svg-import（会把整张图变成一个不可编辑的图片节点）。
const mermaidComplexityHint = "服务端渲染可能失败；先照常导入，真遇到 Parse error 再加 --engine local 改用本地引擎（生成的节点仍可编辑）"

// estimateMermaidComplexity 简单估算复杂度，返回非空字符串表示规模超出实测可渲染范围
func estimateMermaidComplexity(source, sourceType string) string {
	content := source
	if sourceType == "" || sourceType == "file" {
		data, err := os.ReadFile(source)
		if err != nil {
			return ""
		}
		content = string(data)
	}
	participantCount, alt, longLines := 0, 0, 0
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "participant ") || strings.HasPrefix(trimmed, "actor ") {
			participantCount++
		}
		if trimmed == "alt" || strings.HasPrefix(trimmed, "alt ") {
			alt++
		}
		if len(line) > 60 {
			longLines++
		}
	}
	var reasons []string
	if participantCount >= mermaidWarnParticipants {
		reasons = append(reasons, fmt.Sprintf("participant 数 %d ≥ %d", participantCount, mermaidWarnParticipants))
	}
	if alt >= mermaidWarnAltBlocks {
		reasons = append(reasons, fmt.Sprintf("alt 块 %d 个 ≥ %d", alt, mermaidWarnAltBlocks))
	}
	if longLines >= mermaidWarnLongLines {
		reasons = append(reasons, fmt.Sprintf("长标签行 %d 个 ≥ %d", longLines, mermaidWarnLongLines))
	}
	return strings.Join(reasons, "，")
}

// mermaidParseErrorHint 服务端引擎返回 Parse error 时追加切换本地引擎的提示（保留原错误分类）。
func mermaidParseErrorHint(err error, syntax string) error {
	if err == nil || syntax != "mermaid" || !strings.Contains(err.Error(), "Parse error") {
		return err
	}
	return fmt.Errorf("%w\n提示: 服务端无法解析该 Mermaid，可加 --engine local 改用本地引擎（需 whiteboard-cli，生成的节点仍可编辑）", err)
}

var importDiagramCmd = &cobra.Command{
	Use:   "import <whiteboard_id> <source>",
	Short: "导入图表到画板（PlantUML / Mermaid / SVG）",
	Long: `将 PlantUML、Mermaid 或 SVG 导入到飞书画板，服务端解析为可编辑的原生画板节点。

参数:
  <whiteboard_id>    画板 ID（必填）
  <source>           图表代码或文件路径（必填）
  --source-type      源类型：file/content，默认 file
  --syntax           图表语法：plantuml/mermaid/svg，默认 plantuml（未知取值直接报错）
  --diagram-type     图表类型：auto/mindmap/sequence/activity/class/er/flowchart/state/component，默认 auto
  --style            样式类型：board/classic，默认 board
  --engine           server（飞书服务端解析，默认）/ local（whiteboard-cli 本地转换后建节点）
  --client-token     幂等键（≥10 字符，仅 --engine local 生效）
  --output, -o       输出格式 (json)

图表语法（syntax_type）:
  plantuml    PlantUML（1，默认）
  mermaid     Mermaid（2）
  svg         SVG（3）：服务端把 rect/circle/ellipse/text/line/path/polygon 等拆成可编辑节点，
              渐变 fill=url(#id)、自定义虚线等不支持的属性会降级并在输出 degraded_attributes 中列出；
              --diagram-type / --style 只作用于 PlantUML/Mermaid，SVG 请求不发送这两个参数

样式类型:
  board       画板风格（默认）
  classic     经典风格

重试与幂等:
  服务端引擎遇到限流 / 5xx 会自动重试。该接口不认 client_token（实测同一 token 重复请求会重复建图），
  因此未带 --overwrite 时，CLI 在首个请求前记录画板已有的顶层节点，重试前先回读：
  若上一次请求其实已落地（只是响应失败），直接返回新节点而不再重复提交。

示例:
  # 从文件导入 PlantUML 图表
  feishu-cli board import <whiteboard_id> diagram.puml

  # 导入 Mermaid 图表
  feishu-cli board import <whiteboard_id> diagram.mmd --syntax mermaid

  # 导入 SVG（服务端解析为可编辑节点）
  feishu-cli board import <whiteboard_id> drawing.svg --syntax svg

  # 直接导入图表代码
  feishu-cli board import <whiteboard_id> "@startuml\nA -> B: hello\n@enduml" --source-type content

  # 使用经典样式
  feishu-cli board import <whiteboard_id> diagram.puml --style classic

  # 预览请求（不调用 API）
  feishu-cli board import <whiteboard_id> drawing.svg --syntax svg --dry-run -o json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		whiteboardID := args[0]
		source := args[1]
		sourceType, _ := cmd.Flags().GetString("source-type")
		syntax, _ := cmd.Flags().GetString("syntax")
		diagramType, _ := cmd.Flags().GetString("diagram-type")
		style, _ := cmd.Flags().GetString("style")
		parseMode, _ := cmd.Flags().GetInt("parse-mode")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		engine, _ := cmd.Flags().GetString("engine")
		clientToken, _ := cmd.Flags().GetString("client-token")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")
		syntax = strings.ToLower(strings.TrimSpace(syntax))
		clientToken = strings.TrimSpace(clientToken)

		opts := client.ImportDiagramOptions{
			SourceType:  sourceType,
			Syntax:      syntax,
			DiagramType: diagramType,
			Style:       style,
			ParseMode:   parseMode,
			Overwrite:   overwrite,
		}
		// 枚举白名单：过去未知 --syntax（含 svg）会被静默当成 PlantUML 发出去
		if err := client.ValidateImportDiagramOptions(opts); err != nil {
			return clierr.Usage(err)
		}
		if sourceType != "" && sourceType != "file" && sourceType != "content" {
			return clierr.Usagef("--source-type 取值 %q 不受支持，可选值: file / content", sourceType)
		}
		if engine != "server" && engine != "local" {
			return clierr.Usagef("--engine 取值 %q 不受支持，可选值: server / local", engine)
		}
		if err := client.ValidateBoardClientToken(clientToken); err != nil {
			return clierr.Usage(err)
		}
		if clientToken != "" && engine != "local" {
			return clierr.Usagef("--client-token 仅对 --engine local 生效：服务端引擎接口（/nodes/plantuml）不认 client_token，重复请求仍会重复建图；服务端引擎的重试去重由 CLI 自动处理")
		}

		// 复杂度估算 + 提示（仅 Mermaid，server engine）
		if syntax == "mermaid" && engine != "local" {
			if warn := estimateMermaidComplexity(source, sourceType); warn != "" {
				fmt.Fprintf(os.Stderr, "⚠ Mermaid 复杂度提示: %s\n  %s\n", warn, mermaidComplexityHint)
			}
		}

		if dryRun {
			syntaxType, _ := client.BoardSyntaxType(syntax)
			r := map[string]any{
				"whiteboard_id": whiteboardID,
				"source_type":   sourceType,
				"syntax":        syntax,
				"syntax_type":   syntaxType,
				"engine":        engine,
				"style":         style,
				"diagram_type":  diagramType,
				"parse_mode":    parseMode,
				"overwrite":     overwrite,
				"dry_run":       true,
			}
			if engine == "server" {
				// 请求体预览：图表源码只给长度，避免把大段代码打到终端
				body, err := client.BuildImportDiagramBody("", opts)
				if err != nil {
					return clierr.Usage(err)
				}
				body["plant_uml_code"] = "<diagram source>"
				r["request"] = map[string]any{
					"method": "POST",
					"path":   fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/plantuml", whiteboardID),
					"body":   body,
				}
			} else if clientToken != "" {
				r["client_token"] = clientToken
			}
			if output == "json" {
				return printJSON(r)
			}
			fmt.Printf("[dry-run] board import 将调用 %s 引擎 syntax=%s(syntax_type=%d) parse_mode=%d overwrite=%v\n", engine, syntax, syntaxType, parseMode, overwrite)
			return nil
		}

		userAccessToken := resolveOptionalUserToken(cmd)
		opts.UserAccessToken = userAccessToken

		// 本地引擎路径：通过 whiteboard-cli 把 Mermaid/DSL 转节点 JSON 再 create_nodes
		// 适合服务端引擎返回 Parse error 的复杂 Mermaid
		if engine == "local" {
			if !client.WhiteboardCLIBridgeAvailable() {
				return fmt.Errorf("--engine local 需要 whiteboard-cli。安装：npm install -g @larksuite/whiteboard-cli")
			}
			asFile := (sourceType == "" || sourceType == "file")
			nodesJSON, err := client.RenderDiagramToOpenAPINodes(source, syntax, asFile)
			if err != nil {
				return fmt.Errorf("本地引擎转换失败: %w", err)
			}
			nodeIDs, err := client.CreateBoardNodes(whiteboardID, nodesJSON, client.CreateBoardNotesOptions{
				UserAccessToken: userAccessToken,
				Overwrite:       overwrite,
				ClientToken:     clientToken,
			})
			if err != nil {
				return err
			}
			if output == "json" {
				out := map[string]any{
					"whiteboard_id": whiteboardID,
					"engine":        "local",
					"syntax":        syntax,
					"node_count":    len(nodeIDs),
					"node_ids":      nodeIDs,
					"overwrite":     overwrite,
				}
				if clientToken != "" {
					out["client_token"] = clientToken
				}
				return printJSON(out)
			}
			fmt.Printf("图表导入成功（本地引擎）！\n  画板 ID: %s\n  创建节点数: %d\n  语法: %s\n",
				whiteboardID, len(nodeIDs), syntax)
			return nil
		}

		// 未覆盖写入时记录画板已有顶层节点：/nodes/plantuml 不认 client_token，
		// 重试前回读，若上一次请求其实已落地则直接返回新节点，避免重复建图。
		// 读取失败（无读权限等）不影响导入，只是退回无去重的重试。
		var baseline map[string]bool
		if !overwrite {
			if ids, err := boardTopLevelNodeIDs(whiteboardID, userAccessToken); err == nil {
				baseline = ids
			}
		}

		attempt := 0
		retryResult := client.DoWithRetry(func() (*client.ImportDiagramResult, http.Header, error) {
			attempt++
			if attempt > 1 && baseline != nil {
				if id := firstNewTopLevelNode(whiteboardID, userAccessToken, baseline); id != "" {
					fmt.Fprintf(os.Stderr, "  上一次请求已落地（检测到新节点 %s），不再重复提交\n", id)
					return &client.ImportDiagramResult{TicketID: id}, nil, nil
				}
			}
			return client.ImportDiagram(whiteboardID, source, opts)
		}, client.RetryConfig{
			MaxRetries:       5,
			RetryOnRateLimit: true,
			OnRetry: func(attempt int, err error, wait time.Duration) {
				fmt.Fprintf(os.Stderr, "  ⚠ 图表导入重试 %d/5 (等待 %.1fs): %v\n", attempt, wait.Seconds(), err)
			},
		})
		if retryResult.Err != nil {
			return mermaidParseErrorHint(retryResult.Err, syntax)
		}
		result := retryResult.Value

		if output == "json" {
			out := map[string]any{
				"whiteboard_id": whiteboardID,
				"ticket_id":     result.TicketID,
				"syntax":        syntax,
				"style":         style,
			}
			if len(result.DegradedAttributes) > 0 {
				out["degraded_attributes"] = result.DegradedAttributes
			}
			return printJSON(out)
		}
		fmt.Printf("图表导入成功！\n")
		fmt.Printf("  画板 ID: %s\n", whiteboardID)
		if result.TicketID != "" {
			fmt.Printf("  票据 ID: %s\n", result.TicketID)
		}
		fmt.Printf("  语法: %s\n", syntax)
		if syntax != "svg" {
			fmt.Printf("  样式: %s\n", style)
		}
		if len(result.DegradedAttributes) > 0 {
			fmt.Fprintf(os.Stderr, "⚠ 服务端降级了 %d 处不支持的属性（画面可能与源码有出入）:\n", len(result.DegradedAttributes))
			for _, a := range result.DegradedAttributes {
				fmt.Fprintf(os.Stderr, "  - %s\n", a)
			}
		}
		return nil
	},
}

// boardTopLevelNodeIDs 读取画板顶层节点（parent_id 为空）ID 集合。
func boardTopLevelNodeIDs(whiteboardID, userAccessToken string) (map[string]bool, error) {
	raw, err := client.GetBoardNodes(whiteboardID, userAccessToken)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Nodes json.RawMessage `json:"nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	nodes, err := parseBoardExportNodes(resp.Data.Nodes)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if parent, _ := n["parent_id"].(string); parent != "" {
			continue
		}
		if id, _ := n["id"].(string); id != "" {
			ids[id] = true
		}
	}
	return ids, nil
}

// firstNewTopLevelNode 返回相对 baseline 新出现的顶层节点 ID（按 ID 排序取第一个）；没有或读取失败返回 ""。
func firstNewTopLevelNode(whiteboardID, userAccessToken string, baseline map[string]bool) string {
	current, err := boardTopLevelNodeIDs(whiteboardID, userAccessToken)
	if err != nil {
		return ""
	}
	var fresh []string
	for id := range current {
		if !baseline[id] {
			fresh = append(fresh, id)
		}
	}
	if len(fresh) == 0 {
		return ""
	}
	sort.Strings(fresh)
	return fresh[0]
}

func init() {
	boardCmd.AddCommand(importDiagramCmd)
	importDiagramCmd.Flags().String("source-type", "file", "源类型 (file/content)")
	importDiagramCmd.Flags().String("syntax", "plantuml", "图表语法 (plantuml/mermaid/svg)")
	importDiagramCmd.Flags().String("diagram-type", "auto", "图表类型 (auto/mindmap/sequence/activity/class/er/flowchart/state/component)")
	importDiagramCmd.Flags().String("style", "board", "样式类型 (board/classic)")
	importDiagramCmd.Flags().Int("parse-mode", 1, "解析模式 (默认 1)")
	importDiagramCmd.Flags().Bool("overwrite", false, "是否覆盖画板已有内容（服务端 overwrite: true）")
	importDiagramCmd.Flags().String("engine", "server", "渲染引擎 (server=飞书服务端 / local=whiteboard-cli 本地转换)")
	importDiagramCmd.Flags().String("client-token", "", "幂等键（≥10 字符，仅 --engine local 生效；结果未知时用同一个值重跑不会重复建节点）")
	importDiagramCmd.Flags().Bool("dry-run", false, "预览不调用 API")
	importDiagramCmd.Flags().String("user-access-token", "", "User Access Token")
	importDiagramCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
