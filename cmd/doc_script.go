// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.）
// SPDX-License-Identifier: MIT

package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/docxparse"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

// doc script：AI 写文档工作流的辅助脚本（对齐官方 lark-cli docs +script）。
//
//   - init-draft：校验 Presentation Decision，在当前目录创建独占工作区 draft_<8 位十六进制>_folder/，
//     保存 .presentation-decision.json 作为草稿基线，只预留（不创建）draft.xml；
//   - parse：解析本地 DocxXML（--content）或在线文档（--doc，经 docs_ai fetch 取 XML），输出字数 / 块分布画像；
//     加载到 Presentation Decision 时检查字数与块数量约束，并预检本地 / 远程资源。
//
// parse 的通过与否看 assessment.status；画像、决策或资源检查未通过时命令仍以退出码 0 返回。

const (
	docScriptParse              = "parse"
	docScriptInitDraft          = "init-draft"
	docScriptDraftDirPattern    = "draft_*_folder"
	docScriptDraftDirPrefix     = "draft_"
	docScriptDraftDirSuffix     = "_folder"
	docScriptDraftXMLFileName   = "draft.xml"
	docScriptDraftRandomHexLen  = 8
	docScriptDecisionFile       = ".presentation-decision.json"
	docScriptListBlockType      = "list"
	docScriptAssessmentPassed   = "passed"
	docScriptAssessmentFailed   = "failed"
	docScriptDiagnosticError    = "error"
	docScriptCodeWordCountRange = "word_count_out_of_range"
	docScriptCodeRequiredBlock  = "required_block_missing"
	docScriptCodeResourceCheck  = "resource_preflight_failed"
	docScriptCodeImageSource    = "remote_image_source_disallowed"
	docScriptCodeImageMissing   = "remote_image_unavailable"
	docScriptCodeImageFormat    = "remote_image_format_unsupported"
	docScriptCodeImageTooLarge  = "remote_image_too_large"
	docScriptCodeImagePreflight = "remote_image_preflight_failed"

	docScriptDraftTip = "工作区已创建，草稿 XML 尚不存在：直接写入 <cwd>/<draft_path>（首次写入前不要读取该路径）。" +
		"新资源优先放在工作区内并复用已有文件；cwd 内用 @相对路径，其他目录用 @绝对路径（敏感目录会被拒绝）。" +
		"XML 中的相对资源路径先查 cwd，仅文件不存在时再查源 XML 所在目录。后续 CLI 命令都在 cwd 下执行。"
	docScriptDecisionShellHint = "还原原始 JSON 引号；若 shell 吞掉引号导致字符串有歧义，把原始 JSON 以 UTF-8 保存到文件，" +
		"改用 --presentation-decision \"@./decision.json\""
)

