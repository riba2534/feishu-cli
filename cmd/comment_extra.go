package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// commentBatchGetMaxIDs batch_query 单次最多查询的评论数（对齐官方）。
const commentBatchGetMaxIDs = 100

var getCommentCmd = &cobra.Command{
	Use:   "get <file_token> <comment_id>",
	Short: "获取评论详情（含正文与回复）",
	Long: `获取单条评论的详情，包括正文与回复。

参数:
  file_token    文档 Token
  comment_id    评论 ID
  --type        文件类型（默认 docx: doc/docx/sheet/bitable/file/slides）

输出:
  JSON 中 content 为正文（根回复）可读文本，reply_list 原样保留服务端结构。

示例:
  feishu-cli comment get doccnXXX 6916106822734578184 --type docx
  feishu-cli comment get doccnXXX 6916106822734578184 --type docx -o json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		fileType, _ := cmd.Flags().GetString("type")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		comment, err := client.GetComment(args[0], args[1], fileType, userAccessToken)
		if err != nil {
			return err
		}
		if output == "json" {
			return printJSON(comment)
		}
		printCommentText(cmd.OutOrStdout(), 1, comment)
		return nil
	},
}

var batchGetCommentsCmd = &cobra.Command{
	Use:   "batch-get <file_token>",
	Short: "按评论 ID 批量获取评论",
	Long: `按评论 ID 批量获取评论（POST /open-apis/drive/v1/files/:file_token/comments/batch_query）。

参数:
  file_token      文档 Token
  --comment-ids   评论 ID，逗号分隔或重复传入（必填，单次最多 100 个）
  --type          文件类型（默认 docx: doc/docx/sheet/bitable/file/slides）

示例:
  feishu-cli comment batch-get doccnXXX --comment-ids 6916106822734578184,6916106822734578185 --type docx -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		fileType, _ := cmd.Flags().GetString("type")
		output, _ := cmd.Flags().GetString("output")
		rawIDs, _ := cmd.Flags().GetStringSlice("comment-ids")
		ids, err := normalizeCommentIDs(rawIDs)
		if err != nil {
			return err
		}
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		comments, err := client.BatchGetComments(args[0], fileType, ids, userAccessToken)
		if err != nil {
			return err
		}
		if output == "json" {
			if comments == nil {
				comments = []*client.Comment{}
			}
			return printJSON(comments)
		}
		if len(comments) == 0 {
			fmt.Println("未找到评论")
			return nil
		}
		fmt.Printf("共获取 %d 条评论:\n\n", len(comments))
		for i, c := range comments {
			printCommentText(cmd.OutOrStdout(), i+1, c)
		}
		return nil
	},
}

