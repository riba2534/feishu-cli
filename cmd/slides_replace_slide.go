package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var slidesReplaceSlideCmd = &cobra.Command{
	Use:   "replace-slide <presentation>",
	Short: "元素级编辑一页：block_replace 替换 / block_insert 插入（不改页序）",
	Long: `对已有页面做元素级编辑，只动 --parts 点名的元素，同页其他元素不受影响。

参数:
  <presentation>   xml_presentation_id、/slides/ URL 或 /wiki/ URL
  --slide-id       目标页面 slide_id（必填）
  --parts          JSON 数组（必填，1-200 条）；支持 @file、- 读 stdin
  --revision-id    -1（默认）表示最新版本；传具体版本号用于乐观锁
  --tid            并发编辑事务 ID（通常留空）
  --no-lint        跳过服务端 XML lint（lint 的对象是写入后的整页，片段本身合法但把邻居挤出画布也会被拒）
  --dry-run        打印规范化、注入 id 之后真正会发送的请求

parts 格式:
  {"action":"block_replace","block_id":"<元素 id>","replacement":"<shape ...>...</shape>"}
  {"action":"block_insert","insertion":"<shape ...>...</shape>","insert_before_block_id":"<可选>"}

CLI 自动处理（官方实测的契约坑）:
  - block_replace 的 replacement 根元素自动注入 id="<block_id>"（不带会报 3350001）
  - 根为 <shape> 且缺 <content/> 时自动补上（SML 2.0 要求）
  - 兼容别名并在输出 normalizations 中列出：action replace→block_replace、insert→block_insert；
    target_id→block_id；block/content/element/shape→replacement 或 insertion
  - 不支持 str_replace（产品方向是结构化编辑）；整页改写请用 slides update-slide

输出 JSON：parts_count、revision_id；服务端部分失败时透出 failed_part_index / failed_reason，
已写入但有告警时透出 issues。

权限: slides:presentation:update 或 slides:presentation:write_only

示例:
  feishu-cli slides replace-slide <xml_presentation_id> --slide-id <slide_id> --parts @parts.json
  feishu-cli slides replace-slide <xml_presentation_id> --slide-id <slide_id> \
    --parts '[{"action":"block_replace","block_id":"bkW","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"600\" height=\"80\"><content><p>新标题</p></content></shape>"}]'
  feishu-cli slides replace-slide <xml_presentation_id> --slide-id <slide_id> --parts @parts.json --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slideID := strings.TrimSpace(flagString(cmd, "slide-id"))
		if slideID == "" {
			return clierr.Usagef("--slide-id 不能为空")
		}
		rawParts, err := readSlidesFlagInput(cmd, "parts")
		if err != nil {
			return err
		}
		parts, normalizations, err := parseSlidesReplaceParts(rawParts)
		if err != nil {
			return err
		}
		injected, err := injectSlidesReplaceParts(parts)
		if err != nil {
			return err
		}
		revisionID, err := validateSlidesRevisionID(cmd)
		if err != nil {
			return err
		}
		query := map[string]any{"slide_id": slideID, "revision_id": revisionID}
		if tid := strings.TrimSpace(flagString(cmd, "tid")); tid != "" {
			query["tid"] = tid
		}
		body := withSlidesLint(cmd, map[string]any{"parts": injected})

		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			pres, wikiStep, err := slidesDryRunPresentation(args[0])
			if err != nil {
				return err
			}
			var steps []map[string]any
			if wikiStep != nil {
				steps = append(steps, wikiStep)
			}
			steps = append(steps, map[string]any{
				"method": "POST",
				"path":   client.SlidesPresentationPath(pres, "/slide/replace"),
				"params": query,
				"body":   body,
			})
			out := map[string]any{"dry_run": true, "parts_count": len(injected), "steps": steps}
			if len(normalizations) > 0 {
				out["normalizations"] = normalizations
			}
			return printJSON(out)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		userAccessToken := resolveOptionalUserToken(cmd)
		presentationID, err := resolvePresentationArg(args[0], userAccessToken)
		if err != nil {
			return err
		}
		data, err := client.SlidesCall("替换 slides 页面元素", "POST", client.SlidesPresentationPath(presentationID, "/slide/replace"), query, body, userAccessToken)
		if err != nil {
			return enrichSlidesWriteError(err, "")
		}
		result := map[string]any{
			"xml_presentation_id": presentationID,
			"slide_id":            slideID,
			"parts_count":         len(injected),
		}
		if len(normalizations) > 0 {
			result["normalizations"] = normalizations
		}
		copySlidesRevision(result, data)
		// 服务端用 failed_part_index / failed_reason 报告部分失败，原样透出
		for _, key := range []string{"failed_part_index", "failed_reason", "issues"} {
			if v, ok := data[key]; ok {
				result[key] = v
			}
		}
		if reason, _ := data["failed_reason"].(string); strings.TrimSpace(reason) != "" {
			fmt.Fprintf(os.Stderr, "⚠ 部分 parts 未生效（failed_part_index=%v）：%s\n", data["failed_part_index"], reason)
		}
		return printJSON(result)
	},
}

var slidesUpdateSlideCmd = &cobra.Command{
	Use:   "update-slide <presentation>",
	Short: "整页覆盖一页：把完整 <slide> XML 写回该页（保留 slide_id 和页序）",
	Long: `把一整页 XML 写回已有页面，页面变成 --content 描述的样子：保留下来的元素原地更新，
新增元素插入，--content 里没有的元素会被删除；<style> 背景与 <note> 备注也按 XML 更新。
slide_id 与页序不变。只改个别元素时用 slides replace-slide 更省事、更安全。

参数:
  <presentation>   xml_presentation_id、/slides/ URL 或 /wiki/ URL
  --slide-id       要覆盖的页面 slide_id（必填）
  --content        该页完整的目标 XML，单个 <slide> 根元素（必填）；支持 @file、- 读 stdin
  --revision-id    -1（默认）表示最新版本；指定旧版本会以该快照重建本页并丢弃之后的改动
  --tid            并发编辑事务 ID（通常留空）
  --no-lint        跳过服务端 XML lint
  --dry-run        打印将要发送的请求

CLI 自动处理（官方实测的契约坑）:
  - 根 <slide> 自动带上 id=<slide-id>；根上已有的 id 与 --slide-id 不一致时拒绝（防止把 A 页 XML 写到 B 页）
  - 去掉 <slide> 下 <note> 的 id：过期的 note id（从别页复制或页面被重建）会让服务端整页拒绝
    "block is not NoteBlock"
  - <img src="@./pic.png"> 占位符先上传再替换为 file_token
  - 服务端返回 failed_reason 表示整页未写入，按失败退出

推荐流程: slides get <id> --slide-id <slide_id> --output-file page.xml → 编辑 page.xml →
         slides update-slide <id> --slide-id <slide_id> --content @page.xml

权限: slides:presentation:update 或 slides:presentation:write_only

示例:
  feishu-cli slides update-slide <xml_presentation_id> --slide-id <slide_id> --content @page.xml
  feishu-cli slides update-slide <xml_presentation_id> --slide-id <slide_id> --content @page.xml --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slideID := strings.TrimSpace(flagString(cmd, "slide-id"))
		if slideID == "" {
			return clierr.Usagef("--slide-id 不能为空")
		}
		content, err := readSlidesFlagInput(cmd, "content")
		if err != nil {
			return err
		}
		content, err = prepareSlidesUpdateContent(content, slideID)
		if err != nil {
			return err
		}
		revisionID, err := validateSlidesRevisionID(cmd)
		if err != nil {
			return err
		}
		placeholders := extractImagePlaceholderPaths([]string{content})
		if err := validateImagePlaceholderFiles("--content", placeholders); err != nil {
			return err
		}
		query := map[string]any{"slide_id": slideID, "revision_id": revisionID}
		if tid := strings.TrimSpace(flagString(cmd, "tid")); tid != "" {
			query["tid"] = tid
		}
		buildBody := func(c string) map[string]any {
			// block_id 用页面自身 id，服务端一次性替换整个 <slide>
			return withSlidesLint(cmd, map[string]any{"parts": []map[string]any{{
				"action":      "block_replace",
				"block_id":    slideID,
				"replacement": c,
			}}})
		}

		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			pres, wikiStep, err := slidesDryRunPresentation(args[0])
			if err != nil {
				return err
			}
			var steps []map[string]any
			if wikiStep != nil {
				steps = append(steps, wikiStep)
			}
			for _, p := range placeholders {
				steps = append(steps, map[string]any{
					"method": "POST",
					"path":   "/open-apis/drive/v1/medias/upload_all",
					"desc":   fmt.Sprintf("上传 @%s（parent_node=%s）", p, pres),
				})
			}
			steps = append(steps, map[string]any{
				"method": "POST",
				"path":   client.SlidesPresentationPath(pres, "/slide/replace"),
				"params": query,
				"body":   buildBody(content),
			})
			return printJSON(map[string]any{"dry_run": true, "slide_id": slideID, "content_bytes": len(content), "images_to_upload": len(placeholders), "steps": steps})
		}

		if err := config.Validate(); err != nil {
			return err
		}
		userAccessToken := resolveOptionalUserToken(cmd)
		presentationID, err := resolvePresentationArg(args[0], userAccessToken)
		if err != nil {
			return err
		}
		result := map[string]any{"xml_presentation_id": presentationID, "slide_id": slideID}
		uploadedHint := ""
		if len(placeholders) > 0 {
			tokens, uploaded, err := uploadSlidesPlaceholders(presentationID, placeholders, userAccessToken)
			if err != nil {
				return fmt.Errorf("%w\n页面未更新；失败前已上传 %d/%d 张图片", err, uploaded, len(placeholders))
			}
			content = replaceImagePlaceholders(content, tokens)
			result["images_uploaded"] = uploaded
			uploadedHint = fmt.Sprintf("页面失败前已上传 %d 张图片；直接重跑会再上传一份", len(placeholders))
		}
		data, err := client.SlidesCall("整页更新 slides 页面", "POST", client.SlidesPresentationPath(presentationID, "/slide/replace"), query, buildBody(content), userAccessToken)
		if err != nil {
			return enrichSlidesWriteError(err, uploadedHint)
		}
		// 单个 part 承载整页，任何 failed_reason 都意味着页面没写进去，不能当成功报告
		if reason, _ := data["failed_reason"].(string); strings.TrimSpace(reason) != "" {
			hint := "先检查 --content：不支持的元素、缺 <content/> 的 <shape>、坐标超出 960×540"
			if strings.Contains(strings.ToLower(reason), "not found") {
				hint = "检查 <presentation> 与 --slide-id：页面可能已被删除或重建，重新 slides get 取当前 slide_id"
			}
			msg := fmt.Sprintf("页面 %s 未更新: %s\n提示：%s", slideID, reason, hint)
			if uploadedHint != "" {
				msg += "\n提示：" + uploadedHint
			}
			return fmt.Errorf("%s", msg)
		}
		copySlidesRevision(result, data)
		if issues, ok := data["issues"]; ok {
			result["issues"] = issues
		}
		return printJSON(result)
	},
}