var docScriptCmd = &cobra.Command{
	Use:   "script",
	Short: "写作辅助：初始化草稿工作区 / 解析文档画像与预检",
	Long: `AI 写文档工作流的辅助脚本（对齐官方 docs +script），通过 --command 选择：

  init-draft  校验 Presentation Decision（--presentation-decision 必填），在当前目录创建独占工作区
              draft_<8 位十六进制>_folder/，保存 .presentation-decision.json 作为草稿基线，
              并预留尚不存在的 draft.xml 路径。输出 cwd / workspace / draft_path / tip。
  parse       解析本地 DocxXML（--content）或在线文档（--doc，docs_ai fetch 取 XML），输出
              assessment.status、profile（word_count / char_count / block_count / blocks[]）
              和按需出现的 diagnostics[]。

Presentation Decision 是单个 JSON 对象，无约束时传 {}：
  word_count            {"min": N|null, "max": N|null}，仅在有字数要求时填写（至少一侧为正整数，min <= max）
  visual_plan.blocks[]  {"type": "img|whiteboard|html5-block|table|...|list", "min_count": N, "purpose": "..."}
                        type 与 min_count 都填写时才参与数量检查；同一 type 不能重复；list 按 ul + ol 合计
  audience / reader_task / genre_contract / adapter / presentation_mode / visual_plan.reason / purpose
                        是可选描述信息，可省略、为空或 null，不参与判定

parse 规则:
  - --content 与 --doc 二选一；--content 支持字面 XML、@文件路径、-（stdin，@@ 转义字面 @），不支持 Markdown。
  - 用 --content "@./<draft_path>" 时自动加载同目录的 .presentation-decision.json；显式 --presentation-decision 优先。
  - 有 Presentation Decision 时检查字数、块数量，并预检资源：<img path> / <source path> / <whiteboard path> /
    <html5-block path> 指向的本地文件，以及 <img href> 远程图片（Range 探测，会联网）。
  - 检查未通过时退出码仍为 0，assessment.status 为 failed，按 diagnostics[].code / suggested 修复后重新解析。
  - parse 不是 schema 校验器：passed 不保证服务端接受，写入结果以 doc create / content-update 为准。
  - --doc 需要 docx:document:readonly；--content 不调用 OpenAPI。

示例:
  feishu-cli doc script --command init-draft --presentation-decision '{}'
  feishu-cli doc script --command init-draft --presentation-decision "@./decision.json"
  feishu-cli doc script --command parse --content "@./draft_1a2b3c4d_folder/draft.xml"
  feishu-cli doc script --command parse --doc "https://xxx.feishu.cn/docx/doxcnxxx" --as user
  feishu-cli doc script --command parse --content "@./doc.xml" --presentation-decision '{"word_count":{"min":800,"max":null}}'

  @文件路径加引号，避免 PowerShell 把 @ 当作展开运算符。`,
	Args: cobra.NoArgs,
	RunE: runDocScript,
}

func init() {
	docCmd.AddCommand(docScriptCmd)
	f := docScriptCmd.Flags()
	f.String("command", "", "脚本: init-draft | parse（必填）")
	f.String("content", "", "parse: 本地 XML 内容；@文件路径 读文件，- 读 stdin，@@ 转义字面 @；与 --doc 互斥")
	f.String("doc", "", "parse: 在线文档 URL 或 token（docx / wiki）；与 --content 互斥")
	f.String("presentation-decision", "", "Presentation Decision JSON：init-draft 必填并保存为草稿基线，parse 可选；内联 JSON / @文件路径 / -（stdin）")
	addAsFlag(docScriptCmd)
	f.String("user-access-token", "", "User Access Token（可选，--doc 时使用）")
	output.AddDryRunFlag(docScriptCmd)
	output.AddFormatFlags(docScriptCmd)
	mustMarkFlagRequired(docScriptCmd, "command")
}

// ---------- 输出结构 ----------

type docScriptParseResult struct {
	Assessment  docScriptAssessment    `json:"assessment"`
	Profile     docScriptPublicProfile `json:"profile"`
	Diagnostics []docScriptDiagnostic  `json:"diagnostics,omitempty"`
}

type docScriptAssessment struct {
	Status string `json:"status"`
}

type docScriptDiagnostic struct {
	Severity     string                          `json:"severity"`
	Code         string                          `json:"code"`
	Expected     *docScriptDiagnosticExpectation `json:"expected,omitempty"`
	Actual       *int                            `json:"actual,omitempty"`
	ImageIndices []int                           `json:"image_indices,omitempty"`
	Msg          string                          `json:"msg"`
	Suggested    string                          `json:"suggested,omitempty"`
}

type docScriptDiagnosticExpectation struct {
	Type     string `json:"type,omitempty"`
	MinCount int    `json:"min_count,omitempty"`
	Min      *int   `json:"min,omitempty"`
	Max      *int   `json:"max,omitempty"`
}

// docScriptPublicProfile 是对外稳定的画像；解析器内部的 breakdown 暂不暴露。
type docScriptPublicProfile struct {
	WordCount  int                    `json:"word_count"`
	CharCount  int                    `json:"char_count"`
	BlockCount int                    `json:"block_count"`
	Blocks     []docxparse.BlockShare `json:"blocks"`
}

type docScriptDecision struct {
	Audience         string                   `json:"audience"`
	ReaderTask       string                   `json:"reader_task"`
	GenreContract    *string                  `json:"genre_contract"`
	Adapter          *string                  `json:"adapter"`
	PresentationMode string                   `json:"presentation_mode"`
	WordCount        *docScriptDecisionWords  `json:"word_count"`
	VisualPlan       docScriptDecisionVisuals `json:"visual_plan"`
}

