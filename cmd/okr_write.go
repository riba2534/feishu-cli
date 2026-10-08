package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var okrObjectiveCmd = &cobra.Command{
	Use:   "objective",
	Short: "OKR 目标（创建 / 更新）",
	Long: `OKR 目标写入（v2 接口）。

子命令:
  create   在周期下创建目标
  update   更新目标内容 / 备注 / 得分 / 截止时间

OKR 对组织可见，写入前先 --dry-run 预览。cycle_id 是 okr cycle list 默认返回的用户周期 ID。`,
}

var okrKeyResultCmd = &cobra.Command{
	Use:   "key-result",
	Short: "OKR 关键结果（创建 / 更新）",
	Long: `OKR 关键结果写入（v2 接口）。

子命令:
  create   在目标下创建关键结果
  update   更新关键结果内容 / 得分 / 截止时间`,
}

var okrCommentCmd = &cobra.Command{
	Use:   "comment",
	Short: "OKR 评论（列出 / 创建）",
	Long: `OKR 评论（v2 /okr/v2/comments）。

子命令:
  list     列出某个周期 / 进展 / 目标 / 关键结果上的评论
  create   创建评论或回复（必需 User Token）`,
}

// okrContentFlags 读取 --<name> / --<name>-json
func okrContentFlags(cmd *cobra.Command, name string) (map[string]any, error) {
	v, err := client.ParseOKRV2Content(flagString(cmd, name), flagString(cmd, name+"-json"), name)
	if err != nil {
		return nil, clierr.Usage(err)
	}
	return v, nil
}

// runOKRWrite dry-run（不联网）或执行 OKR 写请求
func runOKRWrite(cmd *cobra.Command, req *client.OKRWriteRequest, action string, userRequired bool, render func(data json.RawMessage) error) error {
	if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
		return printDryRunPlan(cmd, action+" 预览（未执行）", nil, []dryRunStep{{Method: req.Method, URL: req.Path, Params: req.Query, Body: req.Body}})
	}
	if err := config.Validate(); err != nil {
		return err
	}
	var token string
	var err error
	if userRequired {
		token, err = requireUserToken(cmd, "okr comment create")
	} else {
		token, err = resolveIdentityToken(cmd)
	}
	if err != nil {
		return err
	}
	data, err := client.DoOKRWrite(req, action, token)
	if err != nil {
		return err
	}
	return render(data)
}

func okrRenderID(cmd *cobra.Command, field, label string) func(json.RawMessage) error {
	return func(data json.RawMessage) error {
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		if flagString(cmd, "output") == "json" {
			if m == nil {
				m = map[string]any{}
			}
			return printJSON(m)
		}
		if id, _ := m[field].(string); id != "" {
			fmt.Printf("%s: %s\n", label, id)
		} else {
			fmt.Printf("%s成功\n", label)
		}
		return nil
	}
}

// parseOKRPatchFlags --score（0-1，一位小数）与 --deadline（毫秒或日期）
func parseOKRPatchFlags(cmd *cobra.Command, withNotes bool) (client.OKRPatchFields, error) {
	var f client.OKRPatchFields
	var err error
	if f.Content, err = okrContentFlags(cmd, "content"); err != nil {
		return f, err
	}
	if withNotes {
		if f.Notes, err = okrContentFlags(cmd, "notes"); err != nil {
			return f, err
		}
	}
	if s := strings.TrimSpace(flagString(cmd, "score")); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return f, clierr.Usagef("--score 需要 0-1 之间的数字")
		}
		f.Score = &v
	}
	if d := strings.TrimSpace(flagString(cmd, "deadline")); d != "" {
		var ms int64
		if n, err := strconv.ParseInt(d, 10, 64); err == nil {
			ms = n
		} else {
			t, err := client.ParseTimeInput(d, true)
			if err != nil {
				return f, clierr.Usagef("--deadline 需要毫秒时间戳或日期（YYYY-MM-DD）")
			}
			ms = t.UnixMilli()
		}
		f.Deadline = &ms
	}
	return f, nil
}

var okrObjectiveCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建 OKR 目标",
	Long: `在周期下创建目标（POST /open-apis/okr/v2/cycles/{cycle_id}/objectives）。

参数:
  --cycle-id        用户周期 ID（必填，okr cycle list 获取；不是 --tenant 的租户周期 ID）
  --content         目标内容纯文本（自动包装为 v2 ContentBlock）；或 --content-json 传完整 ContentBlock
  --notes           备注纯文本；或 --notes-json
  --category-id     目标分类 ID（租户强制分类时需要）
  --dry-run         只预览，不执行

权限: okr:okr.content:writeonly

示例:
  feishu-cli okr objective create --cycle-id 7xxx --content "提升交付质量" --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := okrContentFlags(cmd, "content")
		if err != nil {
			return err
		}
		notes, err := okrContentFlags(cmd, "notes")
		if err != nil {
			return err
		}
		req, err := client.BuildOKRCreateObjective(flagString(cmd, "cycle-id"), content, notes, flagString(cmd, "category-id"), flagString(cmd, "user-id-type"))
		if err != nil {
			return clierr.Usage(err)
		}
		return runOKRWrite(cmd, req, "创建 OKR 目标", false, okrRenderID(cmd, "objective_id", "已创建目标"))
	},
}

var okrObjectiveUpdateCmd = &cobra.Command{
	Use:   "update <objective_id>",
	Short: "更新 OKR 目标",
	Long: `更新目标（PATCH /open-apis/okr/v2/objectives/{objective_id}），只改传入的字段。

参数:
  --content / --content-json   新内容
  --notes / --notes-json       新备注
  --score                      得分 0-1，最多一位小数（如 0.5）
  --deadline                   截止时间：毫秒时间戳或 YYYY-MM-DD
  --dry-run                    只预览，不执行

示例:
  feishu-cli okr objective update 7xxx --score 0.7 --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := parseOKRPatchFlags(cmd, true)
		if err != nil {
			return err
		}
		req, err := client.BuildOKRPatch("objective", args[0], f, flagString(cmd, "user-id-type"))
		if err != nil {
			return clierr.Usage(err)
		}
		return runOKRWrite(cmd, req, "更新 OKR 目标", false, okrRenderID(cmd, "objective_id", "已更新目标"))
	},
}

var okrKeyResultCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建 OKR 关键结果",
	Long: `在目标下创建关键结果（POST /open-apis/okr/v2/objectives/{objective_id}/key_results）。

参数:
  --objective-id   目标 ID（必填）
  --content        关键结果内容纯文本；或 --content-json
  --dry-run        只预览，不执行

示例:
  feishu-cli okr key-result create --objective-id 7xxx --content "缺陷率降到 1% 以下" --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := okrContentFlags(cmd, "content")
		if err != nil {
			return err
		}
		req, err := client.BuildOKRCreateKeyResult(flagString(cmd, "objective-id"), content, flagString(cmd, "user-id-type"))
		if err != nil {
			return clierr.Usage(err)
		}
		return runOKRWrite(cmd, req, "创建 OKR 关键结果", false, okrRenderID(cmd, "key_result_id", "已创建关键结果"))
	},
}

var okrKeyResultUpdateCmd = &cobra.Command{
	Use:   "update <key_result_id>",
	Short: "更新 OKR 关键结果",
	Long: `更新关键结果（PATCH /open-apis/okr/v2/key_results/{key_result_id}），只改传入的字段。

参数:
  --content / --content-json   新内容
  --score                      得分 0-1，最多一位小数
  --deadline                   截止时间：毫秒时间戳或 YYYY-MM-DD
  --dry-run                    只预览，不执行

示例:
  feishu-cli okr key-result update 7xxx --content "缺陷率降到 0.5%" --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := parseOKRPatchFlags(cmd, false)
		if err != nil {
			return err
		}
		req, err := client.BuildOKRPatch("key-result", args[0], f, flagString(cmd, "user-id-type"))
		if err != nil {
			return clierr.Usage(err)
		}
		return runOKRWrite(cmd, req, "更新 OKR 关键结果", false, okrRenderID(cmd, "key_result_id", "已更新关键结果"))
	},
}

var okrCommentListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出 OKR 评论",
	Long: `列出评论（GET /open-apis/okr/v2/comments）。

参数:
  --target-type   cycle | progress | objective | key_result（必填）
  --target-id     目标对象 ID（必填）
  --page-token    分页标记
  --page-size     每页数量（1-100）

示例:
  feishu-cli okr comment list --target-type objective --target-id 7xxx -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tt, tid := flagString(cmd, "target-type"), flagString(cmd, "target-id")
		if err := validateEnum(tt, "--target-type", []string{"cycle", "progress", "objective", "key_result"}); err != nil {
			return err
		}
		if strings.TrimSpace(tid) == "" {
			return clierr.Usagef("--target-id 不能为空")
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		items, next, more, err := client.ListOKRComments(tt, tid, flagString(cmd, "page-token"), flagInt(cmd, "page-size"), flagString(cmd, "user-id-type"), token)
		if err != nil {
			return err
		}
		if more {
			fmt.Fprintf(cmdErrOut(), "提示：还有更多评论，用 --page-token %s 继续\n", next)
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		return printJSON(map[string]any{"comments": items, "page_token": next, "has_more": more})
	},
}

var okrCommentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建 OKR 评论（必需 User Token）",
	Long: `创建评论或回复（POST /open-apis/okr/v2/comments），必需 User Token。