// prepareSlidesUpdateContent 校验整页 XML 并做两项改写：去掉过期 note id、根元素带上 slide id。
func prepareSlidesUpdateContent(content, slideID string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", clierr.Usagef("--content 不能为空（传 XML、@file 或 - 读 stdin）")
	}
	rootID, err := checkSlideRoot(content)
	if err != nil {
		return "", clierr.Usagef("--content %v", err)
	}
	if rootID != "" && rootID != slideID {
		return "", clierr.Usagef("--content 根元素是 <slide id=%q>，但 --slide-id 是 %q；确认要覆盖的是哪一页，或去掉根上的 id", rootID, slideID)
	}
	content = stripSlideNoteID(content)
	stamped, err := ensureXMLRootID(content, slideID)
	if err != nil {
		return "", clierr.Usagef("--content 根 <slide> 的写法无法注入页面 id（例如带命名空间前缀 <sml:slide>）；改写为 <slide>，需要命名空间时用默认 xmlns")
	}
	return stamped, nil
}

// slidesReplacePart 是规范化后的一条 part。
type slidesReplacePart struct {
	Action              string
	BlockID             *string
	Replacement         *string
	Insertion           *string
	InsertBeforeBlockID *string
}

// slidesReplaceNormalization 记录每次兼容性改写，输出给调用方核对最终请求形态。
type slidesReplaceNormalization struct {
	PartIndex int    `json:"part_index"`
	Kind      string `json:"kind"`
	From      string `json:"from"`
	To        string `json:"to"`
}