type docScriptDecisionWords struct {
	Min *int `json:"min"`
	Max *int `json:"max"`
}

type docScriptDecisionVisuals struct {
	Reason string                    `json:"reason"`
	Blocks []docScriptDecisionBlocks `json:"blocks"`
}

type docScriptDecisionBlocks struct {
	Type     *string `json:"type"`
	MinCount *int    `json:"min_count"`
	Purpose  string  `json:"purpose"`
}

type docScriptDraftResult struct {
	CWD       string `json:"cwd"`
	Workspace string `json:"workspace"`
	DraftPath string `json:"draft_path"`
	Tip       string `json:"tip"`
}

// ---------- 输入解析 ----------

// docScriptStdin 供测试替换。
var docScriptStdin io.Reader = os.Stdin

// docScriptInput 是一个支持 字面值 / @文件 / -（stdin）的 flag 的解析结果。
type docScriptInput struct {
	value      string
	path       string // 来自 @文件 时的路径（已 trim）
	fromSource bool   // 来自 @文件 或 stdin（原始字节，不做 shell 引号恢复）
}

func readDocScriptInput(cmd *cobra.Command, name string) (docScriptInput, error) {
	raw, _ := cmd.Flags().GetString(name)
	flag := "--" + name
	switch {
	case raw == "-":
		data, err := io.ReadAll(io.LimitReader(docScriptStdin, docxparse.MaxInputBytes+1))
		if err != nil {
			return docScriptInput{}, fmt.Errorf("%s 读取标准输入失败: %w", flag, err)
		}
		if len(data) > docxparse.MaxInputBytes {
			return docScriptInput{}, clierr.Usagef("%s: 标准输入超过 %d 字节上限", flag, docxparse.MaxInputBytes)
		}
		return docScriptInput{value: stripUTF8BOM(string(data)), fromSource: true}, nil
	case strings.HasPrefix(raw, "@@"):
		return docScriptInput{value: raw[1:]}, nil
	case strings.HasPrefix(raw, "@"):
		path := strings.TrimSpace(raw[1:])
		if path == "" {
			return docScriptInput{}, clierr.Usagef("%s: @ 后面的文件路径不能为空", flag)
		}
		info, err := safefile.StatInputFile(path)
		if err != nil {
			return docScriptInput{}, clierr.Usage(fmt.Errorf("%s: %w", flag, err))
		}
		if info.Size() > docxparse.MaxInputBytes {
			return docScriptInput{}, clierr.Usagef("%s: 文件 %s 超过 %d 字节上限", flag, path, docxparse.MaxInputBytes)
		}
		data, err := readLocalInputFile(path)
		if err != nil {
			return docScriptInput{}, clierr.Usage(fmt.Errorf("%s: %w", flag, err))
		}
		return docScriptInput{value: stripUTF8BOM(string(data)), path: path, fromSource: true}, nil
	default:
		return docScriptInput{value: raw}, nil
	}
}

// ---------- 主流程 ----------

