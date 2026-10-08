package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/riba2534/feishu-cli/internal/client"
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

var driveAddCommentCmd = &cobra.Command{
	Use:   "add-comment",
	Short: "添加富文本评论（支持局部/wiki 解析/多元素）",
	Long: `向文档添加评论。

支持:
  - 全局评论（默认）
  - 局部评论（--block-id 指定锚点，仅 docx 支持）
  - wiki URL 自动解析为 docx 目标
  - 富文本 reply_elements: text / mention_user / link

必填:
  --doc        文档输入，支持四种格式:
                 1. docx token: doccnxxxx
                 2. docx URL:   https://xxx.feishu.cn/docx/xxx
                 3. doc URL:    https://xxx.feishu.cn/doc/xxx
                 4. wiki URL:   https://xxx.feishu.cn/wiki/xxx（自动解析）
  --content    reply_elements JSON 数组（1-1000 字符/元素）

可选:
  --block-id      局部评论锚点（docx 专用）
  --full          强制全局评论（默认）
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

		if docInput == "" {
			return fmt.Errorf("--doc 必填")
		}
		if content == "" {
			return fmt.Errorf("--content 必填")
		}

		// 解析 --content JSON
		replyElements, err := parseReplyElements(content)
		if err != nil {
			return err
		}

		// 解析 --doc 输入
		fileToken, fileType, resolvedBy, err := resolveCommentDoc(docInput, token)
		if err != nil {
			return err
		}

		// local 评论只在 docx 支持
		if blockID != "" && !forceFull {
			if fileType != "docx" {
				return fmt.Errorf("局部评论（--block-id）仅支持 docx 文档，当前 file_type=%s", fileType)
			}
		}

		req := client.CreateNewCommentReq{
			FileToken:     fileToken,
			FileType:      fileType,
			BlockID:       blockID,
			ReplyElements: replyElements,
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
	for i, input := range inputs {
		idx := i + 1
		switch strings.TrimSpace(input.Type) {
		case "text":
			if strings.TrimSpace(input.Text) == "" {
				return nil, fmt.Errorf("--content 第 %d 个元素 type=text 的 text 不能为空", idx)
			}
			if utf8.RuneCountInString(input.Text) > 1000 {
				return nil, fmt.Errorf("--content 第 %d 个元素 text 超过 1000 字符", idx)
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

// resolveCommentDoc 解析 --doc 输入，返回 (file_token, file_type, resolved_by)。
// 只按 URL 路径前缀识别类型（query 中的 /wiki/ 等字样不会劫持解析）；wiki 经 node_by_token 换出底层文档。
func resolveCommentDoc(input, userAccessToken string) (string, string, string, error) {
	res, err := resolveResourceArg(input, resourceArgOptions{
		ArgName:         "--doc",
		DefaultType:     client.ResourceTypeDocx,
		Allowed:         []string{client.ResourceTypeDocx, client.ResourceTypeDoc},
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
		return res.Token, res.Type, "docx_token", nil
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
	driveAddCommentCmd.Flags().String("block-id", "", "局部评论锚点 block_id（docx 专用）")
	driveAddCommentCmd.Flags().Bool("full", false, "强制全局评论")
	driveAddCommentCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	driveAddCommentCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(driveAddCommentCmd, "doc", "content")
}