// normalizeCommentIDs 去空白并校验数量（1-100）与格式。
func normalizeCommentIDs(raw []string) ([]string, error) {
	ids := make([]string, 0, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !client.IsSafeResourceToken(id) {
			return nil, clierr.Usagef("--comment-ids 含非法评论 ID %q", id)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, clierr.Usagef("--comment-ids 至少需要一个评论 ID")
	}
	if len(ids) > commentBatchGetMaxIDs {
		return nil, clierr.Usagef("--comment-ids 单次最多 %d 个，得到 %d 个", commentBatchGetMaxIDs, len(ids))
	}
	return ids, nil
}

var updateReplyCmd = &cobra.Command{
	Use:   "update <file_token> <comment_id> <reply_id>",
	Short: "修改评论回复内容（整体替换）",
	Long: `整体替换一条评论回复的内容（PUT .../comments/:comment_id/replies/:reply_id）。

修改评论的根回复即修改评论正文。只有回复作者身份可以修改，其他身份会得到 1069303 forbidden：
默认使用当前 App/Bot 身份（适合修改同一 App 创建的回复）；修改用户回复需显式传入该作者的
--user-access-token（或 FEISHU_USER_ACCESS_TOKEN）。

内容（二选一）:
  --text      纯文本内容
  --content   reply_elements JSON 数组（同 drive add-comment）：text / mention_user / link
              （实测 link 必须是飞书文档链接，普通网址会返回 1069302 param error）

示例:
  feishu-cli comment reply update doccnXXX 6916106822734578184 6916106822734594568 --text "已更新"
  feishu-cli comment reply update doccnXXX 6916106822734578184 6916106822734594568 \
    --content '[{"type":"text","text":"请 "},{"type":"mention_user","mention_user":"ou_xxx"},{"type":"text","text":" 复核"}]'

  # 预览请求
  feishu-cli comment reply update doccnXXX <comment_id> <reply_id> --text "x" --dry-run`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		fileToken, commentID, replyID := args[0], args[1], args[2]
		fileType, _ := cmd.Flags().GetString("type")
		text, _ := cmd.Flags().GetString("text")
		content, _ := cmd.Flags().GetString("content")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		elements, err := buildReplyUpdateElements(text, content)
		if err != nil {
			return err
		}
		apiPath := fmt.Sprintf("/open-apis/drive/v1/files/%s/comments/%s/replies/%s",
			url.PathEscape(fileToken), url.PathEscape(commentID), url.PathEscape(replyID))
		if dryRun {
			return printJSON(map[string]any{
				"dry_run": true,
				"method":  "PUT",
				"path":    apiPath,
				"query":   map[string]string{"file_type": fileType},
				"body":    map[string]any{"content": map[string]any{"elements": elements}},
			})
		}

		// 写操作：默认保持当前 App/Bot 身份，仅显式 flag/env 时切换为 User（需与回复作者身份一致）
		userAccessToken := resolveOptionalUserToken(cmd)
		if err := client.UpdateCommentReply(fileToken, commentID, replyID, fileType, elements, userAccessToken); err != nil {
			if client.HasAPICode(err, 1069303) {
				return fmt.Errorf("%w\n提示：只有回复作者身份可以修改该回复。Bot 创建的回复用同一 App 身份（默认）；用户创建的回复需传该用户的 --user-access-token", err)
			}
			if client.HasAPICode(err, 1069302) {
				return fmt.Errorf("%w\n提示：参数不合法。link 元素必须是飞书文档链接（如 https://xxx.feishu.cn/docx/...），普通网址会被拒绝；也请检查内容是否为空或超长", err)
			}
			return err
		}
		result := map[string]any{"file_token": fileToken, "comment_id": commentID, "reply_id": replyID, "updated": true}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("回复已更新！\n  评论 ID: %s\n  回复 ID: %s\n", commentID, replyID)
		return nil
	},
}

// buildReplyUpdateElements 把 --text / --content 转为回复接口（v1）使用的 content.elements 结构。
func buildReplyUpdateElements(text, content string) ([]map[string]any, error) {
	text = strings.TrimSpace(text)
	content = strings.TrimSpace(content)
	switch {
	case text != "" && content != "":
		return nil, clierr.Usagef("--text 与 --content 只能二选一")
	case text == "" && content == "":
		return nil, clierr.Usagef("请通过 --text 或 --content 提供回复内容")
	}
	var v2 []map[string]any
	if text != "" {
		v2 = []map[string]any{{"type": "text", "text": text}}
	} else {
		parsed, err := parseReplyElements(content)
		if err != nil {
			return nil, clierr.Usage(err)
		}
		v2 = parsed
	}
	return replyElementsToV1(v2), nil
}

// replyElementsToV1 把 drive add-comment 的简化元素（text / mention_user / link）转换为
// 回复接口 content.elements 的服务端结构（text_run / person / docs_link）。
func replyElementsToV1(elements []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(elements))
	for _, el := range elements {
		switch el["type"] {
		case "text":
			out = append(out, map[string]any{"type": "text_run", "text_run": map[string]any{"text": el["text"]}})
		case "mention_user":
			out = append(out, map[string]any{"type": "person", "person": map[string]any{"user_id": el["mention_user"]}})
		case "link":
			out = append(out, map[string]any{"type": "docs_link", "docs_link": map[string]any{"url": el["link"]}})
		}
	}
	return out
}