func runDocScript(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	command, _ := f.GetString("command")
	command = strings.TrimSpace(command)
	if command != docScriptInitDraft && command != docScriptParse {
		return clierr.Usagef("不支持的 --command %q，可选 init-draft / parse", command)
	}
	opts, err := output.ParseOptions(cmd)
	if err != nil {
		return clierr.Usage(err)
	}
	if err := output.ValidateJQ(opts.JQ); err != nil {
		return clierr.Usage(err)
	}
	if err := validateIdentityAs(cmd); err != nil {
		return err
	}
	rawContent, _ := f.GetString("content")
	rawDecision, _ := f.GetString("presentation-decision")
	doc, _ := f.GetString("doc")
	doc = strings.TrimSpace(doc)
	if rawContent == "-" && rawDecision == "-" {
		return clierr.Usagef("--content 与 --presentation-decision 最多只能有一个读取标准输入（-）")
	}

	if command == docScriptInitDraft {
		switch {
		case strings.TrimSpace(rawContent) != "":
			return clierr.Usagef("--command init-draft 不支持 --content")
		case doc != "":
			return clierr.Usagef("--command init-draft 不支持 --doc")
		case strings.TrimSpace(rawDecision) == "":
			return clierr.Usagef("--command init-draft 需要 --presentation-decision（无约束时传 '{}'）")
		}
		decisionInput, err := readDocScriptInput(cmd, "presentation-decision")
		if err != nil {
			return err
		}
		_, normalized, err := parseDocScriptDecisionInput(decisionInput)
		if err != nil {
			return err
		}
		if opts.DryRun {
			return output.Render(opts, map[string]any{
				"dry_run":               true,
				"desc":                  "创建独占草稿工作区并保存 Presentation Decision，只预留 XML 路径不创建文件；不发起任何 API 请求",
				"command":               docScriptInitDraft,
				"directory_pattern":     docScriptDraftDirPattern,
				"xml_file_name":         docScriptDraftXMLFileName,
				"creates_workspace":     true,
				"creates_draft_file":    false,
				"presentation_decision": true,
				"network":               false,
			})
		}
		result, err := initDocScriptDraft(normalized)
		if err != nil {
			return err
		}
		if err := output.Render(opts, result); err != nil {
			// 输出失败时清理本次创建的工作区，避免留下无人知晓的目录
			if cleanupErr := removeDocScriptWorkspace(result.DraftPath); cleanupErr != nil {
				return errors.Join(err, cleanupErr)
			}
			return err
		}
		return nil
	}

	// parse
	hasContent := strings.TrimSpace(rawContent) != ""
	if !hasContent && doc == "" {
		return clierr.Usagef("--command parse 需要 --content（本地 XML）或 --doc（在线文档 URL / token）其中之一")
	}
	if hasContent && doc != "" {
		return clierr.Usagef("--content 与 --doc 互斥，只能使用其中一个")
	}
	var docRef *resolvedResource
	if doc != "" {
		docRef, err = parseResourceArg(doc, resourceArgOptions{
			ArgName:     "--doc",
			DefaultType: client.ResourceTypeDocx,
			Allowed:     []string{client.ResourceTypeDocx},
			ResolveWiki: true,
		})
		if err != nil {
			return clierr.Usage(err)
		}
	}
	var contentInput docScriptInput
	if hasContent {
		contentInput, err = readDocScriptInput(cmd, "content")
		if err != nil {
			return err
		}
	}
	decision, hasDecision, err := resolveDocScriptDecision(cmd, rawDecision, contentInput.path)
	if err != nil {
		return err
	}

	if opts.DryRun {
		return output.Render(opts, dryRunDocScriptParse(docRef, contentInput, hasDecision))
	}

	content := contentInput.value
	inputLabel := " --content "
	if docRef != nil {
		content, err = fetchDocScriptContent(cmd, docRef)
		if err != nil {
			return err
		}
		inputLabel = "在线文档内容"
	}
	profile, err := docxparse.ParseCompatibleXML(content)
	if err != nil {
		// 与官方一致：本地与在线内容解析失败都归为校验错误（退出码 2）
		return clierr.Usage(fmt.Errorf("无法把%s解析为 DocxXML: %w", inputLabel, err))
	}
	publicProfile := docScriptPublicProfile{
		WordCount:  profile.WordCount,
		CharCount:  profile.CharCount,
		BlockCount: profile.BlockCount,
		Blocks:     profile.Blocks,
	}
	var diagnostics []docScriptDiagnostic
	if hasDecision {
		diagnostics = docScriptPresentationDiagnostics(publicProfile, decision)
		diagnostics = append(diagnostics, docScriptResourceDiagnostics(content, contentInput.path)...)
	}
	status := docScriptAssessmentPassed
	if len(diagnostics) > 0 {
		status = docScriptAssessmentFailed
	}
	return output.Render(opts, docScriptParseResult{
		Assessment:  docScriptAssessment{Status: status},
		Profile:     publicProfile,
		Diagnostics: diagnostics,
	})
}