参数:
  --target-type      cycle | progress | objective | key_result（必填）
  --target-id        目标对象 ID（必填）
  --content          评论纯文本；或 --content-json 传 v2 ContentBlock
  --selected-text    目标/关键结果评论的选中文本
  --select-all       目标/关键结果评论选中全文
  --ref-comment-id   回复的评论 ID
  （目标/关键结果评论必须且只能指定 --selected-text、--select-all、--ref-comment-id 之一）
  --dry-run          只预览，不执行

示例:
  feishu-cli okr comment create --target-type objective --target-id 7xxx \
    --content "这个目标的口径需要再对齐" --select-all --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := okrContentFlags(cmd, "content")
		if err != nil {
			return err
		}
		selectAll, _ := cmd.Flags().GetBool("select-all")
		req, err := client.BuildOKRCommentCreate(client.OKRCommentCreate{
			TargetType:   flagString(cmd, "target-type"),
			TargetID:     flagString(cmd, "target-id"),
			Content:      content,
			SelectedText: flagString(cmd, "selected-text"),
			SelectAll:    selectAll,
			RefCommentID: flagString(cmd, "ref-comment-id"),
			PlainText:    flagString(cmd, "content"),
		}, flagString(cmd, "user-id-type"))
		if err != nil {
			return clierr.Usage(err)
		}
		return runOKRWrite(cmd, req, "创建 OKR 评论", true, okrRenderID(cmd, "comment_id", "已创建评论"))
	},
}

func init() {
	okrCmd.AddCommand(okrObjectiveCmd, okrKeyResultCmd, okrCommentCmd)
	okrObjectiveCmd.AddCommand(okrObjectiveCreateCmd, okrObjectiveUpdateCmd)
	okrKeyResultCmd.AddCommand(okrKeyResultCreateCmd, okrKeyResultUpdateCmd)
	okrCommentCmd.AddCommand(okrCommentListCmd, okrCommentCreateCmd)

	for _, c := range []*cobra.Command{okrObjectiveCreateCmd, okrObjectiveUpdateCmd, okrKeyResultCreateCmd, okrKeyResultUpdateCmd, okrCommentCreateCmd} {
		c.Flags().String("content", "", "内容纯文本（自动包装为 v2 ContentBlock）")
		c.Flags().String("content-json", "", "内容 v2 ContentBlock JSON（与 --content 二选一）")
		c.Flags().Bool("dry-run", false, "只预览请求，不执行")
	}
	for _, c := range []*cobra.Command{okrObjectiveCreateCmd, okrObjectiveUpdateCmd, okrKeyResultCreateCmd, okrKeyResultUpdateCmd, okrCommentCreateCmd, okrCommentListCmd} {
		c.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id / union_id / user_id")
		c.Flags().StringP("output", "o", "", "输出格式（json）")
	}
	okrObjectiveCreateCmd.Flags().String("cycle-id", "", "用户周期 ID（必填）")
	okrObjectiveCreateCmd.Flags().String("category-id", "", "目标分类 ID")
	for _, c := range []*cobra.Command{okrObjectiveCreateCmd, okrObjectiveUpdateCmd} {
		c.Flags().String("notes", "", "备注纯文本")
		c.Flags().String("notes-json", "", "备注 v2 ContentBlock JSON")
	}
	for _, c := range []*cobra.Command{okrObjectiveUpdateCmd, okrKeyResultUpdateCmd} {
		c.Flags().String("score", "", "得分 0-1，最多一位小数")
		c.Flags().String("deadline", "", "截止时间：毫秒时间戳或 YYYY-MM-DD")
	}
	okrKeyResultCreateCmd.Flags().String("objective-id", "", "目标 ID（必填）")
	for _, c := range []*cobra.Command{okrCommentListCmd, okrCommentCreateCmd} {
		c.Flags().String("target-type", "", "cycle | progress | objective | key_result（必填）")
		c.Flags().String("target-id", "", "目标对象 ID（必填）")
	}
	okrCommentListCmd.Flags().String("page-token", "", "分页标记")
	okrCommentListCmd.Flags().Int("page-size", 0, "每页数量（1-100）")
	okrCommentCreateCmd.Flags().String("selected-text", "", "选中文本（目标/关键结果评论）")
	okrCommentCreateCmd.Flags().Bool("select-all", false, "选中全文（目标/关键结果评论）")
	okrCommentCreateCmd.Flags().String("ref-comment-id", "", "回复的评论 ID")
}
