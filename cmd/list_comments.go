package cmd

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var listCommentsCmd = &cobra.Command{
	Use:   "list <file_token>",
	Short: "列出文档评论",
	Long: `列出指定文档的评论（含正文与回复）。

参数:
  file_token        文档 Token
  --type            文件类型（必填: doc/docx/sheet/bitable/file/slides）
  --solved-status   解决状态过滤: all（默认，全部）/ false（仅未解决）/ true（仅已解决）
  --comment-scope   评论范围过滤: all（默认）/ whole（全文评论）/ partial（局部/划词评论）
  --page-size       每页数量（1-100，默认 50）
  --page-token      续翻上一次输出提示中的 page_token
  --page-all        自动翻页拉取全部评论（--page-limit 限制最多页数，默认 50，0 = 不限）

输出:
  JSON（-o json）为评论数组；每条评论的 content 为正文（根回复）可读文本，
  reply_list 原样保留服务端结构（replies[].content.elements 中的 text_run / docs_link / person）。
  还有更多评论时（has_more=true）在 stderr 提示续翻用的 page_token，stdout 保持纯 JSON。

身份说明:
  默认优先使用 auth login 的 User Token，不可用时告警并回退 App Token；
  若文档归个人所有且 App 未被加为协作者，App Token 会得到 1069303 forbidden。

示例:
  # 列出文档评论
  feishu-cli comment list doccnXXX --type docx

  # 只看未解决的评论，拉全量
  feishu-cli comment list doccnXXX --type docx --solved-status false --page-all

  # 续翻下一页
  feishu-cli comment list doccnXXX --type docx --page-token <page_token>

  # JSON 格式输出
  feishu-cli comment list doccnXXX --type docx --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		fileType, _ := cmd.Flags().GetString("type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		output, _ := cmd.Flags().GetString("output")
		solvedStatus, _ := cmd.Flags().GetString("solved-status")
		commentScope, _ := cmd.Flags().GetString("comment-scope")

		if pageSize < 1 || pageSize > 100 {
			return clierr.Usagef("--page-size 必须在 1-100 之间，得到 %d", pageSize)
		}
		isSolved, err := parseCommentSolvedStatus(solvedStatus)
		if err != nil {
			return err
		}
		isWhole, err := parseCommentScope(commentScope)
		if err != nil {
			return err
		}
		pageOpts, err := readListPageOptions(cmd)
		if err != nil {
			return err
		}
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		res, err := collectListPages(pageOpts, func(pageToken string) ([]*client.Comment, string, bool, error) {
			return client.ListCommentsWithOptions(client.ListCommentsOptions{
				FileToken: fileToken,
				FileType:  fileType,
				PageSize:  pageSize,
				PageToken: pageToken,
				IsSolved:  isSolved,
				IsWhole:   isWhole,
			}, userAccessToken)
		})
		if err != nil {
			return err
		}
		comments := res.Items

		if output == "json" {
			if comments == nil {
				comments = []*client.Comment{}
			}
			if err := printJSON(comments); err != nil {
				return err
			}
			printListPageHint(cmd.ErrOrStderr(), res)
			return nil
		}

		if len(comments) == 0 {
			fmt.Println("该文档暂无评论")
			printListPageHint(cmd.ErrOrStderr(), res)
			return nil
		}

		fmt.Printf("共找到 %d 条评论:\n\n", len(comments))
		for i, c := range comments {
			printCommentText(cmd.OutOrStdout(), i+1, c)
		}
		printListPageHint(cmd.ErrOrStderr(), res)
		return nil
	},
}

// parseCommentSolvedStatus 解析 --solved-status：all（不过滤）/ true / false。
func parseCommentSolvedStatus(v string) (*bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "all":
		return nil, nil
	case "true", "solved":
		b := true
		return &b, nil
	case "false", "unsolved":
		b := false
		return &b, nil
	}
	return nil, clierr.Usagef("--solved-status 只能是 all / true / false，得到 %q", v)
}

// parseCommentScope 解析 --comment-scope：all（不过滤）/ whole（全文评论）/ partial（局部评论）。
func parseCommentScope(v string) (*bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "all":
		return nil, nil
	case "whole":
		b := true
		return &b, nil
	case "partial", "local":
		b := false
		return &b, nil
	}
	return nil, clierr.Usagef("--comment-scope 只能是 all / whole / partial，得到 %q", v)
}

// printCommentText 以可读文本输出一条评论（正文 + 回复）。
func printCommentText(w io.Writer, idx int, c *client.Comment) {
	status := "未解决"
	if c.IsSolved {
		status = "已解决"
	}
	scope := "局部评论"
	if c.IsWhole {
		scope = "全文评论"
	}
	fmt.Fprintf(w, "[%d] 评论 ID: %s\n", idx, c.CommentID)
	fmt.Fprintf(w, "    状态:     %s\n", status)
	fmt.Fprintf(w, "    类型:     %s\n", scope)
	if c.UserID != "" {
		fmt.Fprintf(w, "    发起人:   %s\n", c.UserID)
	}
	if !c.IsWhole && c.Quote != "" {
		fmt.Fprintf(w, "    划词原文: %s\n", c.Quote)
	}
	if c.CreateTime > 0 {
		t := time.Unix(int64(c.CreateTime), 0)
		fmt.Fprintf(w, "    创建时间: %s\n", t.Format("2006-01-02 15:04:05"))
	}
	if c.Content != "" {
		fmt.Fprintf(w, "    正文:     %s\n", c.Content)
	}
	replies := c.Replies()
	if len(replies) > 1 {
		fmt.Fprintf(w, "    回复（%d）:\n", len(replies)-1)
		for _, r := range replies[1:] {
			who := r.UserID
			if who == "" {
				who = "-"
			}
			fmt.Fprintf(w, "      - [%s] %s: %s\n", r.ReplyID, who, r.Content)
		}
	}
	if c.ReplyHasMore {
		fmt.Fprintf(w, "    （回复未显示完整，用 feishu-cli comment reply list <file_token> %s --page-all 查看全部）\n", c.CommentID)
	}
	fmt.Fprintln(w)
}

func init() {
	commentCmd.AddCommand(listCommentsCmd)
	listCommentsCmd.Flags().String("type", "", "文件类型（必填: doc/docx/sheet/bitable/file/slides）")
	listCommentsCmd.Flags().Int("page-size", 50, "每页数量（1-100）")
	listCommentsCmd.Flags().String("solved-status", "all", "解决状态过滤: all / false（未解决）/ true（已解决）")
	listCommentsCmd.Flags().String("comment-scope", "all", "评论范围过滤: all / whole（全文）/ partial（局部）")
	addListPageFlags(listCommentsCmd)
	listCommentsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(listCommentsCmd, "type")
}