var (
	slidesReplaceActionAliases = map[string]string{"replace": "block_replace", "insert": "block_insert"}
	slidesReplacePayloadAlias  = []string{"block", "content", "element", "shape"}
	slidesReplaceSchemas       = map[string]struct {
		fields  []string
		payload string
		example string
	}{
		"block_replace": {[]string{"action", "block_id", "replacement"}, "replacement",
			`{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"text\"><content><p>text</p></content></shape>"}`},
		"block_insert": {[]string{"action", "insertion", "insert_before_block_id"}, "insertion",
			`{"action":"block_insert","insertion":"<shape type=\"rect\" topLeftX=\"80\" topLeftY=\"80\" width=\"100\" height=\"100\"/>"}`},
	}
)

// parseSlidesReplaceParts 解析 --parts：只做语义确定的别名规范化，然后严格校验字段与类型。
func parseSlidesReplaceParts(raw string) ([]slidesReplacePart, []slidesReplaceNormalization, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil, clierr.Usagef("--parts 不能为空")
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(s), &decoded); err != nil {
		return nil, nil, clierr.Usagef("--parts 不是合法 JSON（需要对象数组）: %v\n提示：用 --parts @parts.json 或管道 --parts - 避免 shell 转义错误", err)
	}
	if len(decoded) == 0 {
		return nil, nil, clierr.Usagef("--parts 至少需要 1 条")
	}
	if len(decoded) > maxSlidesReplaceParts {
		return nil, nil, clierr.Usagef("--parts 有 %d 条，超过上限 %d", len(decoded), maxSlidesReplaceParts)
	}
	var norms []slidesReplaceNormalization
	parts := make([]slidesReplacePart, 0, len(decoded))
	for i, m := range decoded {
		action, _ := m["action"].(string)
		if _, ok := m["action"]; ok {
			if _, isStr := m["action"].(string); !isStr {
				return nil, nil, clierr.Usagef("--parts[%d].action 必须是字符串", i)
			}
		}
		if canonical, ok := slidesReplaceActionAliases[action]; ok {
			norms = append(norms, slidesReplaceNormalization{i, "action", action, canonical})
			action = canonical
			m["action"] = canonical
		}
		switch action {
		case "block_replace", "block_insert":
		case "":
			return nil, nil, clierr.Usagef("--parts[%d].action 必填（block_replace / block_insert）", i)
		case "str_replace":
			return nil, nil, clierr.Usagef("--parts[%d] 不支持 action %q：只允许结构化的 block_replace / block_insert", i, action)
		case "page_replace", "slide_replace":
			return nil, nil, clierr.Usagef("--parts[%d] action %q 是整页替换，请改用 slides update-slide", i, action)
		default:
			return nil, nil, clierr.Usagef("--parts[%d] 未知 action %q，可选: block_replace / block_insert", i, action)
		}
		schema := slidesReplaceSchemas[action]
		// 别名规范化：target_id→block_id（仅 block_replace）、各种 payload 别名→replacement/insertion
		aliases := map[string]string{}
		if action == "block_replace" {
			aliases["target_id"] = "block_id"
		}
		for _, a := range slidesReplacePayloadAlias {
			aliases[a] = schema.payload
		}
		aliasKeys := make([]string, 0, len(aliases))
		for k := range aliases {
			aliasKeys = append(aliasKeys, k)
		}
		sort.Strings(aliasKeys)
		for _, alias := range aliasKeys {
			canonical := aliases[alias]
			v, ok := m[alias]
			if !ok {
				continue
			}
			if existing, has := m[canonical]; has && fmt.Sprint(existing) != fmt.Sprint(v) {
				return nil, nil, clierr.Usagef("--parts[%d] 字段 %q 与 %q 冲突；只传 %q", i, canonical, alias, canonical)
			}
			m[canonical] = v
			delete(m, alias)
			norms = append(norms, slidesReplaceNormalization{i, "field", alias, canonical})
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if !containsString(schema.fields, k) {
				return nil, nil, clierr.Usagef("--parts[%d] 未知字段 %q；%s 允许的字段: %s\n示例：%s", i, k, action, strings.Join(schema.fields, ", "), schema.example)
			}
		}
		p := slidesReplacePart{Action: action}
		for _, f := range []struct {
			key string
			dst **string
		}{{"block_id", &p.BlockID}, {"replacement", &p.Replacement}, {"insertion", &p.Insertion}, {"insert_before_block_id", &p.InsertBeforeBlockID}} {
			v, ok := m[f.key]
			if !ok {
				continue
			}
			str, isStr := v.(string)
			if !isStr {
				return nil, nil, clierr.Usagef("--parts[%d].%s 必须是字符串", i, f.key)
			}
			*f.dst = &str
		}
		switch action {
		case "block_replace":
			if p.BlockID == nil || strings.TrimSpace(*p.BlockID) == "" {
				return nil, nil, clierr.Usagef("--parts[%d]（block_replace）缺少非空 block_id", i)
			}
			if p.Replacement == nil || strings.TrimSpace(*p.Replacement) == "" {
				return nil, nil, clierr.Usagef("--parts[%d]（block_replace）缺少非空 replacement", i)
			}
		case "block_insert":
			if p.Insertion == nil || strings.TrimSpace(*p.Insertion) == "" {
				return nil, nil, clierr.Usagef("--parts[%d]（block_insert）缺少非空 insertion", i)
			}
		}
		parts = append(parts, p)
	}
	return parts, norms, nil
}