func dryRunDocScriptParse(docRef *resolvedResource, contentInput docScriptInput, hasDecision bool) map[string]any {
	if docRef != nil {
		docID := docRef.Token
		desc := "OpenAPI: 读取文档 XML 后解析画像"
		if docRef.InputType == client.ResourceTypeWiki {
			docID = "<wiki 解析后的 docx token>"
			desc = "先经 wiki node_by_token 解析出底层 docx，再读取文档 XML 后解析画像"
		}
		plan := map[string]any{
			"dry_run": true,
			"desc":    desc,
			"command": docScriptParse,
			"api": []dryRunStep{{
				Method: "POST",
				URL:    fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", docID),
				Body:   docScriptFetchBody(),
			}},
			"document_id": docRef.Token,
			"network":     true,
		}
		if hasDecision {
			plan["presentation_decision"] = true
		}
		return plan
	}
	network := hasDecision && docScriptHasRemoteImages(contentInput.value)
	desc := "本地 DocxXML 解析，不调用 OpenAPI"
	if network {
		desc = "本地 DocxXML 解析；远程图片可用性预检会发起 Range 探测请求（不缓存图片内容）"
	}
	plan := map[string]any{
		"dry_run":     true,
		"desc":        desc,
		"command":     docScriptParse,
		"input_bytes": len(contentInput.value),
		"network":     network,
	}
	if hasDecision {
		plan["presentation_decision"] = true
	}
	return plan
}

// docScriptFetchBody 与官方一致：取 XML，不导出 block id / 样式 / 引用附加数据。
func docScriptFetchBody() map[string]any {
	return map[string]any{
		"format":      "xml",
		"extra_param": docsFetchExtraParam,
		"export_option": map[string]any{
			"export_block_id":        false,
			"export_style_attrs":     false,
			"export_cite_extra_data": false,
		},
	}
}

func fetchDocScriptContent(cmd *cobra.Command, docRef *resolvedResource) (string, error) {
	if err := config.Validate(); err != nil {
		return "", err
	}
	userAccessToken, err := resolveIdentityToken(cmd)
	if err != nil {
		return "", err
	}
	res := *docRef
	if err := resolveWikiInResource(&res, resourceArgOptions{
		ArgName:         "--doc",
		Allowed:         []string{client.ResourceTypeDocx},
		ResolveWiki:     true,
		UserAccessToken: userAccessToken,
	}); err != nil {
		return "", err
	}
	noteWikiResolved(&res)
	data, err := client.FetchDocsAI(res.Token, docScriptFetchBody(), userAccessToken)
	if err != nil {
		return "", err
	}
	document, ok := data["document"].(map[string]any)
	if !ok || document == nil {
		return "", fmt.Errorf("读取文档响应缺少 document 字段")
	}
	content, ok := document["content"].(string)
	if !ok {
		return "", fmt.Errorf("读取文档响应缺少 document.content 字段")
	}
	return content, nil
}

// ---------- Presentation Decision ----------

// resolveDocScriptDecision 返回显式传入的决策；未传时，若 --content 来自 @文件，则加载同目录保存的决策。
func resolveDocScriptDecision(cmd *cobra.Command, rawDecision, contentPath string) (docScriptDecision, bool, error) {
	if strings.TrimSpace(rawDecision) != "" {
		input, err := readDocScriptInput(cmd, "presentation-decision")
		if err != nil {
			return docScriptDecision{}, false, err
		}
		decision, _, err := parseDocScriptDecisionInput(input)
		return decision, err == nil, err
	}
	if contentPath == "" {
		return docScriptDecision{}, false, nil
	}
	savedPath := filepath.Join(filepath.Dir(contentPath), docScriptDecisionFile)
	if _, err := os.Stat(savedPath); errors.Is(err, fs.ErrNotExist) {
		return docScriptDecision{}, false, nil
	}
	raw, err := readLocalInputFile(savedPath)
	if err != nil {
		return docScriptDecision{}, false, fmt.Errorf("读取已保存的 Presentation Decision 失败: %w", err)
	}
	decision, err := parseDocScriptDecision(stripUTF8BOM(string(raw)))
	if err != nil {
		// 与官方一致：已保存的决策损坏归为校验错误（退出码 2），修复方式是重新 init-draft 而不是重试
		return docScriptDecision{}, false, clierr.Usage(fmt.Errorf("已保存的 Presentation Decision（%s）无效，请勿手动修改该文件，重新执行 init-draft: %w",
			savedPath, err))
	}
	return decision, true, nil
}