var reactReplyCmd = &cobra.Command{
	Use:   "react <file_token> <reply_id>",
	Short: "为评论回复添加/取消表情回应",
	Long: `为评论回复添加或取消表情回应（POST /open-apis/drive/v2/files/:file_token/comments/reaction）。

对评论的根回复回应即对评论本身回应。add / delete 都是幂等的；delete 只取消当前身份的回应。
默认使用当前 App/Bot 身份；以用户身份回应需显式传入 --user-access-token（或 FEISHU_USER_ACCESS_TOKEN）。

参数:
  --emoji    表情类型（必填，区分大小写，如 THUMBSUP / HEART / DONE / OK / LGTM）
  --action   add（默认）/ delete
  --type     文件类型（默认 docx）

示例:
  feishu-cli comment reply react doccnXXX 6916106822734594568 --emoji THUMBSUP
  feishu-cli comment reply react doccnXXX 6916106822734594568 --emoji THUMBSUP --action delete`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		fileToken, replyID := args[0], args[1]
		fileType, _ := cmd.Flags().GetString("type")
		emoji, _ := cmd.Flags().GetString("emoji")
		action, _ := cmd.Flags().GetString("action")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		emoji = strings.TrimSpace(emoji)
		if _, ok := commentReactionTypes[emoji]; !ok {
			return clierr.Usagef("未知表情 --emoji %q（区分大小写，如 THUMBSUP / HEART / DONE / OK / LGTM）；服务端会把任意字符串存成无法显示的表情，因此本地校验", emoji)
		}
		action = strings.ToLower(strings.TrimSpace(action))
		if action != "add" && action != "delete" {
			return clierr.Usagef("--action 只能是 add 或 delete，得到 %q", action)
		}
		body := map[string]any{"action": action, "reaction_type": emoji, "reply_id": replyID}
		if dryRun {
			return printJSON(map[string]any{
				"dry_run": true,
				"method":  "POST",
				"path":    fmt.Sprintf("/open-apis/drive/v2/files/%s/comments/reaction", url.PathEscape(fileToken)),
				"query":   map[string]string{"file_type": fileType},
				"body":    body,
			})
		}

		userAccessToken := resolveOptionalUserToken(cmd)
		if err := client.ReactCommentReply(fileToken, fileType, replyID, emoji, action, userAccessToken); err != nil {
			return err
		}
		result := map[string]any{"file_token": fileToken, "reply_id": replyID, "reaction_type": emoji, "action": action, "updated": true}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("表情回应已更新！\n  回复 ID: %s\n  表情:    %s（%s）\n", replyID, emoji, action)
		return nil
	},
}