// injectSlidesReplaceParts 生成请求体 parts：block_replace 根元素注入 id，<shape> 补 <content/>。
func injectSlidesReplaceParts(parts []slidesReplacePart) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(parts))
	for i, p := range parts {
		m := map[string]any{"action": p.Action}
		switch p.Action {
		case "block_replace":
			fixed, err := ensureXMLRootID(*p.Replacement, *p.BlockID)
			if err != nil {
				return nil, clierr.Usagef("--parts[%d].replacement: %v", i, err)
			}
			m["block_id"] = *p.BlockID
			m["replacement"] = ensureShapeHasContent(fixed)
		case "block_insert":
			m["insertion"] = ensureShapeHasContent(*p.Insertion)
			if p.InsertBeforeBlockID != nil {
				m["insert_before_block_id"] = *p.InsertBeforeBlockID
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func init() {
	slidesCmd.AddCommand(slidesReplaceSlideCmd)
	slidesReplaceSlideCmd.Flags().String("slide-id", "", "目标页面 slide_id（必填）")
	slidesReplaceSlideCmd.Flags().String("parts", "", "replace parts JSON 数组（支持 @file、- 读 stdin，最多 200 条）")
	slidesReplaceSlideCmd.Flags().Int("revision-id", -1, "基于的演示文稿版本（-1 表示最新；正整数用于乐观锁）")
	slidesReplaceSlideCmd.Flags().String("tid", "", "并发编辑事务 ID（通常留空）")
	addSlidesNoLintFlag(slidesReplaceSlideCmd)
	slidesReplaceSlideCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不调用 API")
	slidesReplaceSlideCmd.Flags().String("user-access-token", "", "User Access Token")

	slidesCmd.AddCommand(slidesUpdateSlideCmd)
	slidesUpdateSlideCmd.Flags().String("slide-id", "", "要整页覆盖的页面 slide_id（必填）")
	slidesUpdateSlideCmd.Flags().String("content", "", "该页完整目标 XML，单个 <slide> 根（支持 @file、- 读 stdin）")
	slidesUpdateSlideCmd.Flags().Int("revision-id", -1, "基于的演示文稿版本（-1 表示最新）")
	slidesUpdateSlideCmd.Flags().String("tid", "", "并发编辑事务 ID（通常留空）")
	addSlidesNoLintFlag(slidesUpdateSlideCmd)
	slidesUpdateSlideCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不调用 API")
	slidesUpdateSlideCmd.Flags().String("user-access-token", "", "User Access Token")
}