// parseDocScriptDecisionInput 严格解析决策；直接内联输入失败时，恢复被 Windows 命令行垫片保留的外层单引号，
// 以及 PowerShell 5.x 吞掉引号但可按 schema 无歧义重建的 JSON。返回规范化后的原始 JSON（用于落盘）。
func parseDocScriptDecisionInput(input docScriptInput) (docScriptDecision, string, error) {
	raw := strings.TrimSpace(input.value)
	decision, err := parseDocScriptDecision(raw)
	if err == nil || input.fromSource {
		return decision, raw, err
	}
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
		decision, err = parseDocScriptDecision(raw)
		if err == nil {
			return decision, raw, nil
		}
	}
	if docScriptDecisionLooksShellMangled(raw) {
		normalized, recoveryErr := recoverDocScriptDecisionJSON(raw)
		if recoveryErr == nil {
			decision, err = parseDocScriptDecision(normalized)
			if err == nil {
				return decision, normalized, nil
			}
			return docScriptDecision{}, normalized, err
		}
		err = fmt.Errorf("%w；提示: %s", err, docScriptDecisionShellHint)
	}
	return docScriptDecision{}, raw, err
}

func docScriptDecisionError(format string, a ...any) error {
	return clierr.Usagef("--presentation-decision "+format, a...)
}

// parseDocScriptDecision 严格解析并校验 Presentation Decision（未知字段、多余 JSON 值都拒绝）。
func parseDocScriptDecision(raw string) (docScriptDecision, error) {
	var decision docScriptDecision
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decision); err != nil {
		return docScriptDecision{}, clierr.Usage(fmt.Errorf("--presentation-decision 必须是合法的 Presentation Decision JSON 对象: %w", err))
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return docScriptDecision{}, docScriptDecisionError("只能包含一个 JSON 对象，当前提供了多个 JSON 值")
		}
		return docScriptDecision{}, clierr.Usage(fmt.Errorf("--presentation-decision 只能包含一个 JSON 对象: %w", err))
	}
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rawFields); err != nil {
		return docScriptDecision{}, clierr.Usage(fmt.Errorf("--presentation-decision 必须是合法的 Presentation Decision JSON 对象: %w", err))
	}
	if rawFields == nil {
		return docScriptDecision{}, docScriptDecisionError("必须是 JSON 对象；没有任何约束时传 {}")
	}
	if rawWordCount, ok := rawFields["word_count"]; ok {
		if strings.TrimSpace(string(rawWordCount)) == "null" {
			return docScriptDecision{}, docScriptDecisionError("未要求字数时应省略 word_count 字段，不要写 null（只在用户提出字数要求时填写）")
		}
		if decision.WordCount == nil {
			return docScriptDecision{}, docScriptDecisionError("word_count 必须是包含 min 与 max 的对象")
		}
		var wordCountFields map[string]json.RawMessage
		if err := json.Unmarshal(rawWordCount, &wordCountFields); err != nil {
			return docScriptDecision{}, docScriptDecisionError("word_count 必须是包含 min 与 max 的对象")
		}
		for _, field := range []string{"min", "max"} {
			if _, ok := wordCountFields[field]; !ok {
				return docScriptDecision{}, docScriptDecisionError("缺少 word_count.%s；未限制的一侧写 null", field)
			}
		}
		wc := decision.WordCount
		switch {
		case wc.Min == nil && wc.Max == nil:
			return docScriptDecision{}, docScriptDecisionError("word_count 至少要设置 min 或 max 之一；没有字数要求时省略整个 word_count")
		case wc.Min != nil && *wc.Min <= 0:
			return docScriptDecision{}, docScriptDecisionError("word_count.min 必须是正整数")
		case wc.Max != nil && *wc.Max <= 0:
			return docScriptDecision{}, docScriptDecisionError("word_count.max 必须是正整数")
		case wc.Min != nil && wc.Max != nil && *wc.Min > *wc.Max:
			return docScriptDecision{}, docScriptDecisionError("word_count.min 不能大于 word_count.max")
		}
	}
	seen := make(map[string]struct{}, len(decision.VisualPlan.Blocks))
	for i := range decision.VisualPlan.Blocks {
		requirement := &decision.VisualPlan.Blocks[i]
		if requirement.Type != nil {
			blockType := strings.TrimSpace(*requirement.Type)
			if blockType != docScriptListBlockType && !docxparse.IsPresentationBlockType(blockType) {
				return docScriptDecision{}, docScriptDecisionError("visual_plan.blocks[%d].type %q 不是支持的展示块类型", i, blockType)
			}
			requirement.Type = &blockType
		}
		if requirement.MinCount != nil && *requirement.MinCount <= 0 {
			return docScriptDecision{}, docScriptDecisionError("visual_plan.blocks[%d].min_count 必须是正整数", i)
		}
		if requirement.Type == nil || requirement.MinCount == nil {
			continue
		}
		if _, exists := seen[*requirement.Type]; exists {
			return docScriptDecision{}, docScriptDecisionError("visual_plan.blocks 重复声明了类型 %q，请合并为一条最低数量", *requirement.Type)
		}
		seen[*requirement.Type] = struct{}{}
	}
	return decision, nil
}