// commentReactionTypes 对齐平台 reaction_type 枚举（区分大小写）。服务端不校验该字段，
// 任意字符串都会被存成无法显示的表情，所以本地校验是唯一防线。
var commentReactionTypes = map[string]struct{}{
	"ANGRY": {}, "APPLAUSE": {}, "ATTENTION": {}, "AWESOME": {}, "BEAR": {}, "BEER": {},
	"BETRAYED": {}, "BIGKISS": {}, "BLACKFACE": {}, "BLUBBER": {}, "BLUSH": {}, "BOMB": {},
	"CAKE": {}, "CHUCKLE": {}, "CLAP": {}, "CLEAVER": {}, "COMFORT": {}, "CRAZY": {}, "CRY": {},
	"CUCUMBER": {}, "DETERGENT": {}, "DIZZY": {}, "DONE": {}, "DONNOTGO": {}, "DROOL": {},
	"DROWSY": {}, "DULL": {}, "DULLSTARE": {}, "EATING": {}, "EMBARRASSED": {}, "ENOUGH": {},
	"ERROR": {}, "EYESCLOSED": {}, "FACEPALM": {}, "FINGERHEART": {}, "FISTBUMP": {},
	"FOLLOWME": {}, "FROWN": {}, "GIFT": {}, "GLANCE": {}, "GOODJOB": {}, "HAMMER": {},
	"HAUGHTY": {}, "HEADSET": {}, "HEART": {}, "HEARTBROKEN": {}, "HIGHFIVE": {}, "HUG": {},
	"HUSKY": {}, "INNOCENTSMILE": {}, "JIAYI": {}, "JOYFUL": {}, "KISS": {}, "LAUGH": {},
	"LIPS": {}, "LOL": {}, "LOOKDOWN": {}, "LOVE": {}, "MONEY": {}, "MUSCLE": {},
	"NOSEPICK": {}, "OBSESSED": {}, "OK": {}, "PARTY": {}, "PETRIFIED": {}, "POOP": {},
	"PRAISE": {}, "PROUD": {}, "PUKE": {}, "RAINBOWPUKE": {}, "ROSE": {}, "SALUTE": {},
	"SCOWL": {}, "SHAKE": {}, "SHHH": {}, "SHOCKED": {}, "SHOWOFF": {}, "SHY": {}, "SICK": {},
	"SILENT": {}, "SKULL": {}, "SLAP": {}, "SLEEP": {}, "SLIGHT": {}, "SMART": {}, "SMILE": {},
	"SMIRK": {}, "SMOOCH": {}, "SMUG": {}, "SOB": {}, "SPEECHLESS": {}, "SPITBLOOD": {},
	"STRIVE": {}, "SWEAT": {}, "TEARS": {}, "TEASE": {}, "TERROR": {}, "THANKS": {},
	"THINKING": {}, "THUMBSUP": {}, "TOASTED": {}, "TONGUE": {}, "TRICK": {}, "UPPERLEFT": {},
	"WAIL": {}, "WAVE": {}, "WELLDONE": {}, "WHAT": {}, "WHIMPER": {}, "WINK": {}, "WITTY": {},
	"WOW": {}, "WRONGED": {}, "XBLUSH": {}, "YAWN": {}, "YEAH": {}, "FIREWORKS": {}, "BULL": {},
	"CALF": {}, "AWESOMEN": {}, "2021": {}, "CANDIEDHAWS": {}, "REDPACKET": {}, "FORTUNE": {},
	"LUCK": {}, "FIRECRACKER": {}, "Yes": {}, "No": {}, "Get": {}, "LGTM": {}, "Lemon": {},
	"EatingFood": {}, "Hundred": {}, "MinusOne": {}, "ThumbsDown": {}, "Fire": {}, "OKR": {},
	"Drumstick": {}, "BubbleTea": {}, "Loudspeaker": {}, "Pin": {}, "Coffee": {}, "Alarm": {},
	"Trophy": {}, "Music": {}, "Typing": {}, "Pepper": {}, "CheckMark": {}, "CrossMark": {},
}

func init() {
	commentCmd.AddCommand(getCommentCmd)
	getCommentCmd.Flags().String("type", "docx", "文件类型（doc/docx/sheet/bitable/file/slides）")
	getCommentCmd.Flags().StringP("output", "o", "", "输出格式（json）")

	commentCmd.AddCommand(batchGetCommentsCmd)
	batchGetCommentsCmd.Flags().String("type", "docx", "文件类型（doc/docx/sheet/bitable/file/slides）")
	batchGetCommentsCmd.Flags().StringSlice("comment-ids", nil, "评论 ID（逗号分隔或重复传入，最多 100 个）")
	batchGetCommentsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(batchGetCommentsCmd, "comment-ids")

	// --type 继承 replyCmd 的 persistent flag
	replyCmd.AddCommand(updateReplyCmd)
	updateReplyCmd.Flags().String("text", "", "纯文本内容（与 --content 二选一）")
	updateReplyCmd.Flags().String("content", "", "reply_elements JSON 数组（与 --text 二选一）")
	updateReplyCmd.Flags().Bool("dry-run", false, "仅打印请求，不实际修改")
	updateReplyCmd.Flags().StringP("output", "o", "", "输出格式（json）")

	replyCmd.AddCommand(reactReplyCmd)
	reactReplyCmd.Flags().String("emoji", "", "表情类型（必填，区分大小写，如 THUMBSUP）")
	reactReplyCmd.Flags().String("action", "add", "add / delete")
	reactReplyCmd.Flags().Bool("dry-run", false, "仅打印请求，不实际修改")
	reactReplyCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(reactReplyCmd, "emoji")
}
