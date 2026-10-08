package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// commentReplyElementInput --content JSON 数组的单项
type commentReplyElementInput struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	MentionUser string `json:"mention_user,omitempty"`
	Link        string `json:"link,omitempty"`
}

// commentFileFullAnchorBlockID 云盘文件（type=file）全文评论的锚点占位值。
// 服务端目前拒绝 file 目标省略 anchor.block_id，官方 lark-cli 同样下发该占位值。
const commentFileFullAnchorBlockID = "test"

// maxCommentTotalRunes 服务端对一条评论所有 reply_elements 文本合计的字符上限（按 rune 计，实测 10000，
// 超限返回不透明的 1069302；拆成多个 text 元素也计入同一额度）。
const maxCommentTotalRunes = 10000

var driveAddCommentCmd = &cobra.Command{
	Use:   "add-comment",
	Short: "添加富文本评论（支持局部/单元格/幻灯片/多维表格记录/wiki 解析/多元素）",
	Long: `向文档添加评论。

支持:
  - 全局评论（docx/doc 默认）
  - 局部评论（--block-id 指定锚点）：
      docx     --block-id <block_id>
      sheet    --block-id <sheetId>!<cell>，如 a281f9!D6（单元格评论，必填）
      slides   --block-id <slide-block-type>!<xml-id>，如 shape!bPq（必填）
      bitable  --block-id <table-id>!<record-id>!<view-id>（记录评论，必填）
  - 云盘文件（file）全文评论（服务端仅支持部分扩展名，如 .md/.txt/.json/.csv/.pptx/.png/.jpg/.zip 等）
  - wiki URL / token 自动解析为底层文档
  - 富文本 reply_elements: text / mention_user / link（所有 text 合计不超过 10000 字符）

必填:
  --doc        文档输入: 裸 token（配合 --type，默认 docx）或 docx/doc/sheets/slides/base/file/wiki URL
  --content    reply_elements JSON 数组

可选:
  --type          裸 token 的文档类型（docx/doc/sheet/slides/bitable/file；默认 docx）
  --block-id      局部评论锚点（格式见上）
  --full          强制全局评论（docx/doc/file）
  --user-access-token  覆盖登录态

权限:
  - User Access Token
  - docs:document.comment:create / docs:document.comment:write_only

示例:
  # 全局评论
  feishu-cli drive add-comment --doc doccnxxxx --content '[{"type":"text","text":"需要修改标题"}]'

  # 局部评论（必须知道 block_id）
  feishu-cli drive add-comment --doc https://xxx.feishu.cn/docx/yyy --block-id blk_xxx \
    --content '[{"type":"text","text":"这段重写"}]'

  # wiki URL 自动解析
  feishu-cli drive add-comment --doc https://xxx.feishu.cn/wiki/zzz --content '[{"type":"text","text":"收到"}]'

  # 电子表格单元格评论
  feishu-cli drive add-comment --doc shtcnxxx --type sheet --block-id a281f9!D6 \
    --content '[{"type":"text","text":"这个数需要核对"}]'

  # 多维表格记录评论
  feishu-cli drive add-comment --doc https://xxx.feishu.cn/base/bascnxxx \
    --block-id tblxxx!recxxx!vewxxx --content '[{"type":"text","text":"请补充"}]'

  # 富文本：文本 + 提及 + 链接
  feishu-cli drive add-comment --doc doccnxxxx --content '[
    {"type":"text","text":"参考文档 "},
    {"type":"link","link":"https://feishu.cn"},
    {"type":"text","text":" @"},
    {"type":"mention_user","mention_user":"ou_xxx"}
  ]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := requireUserToken(cmd, "drive add-comment")
		if err != nil {
			return err
		}

		docInput, _ := cmd.Flags().GetString("doc")
		content, _ := cmd.Flags().GetString("content")
		blockID, _ := cmd.Flags().GetString("block-id")
		forceFull, _ := cmd.Flags().GetBool("full")
		output, _ := cmd.Flags().GetString("output")
		explicitType, _ := cmd.Flags().GetString("type")
		blockID = strings.TrimSpace(blockID)

		if docInput == "" {
			return fmt.Errorf("--doc 必填")
		}
		if content == "" {
			return fmt.Errorf("--content 必填")
		}

		// 解析 --content JSON
		replyElements, err := parseReplyElements(content)
		if err != nil {
			return clierr.Usage(err)
		}

		// 解析 --doc 输入
		fileToken, fileType, resolvedBy, err := resolveCommentDocWithType(docInput, explicitType, token)
		if err != nil {
			return err
		}

		anchor, err := buildCommentAnchor(fileType, blockID, forceFull)
		if err != nil {
			return err
		}
		if anchor == nil || fileType == "file" {
			// 全文评论不带 block_id（file 的占位锚点不是用户指定的局部锚点）
			blockID = ""
		}

		req := client.CreateNewCommentReq{
			FileToken:     fileToken,
			FileType:      fileType,
			ReplyElements: replyElements,
			Anchor:        anchor,
		}

		data, err := client.CreateNewComment(req, token)
		if err != nil {
			return err
		}

		result := map[string]any{
			"file_token":  fileToken,
			"file_type":   fileType,
			"resolved_by": resolvedBy,
			"data":        json.RawMessage(data),
		}
		if blockID != "" {
			result["block_id"] = blockID
			result["is_whole"] = false
		} else {
			result["is_whole"] = true
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("评论创建成功!\n")
		fmt.Printf("  文件:       %s (%s)\n", fileToken, fileType)
		if blockID != "" {
			fmt.Printf("  锚点 block: %s\n", blockID)
		}
		// 尝试打印 comment_id
		var parsed struct {
			CommentID string `json:"comment_id"`
		}
		_ = json.Unmarshal(data, &parsed)
		if parsed.CommentID != "" {
			fmt.Printf("  评论 ID:    %s\n", parsed.CommentID)
		}
		return nil
	},
}

// buildCommentAnchor 按文档类型构造 new_comments 的 anchor：
//   - docx：--block-id 非空且未 --full 时为局部评论 {block_id}；doc 不支持局部评论；
//   - sheet：必须 --block-id <sheetId>!<cell> → {block_id, sheet_col, sheet_row}（0 起始）；
//   - slides：必须 --block-id <slide-block-type>!<xml-id> → {block_id, slide_block_type}；
//   - bitable：必须 --block-id <table-id>!<record-id>!<view-id> → {block_id, base_record_id, base_view_id}；
//   - file：全文评论，锚点为服务端要求的占位 block_id。
//
// 返回 nil 表示全文评论（不下发 anchor）。
func buildCommentAnchor(fileType, blockID string, forceFull bool) (map[string]any, error) {
	switch fileType {
	case "docx", "doc":
		if blockID == "" || forceFull {
			return nil, nil
		}
		if fileType != "docx" {
			return nil, clierr.Usagef("局部评论（--block-id）仅支持 docx 文档，当前 file_type=%s", fileType)
		}
		return map[string]any{"block_id": blockID}, nil
	case "sheet":
		if forceFull {
			return nil, clierr.Usagef("电子表格评论不支持 --full，请用 --block-id <sheetId>!<cell>（如 a281f9!D6）")
		}
		if blockID == "" {
			return nil, clierr.Usagef("电子表格评论必须指定 --block-id <sheetId>!<cell>（如 a281f9!D6）")
		}
		sheetID, col, row, err := parseSheetCellAnchor(blockID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"block_id": sheetID, "sheet_col": col, "sheet_row": row}, nil
	case "slides":
		if forceFull {
			return nil, clierr.Usagef("幻灯片评论不支持 --full，请用 --block-id <slide-block-type>!<xml-id>（如 shape!bPq）")
		}
		parts := strings.SplitN(blockID, "!", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, clierr.Usagef("幻灯片评论必须指定 --block-id <slide-block-type>!<xml-id>（如 shape!bPq），得到 %q", blockID)
		}
		return map[string]any{"block_id": strings.TrimSpace(parts[1]), "slide_block_type": strings.TrimSpace(parts[0])}, nil
	case "bitable":
		if forceFull {
			return nil, clierr.Usagef("多维表格评论不支持 --full，请用 --block-id <table-id>!<record-id>!<view-id>")
		}
		parts := strings.Split(blockID, "!")
		if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
			return nil, clierr.Usagef("多维表格记录评论必须指定 --block-id <table-id>!<record-id>!<view-id>，得到 %q", blockID)
		}
		return map[string]any{
			"block_id":       strings.TrimSpace(parts[0]),
			"base_record_id": strings.TrimSpace(parts[1]),
			"base_view_id":   strings.TrimSpace(parts[2]),
		}, nil
	case "file":
		if blockID != "" && !forceFull {
			return nil, clierr.Usagef("云盘文件仅支持全文评论，不支持 --block-id")
		}
		return map[string]any{"block_id": commentFileFullAnchorBlockID}, nil
	}
	return nil, clierr.Usagef("不支持对 %s 类型添加评论（支持 docx/doc/sheet/slides/bitable/file）", fileType)
}

// parseSheetCellAnchor 解析 "<sheetId>!<cell>"（如 a281f9!D6）为 sheetId 与 0 起始的列/行号。
func parseSheetCellAnchor(input string) (string, int, int, error) {
	parts := strings.SplitN(strings.TrimSpace(input), "!", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", 0, 0, clierr.Usagef("电子表格 --block-id 格式应为 <sheetId>!<cell>（如 a281f9!D6），得到 %q", input)
	}
	sheetID := strings.TrimSpace(parts[0])
	cell := strings.ToUpper(strings.TrimSpace(parts[1]))
	i := 0
	for i < len(cell) && cell[i] >= 'A' && cell[i] <= 'Z' {
		i++
	}
	if i == 0 || i >= len(cell) {
		return "", 0, 0, clierr.Usagef("单元格引用 %q 非法（应形如 D6）", parts[1])
	}
	col := 0
	for _, ch := range cell[:i] {
		col = col*26 + int(ch-'A'+1)
		if col > 1<<20 {
			return "", 0, 0, clierr.Usagef("单元格引用 %q 列号过大", parts[1])
		}
	}
	row, err := strconv.Atoi(cell[i:])
	if err != nil || row < 1 {
		return "", 0, 0, clierr.Usagef("单元格引用 %q 的行号非法（必须 >= 1）", parts[1])
	}
	return sheetID, col - 1, row - 1, nil
}

// parseReplyElements 解析 --content JSON 数组
func parseReplyElements(raw string) ([]map[string]any, error) {
	var inputs []commentReplyElementInput
	if err := json.Unmarshal([]byte(raw), &inputs); err != nil {
		return nil, fmt.Errorf("--content 不是合法 JSON: %w\n示例: --content '[{\"type\":\"text\",\"text\":\"评论内容\"}]'", err)
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("--content 至少包含一个 reply element")
	}

	out := make([]map[string]any, 0, len(inputs))
	totalRunes := 0
	for i, input := range inputs {
		idx := i + 1
		switch strings.TrimSpace(input.Type) {
		case "text":
			if strings.TrimSpace(input.Text) == "" {
				return nil, fmt.Errorf("--content 第 %d 个元素 type=text 的 text 不能为空", idx)
			}
			// 服务端按所有 text 元素的原始字符数合计计算上限（拆成多个元素不能绕过），超限返回不透明的 1069302
			totalRunes += utf8.RuneCountInString(input.Text)
			if totalRunes > maxCommentTotalRunes {
				return nil, fmt.Errorf("--content 所有 text 元素合计 %d 字符（到第 %d 个元素），超过服务端上限 %d；请缩短评论内容（拆分为多个元素不能绕过合计上限）", totalRunes, idx, maxCommentTotalRunes)
			}
			out = append(out, map[string]any{"type": "text", "text": input.Text})
		case "mention_user":
			target := input.MentionUser
			if target == "" {
				target = input.Text
			}
			if target == "" {
				return nil, fmt.Errorf("--content 第 %d 个元素 type=mention_user 需要 mention_user 或 text 字段", idx)
			}
			out = append(out, map[string]any{"type": "mention_user", "mention_user": target})
		case "link":
			target := input.Link
			if target == "" {
				target = input.Text
			}
			if target == "" {
				return nil, fmt.Errorf("--content 第 %d 个元素 type=link 需要 link 或 text 字段", idx)
			}
			out = append(out, map[string]any{"type": "link", "link": target})
		default:
			return nil, fmt.Errorf("--content 第 %d 个元素不支持的 type=%q（合法值: text, mention_user, link）", idx, input.Type)
		}
	}
	return out, nil
}

// commentTargetTypes 是 new_comments 支持的目标类型。
var commentTargetTypes = []string{
	client.ResourceTypeDocx, client.ResourceTypeDoc, client.ResourceTypeSheet,
	client.ResourceTypeSlides, client.ResourceTypeBitable, client.ResourceTypeFile,
}

// resolveCommentDoc 解析 --doc 输入（裸 token 默认 docx），返回 (file_token, file_type, resolved_by)。
func resolveCommentDoc(input, userAccessToken string) (string, string, string, error) {
	return resolveCommentDocWithType(input, "", userAccessToken)
}

// resolveCommentDocWithType 解析 --doc 输入，返回 (file_token, file_type, resolved_by)。
// 只按 URL 路径前缀识别类型（query 中的 /wiki/ 等字样不会劫持解析）；wiki 经 node_by_token 换出底层文档；
// 裸 token 用 explicitType（为空时默认 docx，保持历史行为）。
func resolveCommentDocWithType(input, explicitType, userAccessToken string) (string, string, string, error) {
	res, err := resolveResourceArg(input, resourceArgOptions{
		ArgName:         "--doc",
		ExplicitType:    explicitType,
		DefaultType:     client.ResourceTypeDocx,
		Allowed:         commentTargetTypes,
		ResolveWiki:     true,
		UserAccessToken: userAccessToken,
	})
	if err != nil {
		return "", "", "", err
	}
	switch {
	case res.WikiNode != nil:
		return res.Token, res.Type, "wiki", nil
	case res.FromURL:
		return res.Token, res.Type, res.Type + "_url", nil
	default:
		// 裸 token：resolved_by 反映实际类型（docx 时仍为 docx_token，保持兼容）
		return res.Token, res.Type, res.Type + "_token", nil
	}
}

// extractURLSegmentToken 从 URL 里提取某个路径段后面紧跟的 token
// 比如 /docx/doccnxxx 返回 doccnxxx
func extractURLSegmentToken(rawURL, segment string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	path := u.Path
	idx := strings.Index(path, segment)
	if idx < 0 {
		return "", false
	}
	remain := path[idx+len(segment):]
	// 截到下一个 /
	if next := strings.Index(remain, "/"); next >= 0 {
		remain = remain[:next]
	}
	if remain == "" {
		return "", false
	}
	return remain, true
}

func init() {
	driveCmd.AddCommand(driveAddCommentCmd)
	driveAddCommentCmd.Flags().String("doc", "", "文档输入：docx token / docx URL / doc URL / wiki URL（必填）")
	driveAddCommentCmd.Flags().String("content", "", "reply_elements JSON 数组（必填）")
	driveAddCommentCmd.Flags().String("type", "", "裸 token 的文档类型（docx/doc/sheet/slides/bitable/file；默认 docx）")
	driveAddCommentCmd.Flags().String("block-id", "", "局部评论锚点（docx: block_id；sheet: <sheetId>!<cell>；slides: <type>!<xml-id>；bitable: <table>!<record>!<view>）")
	driveAddCommentCmd.Flags().Bool("full", false, "强制全局评论")
	driveAddCommentCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	driveAddCommentCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(driveAddCommentCmd, "doc", "content")
}