// docScriptDecisionLooksShellMangled 判断内联决策是否像被 shell 吞掉引号（键或值缺引号）。
func docScriptDecisionLooksShellMangled(raw string) bool {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '{' {
		return false
	}
	body := strings.TrimSpace(raw[1:])
	if body == "" || body[0] == '}' {
		return false
	}
	if body[0] != '"' {
		return true
	}
	// PowerShell 可能保留键的引号而去掉标量值的引号；只有 schema 能无歧义重建时才视为可恢复
	normalized, err := recoverDocScriptDecisionJSON(raw)
	return err == nil && normalized != raw
}

// docScriptPresentationDiagnostics 按决策检查字数区间（闭区间）与展示块最低数量。
func docScriptPresentationDiagnostics(profile docScriptPublicProfile, decision docScriptDecision) []docScriptDiagnostic {
	var diagnostics []docScriptDiagnostic
	if wc := decision.WordCount; wc != nil {
		below := wc.Min != nil && profile.WordCount < *wc.Min
		above := wc.Max != nil && profile.WordCount > *wc.Max
		if below || above {
			suggested := ""
			switch {
			case wc.Min != nil && wc.Max != nil:
				suggested = fmt.Sprintf("把字数调整到 %d-%d 之间。", *wc.Min, *wc.Max)
			case wc.Min != nil:
				suggested = fmt.Sprintf("把字数增加到至少 %d。", *wc.Min)
			default:
				suggested = fmt.Sprintf("把字数压缩到不超过 %d。", *wc.Max)
			}
			actual := profile.WordCount
			diagnostics = append(diagnostics, docScriptDiagnostic{
				Severity:  docScriptDiagnosticError,
				Code:      docScriptCodeWordCountRange,
				Expected:  &docScriptDiagnosticExpectation{Min: wc.Min, Max: wc.Max},
				Actual:    &actual,
				Msg:       "字数不满足 Presentation Decision。",
				Suggested: suggested,
			})
		}
	}
	for _, required := range decision.VisualPlan.Blocks {
		if required.Type == nil || required.MinCount == nil {
			continue
		}
		blockType, minCount := *required.Type, *required.MinCount
		actual := docScriptBlockCount(profile.Blocks, blockType)
		if actual >= minCount {
			continue
		}
		purpose := ""
		if text := strings.TrimSpace(required.Purpose); text != "" {
			purpose = "（用途：" + text + "）"
		}
		diagnostics = append(diagnostics, docScriptDiagnostic{
			Severity:  docScriptDiagnosticError,
			Code:      docScriptCodeRequiredBlock,
			Expected:  &docScriptDiagnosticExpectation{Type: blockType, MinCount: minCount},
			Actual:    &actual,
			Msg:       fmt.Sprintf("草稿缺少必需的 %s 块%s。", blockType, purpose),
			Suggested: fmt.Sprintf("至少补充 %d 个 %s 块%s。", minCount, blockType, purpose),
		})
	}
	return diagnostics
}

func docScriptBlockCount(blocks []docxparse.BlockShare, blockType string) int {
	if blockType == docScriptListBlockType {
		return docScriptBlockCount(blocks, "ul") + docScriptBlockCount(blocks, "ol")
	}
	for _, block := range blocks {
		if block.Type == blockType {
			return block.Count
		}
	}
	return 0
}

// ---------- init-draft 工作区 ----------

func initDocScriptDraft(rawDecision string) (docScriptDraftResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return docScriptDraftResult{}, fmt.Errorf("获取当前工作目录失败: %w", err)
	}
	var (
		draftPath string
		workspace string
	)
	// 8 位随机十六进制几乎不会碰撞；极端情况下重试几次，绝不复用已存在的目录
	for attempt := 0; attempt < 5; attempt++ {
		draftPath, err = newDocScriptWorkspacePath()
		if err != nil {
			return docScriptDraftResult{}, fmt.Errorf("生成草稿工作区路径失败: %w", err)
		}
		workspace = filepath.Dir(draftPath)
		if err := safefile.ValidateOutputPath(filepath.Join(cwd, workspace)); err != nil {
			return docScriptDraftResult{}, err
		}
		err = os.Mkdir(workspace, 0o755)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return docScriptDraftResult{}, fmt.Errorf("创建草稿工作区 %s 失败: %w", workspace, err)
		}
	}
	if err != nil {
		return docScriptDraftResult{}, fmt.Errorf("创建草稿工作区失败（多次生成的目录名均已存在）: %w", err)
	}
	if err := safefile.AtomicWriteFile(filepath.Join(workspace, docScriptDecisionFile), []byte(rawDecision), 0o644); err != nil {
		if cleanupErr := removeDocScriptWorkspace(draftPath); cleanupErr != nil {
			return docScriptDraftResult{}, errors.Join(fmt.Errorf("保存 Presentation Decision 失败: %w", err), cleanupErr)
		}
		return docScriptDraftResult{}, fmt.Errorf("保存 Presentation Decision 失败: %w", err)
	}
	return docScriptDraftResult{
		CWD:       cwd,
		Workspace: workspace,
		DraftPath: draftPath,
		Tip:       docScriptDraftTip,
	}, nil
}

func newDocScriptWorkspacePath() (string, error) {
	random := make([]byte, docScriptDraftRandomHexLen/2)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	dir := docScriptDraftDirPrefix + hex.EncodeToString(random) + docScriptDraftDirSuffix
	return filepath.Join(dir, docScriptDraftXMLFileName), nil
}

// removeDocScriptWorkspace 只清理本次创建的工作区：先删决策文件，再非递归删除目录；
// 路径不符合 draft_<8hex>_folder/draft.xml 时拒绝删除。
func removeDocScriptWorkspace(draftPath string) error {
	clean := filepath.Clean(draftPath)
	if !isDocScriptWorkspacePath(clean) {
		return fmt.Errorf("拒绝删除非预期的草稿工作区路径 %s", filepath.Dir(clean))
	}
	for _, entry := range []string{filepath.Join(filepath.Dir(clean), docScriptDecisionFile), filepath.Dir(clean)} {
		if err := os.Remove(entry); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("清理失败的草稿工作区 %s 失败: %w", entry, err)
		}
	}
	return nil
}

func isDocScriptWorkspacePath(path string) bool {
	dir := filepath.Dir(path)
	name := filepath.Base(dir)
	random := strings.TrimSuffix(strings.TrimPrefix(name, docScriptDraftDirPrefix), docScriptDraftDirSuffix)
	_, decodeErr := hex.DecodeString(random)
	return filepath.Dir(dir) == "." &&
		filepath.Base(path) == docScriptDraftXMLFileName &&
		strings.HasPrefix(name, docScriptDraftDirPrefix) && strings.HasSuffix(name, docScriptDraftDirSuffix) &&
		len(random) == docScriptDraftRandomHexLen && decodeErr == nil
}
