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

// ============================================================================
// okr weight / reorder / indicator update / comment get|update|delete / objective batch-create
// 身份沿用 okr 全家桶的 --as（默认 bot）；评论修改与删除和 comment create 一致必需 User Token。
// ============================================================================

// readJSONTextFlag 读取内联 JSON 或 @文件 的原始文本（解析交给调用方）
func readJSONTextFlag(cmd *cobra.Command, name string) (string, error) {
	raw := strings.TrimSpace(flagString(cmd, name))
	if raw == "" {
		return "", clierr.Usagef("--%s 不能为空", name)
	}
	return readInlineOrAtFile(raw, name)
}

// readInlineOrAtFile 返回已 trim 的内联值；以 @ 开头时读取对应本地文件内容。
func readInlineOrAtFile(raw, flagName string) (string, error) {
	if !strings.HasPrefix(raw, "@") {
		return raw, nil
	}
	path := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if path == "" {
		return "", clierr.Usagef("--%s 的 @ 后需要文件路径", flagName)
	}
	b, err := readLocalInputFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// validateOKRPositiveID 校验正整数 ID（cycle_id / objective_id 等）
func validateOKRPositiveID(v, flag string) error {
	if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err != nil || n <= 0 {
		return clierr.Usagef("--%s 必须是正整数 ID，得到 %q", flag, v)
	}
	return nil
}

// okrLevelArgs 校验 --level / --cycle-id / --objective-id 组合
func okrLevelArgs(cmd *cobra.Command) (level, cycleID, objectiveID string, err error) {
	level = strings.TrimSpace(flagString(cmd, "level"))
	if err = validateEnum(level, "--level", []string{client.OKRLevelObjective, client.OKRLevelKeyResult}); err != nil {
		return
	}
	cycleID = strings.TrimSpace(flagString(cmd, "cycle-id"))
	objectiveID = strings.TrimSpace(flagString(cmd, "objective-id"))
	if level == client.OKRLevelObjective {
		err = validateOKRPositiveID(cycleID, "cycle-id")
		return
	}
	if objectiveID == "" {
		err = clierr.Usagef("--level key-result 需要 --objective-id")
		return
	}
	err = validateOKRPositiveID(objectiveID, "objective-id")
	return
}

// okrResolveToken 统一的凭据解析（dry-run 之后调用）
func okrResolveToken(cmd *cobra.Command, userRequired bool, action string) (string, error) {
	if err := config.Validate(); err != nil {
		return "", err
	}
	if userRequired {
		return requireUserToken(cmd, action)
	}
	return resolveIdentityToken(cmd)
}

var okrWeightCmd = &cobra.Command{
	Use:   "weight",
	Short: "调整 OKR 目标或关键结果的权重",
	Long: `调整周期内目标的权重，或目标下关键结果的权重。

只需给出要改的项：其余项按原权重比例分摊剩余权重（原权重全为 0 时均分），
三位小数定点计算保证总和恰为 1，再把完整列表 PUT 给服务端
（/okr/v2/cycles/{cycle_id}/objectives_weight 或 /okr/v2/objectives/{objective_id}/key_results_weight）。

参数:
  --level          objective | key-result（必填）
  --cycle-id       用户周期 ID（--level objective 必填）
  --objective-id   目标 ID（--level key-result 必填）
  --weights        JSON 数组（内联或 @文件）：[{"id":"7xxx","weight":0.4}]
                   每项 0..1、最多 3 位小数、id 不重复、总和 ≤ 1
  --dry-run        只预览（不联网；最终完整列表要读取现有项后才能算出）

权限: okr:okr.content:writeonly

示例:
  feishu-cli okr weight --level objective --cycle-id 7xxx --weights '[{"id":"7aaa","weight":0.4}]' --dry-run
  feishu-cli okr weight --level key-result --objective-id 7aaa --weights @weights.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		level, cycleID, objectiveID, err := okrLevelArgs(cmd)
		if err != nil {
			return err
		}
		raw, err := readJSONTextFlag(cmd, "weights")
		if err != nil {
			return err
		}
		ops, err := client.ParseOKRWeightOps(raw)
		if err != nil {
			return clierr.Usage(err)
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			preview := make([]client.OKRWeightResult, 0, len(ops))
			for _, op := range ops {
				preview = append(preview, client.OKRWeightResult{ID: op.ID, Weight: json.Number(strconv.FormatFloat(op.Weight, 'f', 3, 64))})
			}
			put := client.OKRWeightRequest(level, cycleID, objectiveID, preview)
			return printDryRunPlan(cmd, "okr weight 预览（未执行）", nil, []dryRunStep{
				{Method: "GET", URL: client.OKRLevelPath(level, cycleID, objectiveID), Desc: "读取现有项与原权重", Params: map[string]any{"page_size": 100}},
				{Method: put.Method, URL: put.Path, Desc: "提交完整权重列表：下面只列出指定项，其余项按原权重比例分摊剩余权重后一并提交", Body: put.Body},
			})
		}
		token, err := okrResolveToken(cmd, false, "okr weight")
		if err != nil {
			return err
		}
		items, err := client.ListOKRLevelItems(level, cycleID, objectiveID, token)
		if err != nil {
			return err
		}
		weights, err := client.NormalizeOKRWeights(items, ops)
		if err != nil {
			return clierr.Usage(err)
		}
		if _, err := client.DoOKRWrite(client.OKRWeightRequest(level, cycleID, objectiveID, weights), "更新 OKR 权重", token); err != nil {
			return err
		}
		if flagString(cmd, "output") == "json" {
			return printJSON(map[string]any{"level": level, "cycle_id": cycleID, "objective_id": objectiveID, "total": len(weights), "weights": weights})
		}
		fmt.Printf("已更新 %d 项权重:\n", len(weights))
		for _, w := range weights {
			fmt.Printf("  %s: %s\n", w.ID, w.Weight)
		}
		return nil
	},
}

var okrReorderCmd = &cobra.Command{
	Use:   "reorder",
	Short: "调整 OKR 目标或关键结果的顺序",
	Long: `调整周期内目标的顺序，或目标下关键结果的顺序。

只需给出要移动的项与目标位置（1 表示第一位），其余项保持原相对顺序依次填入空位，
再把完整 ID 列表 PUT 给服务端（objectives_position / key_results_position）。

参数:
  --level          objective | key-result（必填）
  --cycle-id       用户周期 ID（--level objective 必填）
  --objective-id   目标 ID（--level key-result 必填）
  --ops            JSON 数组（内联或 @文件）：[{"id":"7xxx","position":1}]，id 与 position 不能重复
  --dry-run        只预览（不联网；完整顺序要读取现有项后才能算出）

权限: okr:okr.content:writeonly

示例:
  feishu-cli okr reorder --level key-result --objective-id 7aaa --ops '[{"id":"7kr2","position":1}]' --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		level, cycleID, objectiveID, err := okrLevelArgs(cmd)
		if err != nil {
			return err
		}
		raw, err := readJSONTextFlag(cmd, "ops")
		if err != nil {
			return err
		}
		ops, err := client.ParseOKRReorderOps(raw)
		if err != nil {
			return clierr.Usage(err)
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			ids := make([]string, 0, len(ops))
			for _, op := range ops {
				ids = append(ids, op.ID)
			}
			put := client.OKRReorderRequest(level, cycleID, objectiveID, ids)
			return printDryRunPlan(cmd, "okr reorder 预览（未执行）", map[string]any{"ops": ops}, []dryRunStep{
				{Method: "GET", URL: client.OKRLevelPath(level, cycleID, objectiveID), Desc: "读取现有顺序", Params: map[string]any{"page_size": 100}},
				{Method: put.Method, URL: put.Path, Desc: "提交完整 ID 顺序：下面只列出被移动的项，其余项按原顺序填入空位后一并提交", Body: put.Body},
			})
		}
		token, err := okrResolveToken(cmd, false, "okr reorder")
		if err != nil {
			return err
		}
		items, err := client.ListOKRLevelItems(level, cycleID, objectiveID, token)
		if err != nil {
			return err
		}
		ids, err := client.BuildOKRReorderedIDs(items, ops)
		if err != nil {
			return clierr.Usage(err)
		}
		if _, err := client.DoOKRWrite(client.OKRReorderRequest(level, cycleID, objectiveID, ids), "调整 OKR 顺序", token); err != nil {
			return err
		}
		if flagString(cmd, "output") == "json" {
			return printJSON(map[string]any{"level": level, "cycle_id": cycleID, "objective_id": objectiveID, "total": len(ids), "order": ids})
		}
		fmt.Printf("已调整顺序（共 %d 项）:\n", len(ids))
		for i, id := range ids {
			fmt.Printf("  %d. %s\n", i+1, id)
		}
		return nil
	},
}

var okrIndicatorCmd = &cobra.Command{
	Use:   "indicator",
	Short: "OKR 量化指标",
	Long: `OKR 量化指标。

子命令:
  update   更新目标或关键结果的指标当前值`,
}

var okrIndicatorUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新 OKR 指标当前值",
	Long: `更新目标或关键结果绑定的量化指标当前值：先查询指标 ID，再 PATCH /okr/v2/indicators/{indicator_id}。

参数:
  --level     objective | key-result（必填）
  --id        目标或关键结果 ID（必填）
  --value     新的当前值（数字，-99999999999 ~ 99999999999）
  --dry-run   只预览，不执行

权限: okr:okr.content:writeonly

示例:
  feishu-cli okr indicator update --level key-result --id 7kr1 --value 42 --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		level := strings.TrimSpace(flagString(cmd, "level"))
		if err := validateEnum(level, "--level", []string{client.OKRLevelObjective, client.OKRLevelKeyResult}); err != nil {
			return err
		}
		id := strings.TrimSpace(flagString(cmd, "id"))
		if err := validateOKRPositiveID(id, "id"); err != nil {
			return err
		}
		value, err := client.ParseOKRIndicatorValue(flagString(cmd, "value"))
		if err != nil {
			return clierr.Usage(err)
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			patch := client.OKRIndicatorPatchRequest("<indicator_id>", value)
			return printDryRunPlan(cmd, "okr indicator update 预览（未执行）", nil, []dryRunStep{
				{Method: "GET", URL: client.OKRIndicatorListPath(level, id), Desc: "查询绑定的指标 ID", Params: map[string]any{"page_size": 100}},
				{Method: patch.Method, URL: "/open-apis/okr/v2/indicators/<indicator_id>", Desc: "更新指标当前值", Body: patch.Body},
			})
		}
		token, err := okrResolveToken(cmd, false, "okr indicator update")
		if err != nil {
			return err
		}
		indicatorID, err := client.GetOKRIndicatorID(level, id, token)
		if err != nil {
			return err
		}
		if _, err := client.DoOKRWrite(client.OKRIndicatorPatchRequest(indicatorID, value), "更新 OKR 指标", token); err != nil {
			return err
		}
		result := map[string]any{"indicator_id": indicatorID, "current_value": value, "level": level, "target_id": id}
		if flagString(cmd, "output") == "json" {
			return printJSON(result)
		}
		fmt.Printf("已更新指标 %s 当前值为 %v（%s %s）\n", indicatorID, value, level, id)
		return nil
	},
}

func okrCommentIDArg(args []string) (string, error) {
	id := strings.TrimSpace(args[0])
	if err := validateOKRPositiveID(id, "comment_id"); err != nil {
		return "", err
	}
	return id, nil
}

var okrCommentGetCmd = &cobra.Command{
	Use:   "get <comment_id>",
	Short: "获取一条 OKR 评论",
	Long: `获取单条评论（GET /open-apis/okr/v2/comments/{comment_id}），输出原始 JSON。

示例:
  feishu-cli okr comment get 7xxx --as user`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := okrCommentIDArg(args)
		if err != nil {
			return err
		}
		if err := validateUserIDType(flagString(cmd, "user-id-type")); err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		token, err := okrResolveToken(cmd, false, "okr comment get")
		if err != nil {
			return err
		}
		data, err := client.DoOKRWrite(&client.OKRWriteRequest{Method: "GET", Path: client.OKRCommentPath(id), Query: map[string]any{"user_id_type": okrUserIDTypeOrDefault(flagString(cmd, "user-id-type"))}}, "获取 OKR 评论", token)
		if err != nil {
			return err
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil || v == nil {
			v = map[string]any{}
		}
		return printJSON(v)
	},
}

var okrCommentUpdateCmd = &cobra.Command{
	Use:   "update <comment_id>",
	Short: "修改 OKR 评论（必需 User Token）",
	Long: `修改评论内容（PATCH /open-apis/okr/v2/comments/{comment_id}），必需 User Token。

参数:
  --content         新内容纯文本；或 --content-json 传 v2 ContentBlock
  --dry-run         只预览，不执行

示例:
  feishu-cli okr comment update 7xxx --content "口径已对齐" --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := okrCommentIDArg(args)
		if err != nil {
			return err
		}
		content, err := okrContentFlags(cmd, "content")
		if err != nil {
			return err
		}
		if content == nil {
			return clierr.Usagef("缺少评论内容（--content 或 --content-json）")
		}
		if err := validateUserIDType(flagString(cmd, "user-id-type")); err != nil {
			return err
		}
		req := &client.OKRWriteRequest{Method: "PATCH", Path: client.OKRCommentPath(id), Query: map[string]any{"user_id_type": okrUserIDTypeOrDefault(flagString(cmd, "user-id-type"))}, Body: map[string]any{"content": content}}
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return printDryRunPlan(cmd, "okr comment update 预览（未执行）", nil, []dryRunStep{{Method: req.Method, URL: req.Path, Params: req.Query, Body: req.Body}})
		}
		token, err := okrResolveToken(cmd, true, "okr comment update")
		if err != nil {
			return err
		}
		data, err := client.DoOKRWrite(req, "修改 OKR 评论", token)
		if err != nil {
			return err
		}
		return okrRenderID(cmd, "comment_id", "已修改评论")(data)
	},
}

var okrCommentDeleteCmd = &cobra.Command{
	Use:   "delete <comment_id>",
	Short: "删除 OKR 评论（必需 User Token）",
	Long: `删除评论（DELETE /open-apis/okr/v2/comments/{comment_id}），不可恢复，必需 User Token。

确认门禁: 交互终端会询问确认；非交互环境（AI Agent / 管道）未带 --yes 时退出码 10、不执行。

示例:
  feishu-cli okr comment delete 7xxx --dry-run
  feishu-cli okr comment delete 7xxx --yes`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := okrCommentIDArg(args)
		if err != nil {
			return err
		}
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return printDryRunPlan(cmd, "okr comment delete 预览（未执行）", nil, []dryRunStep{{Method: "DELETE", URL: client.OKRCommentPath(id), Desc: "删除评论（不可恢复）"}})
		}
		token, err := okrResolveToken(cmd, true, "okr comment delete")
		if err != nil {
			return err
		}
		if err := confirmDangerousAction(cmd, fmt.Sprintf("确认删除 OKR 评论 %s？此操作不可恢复", id)); err != nil {
			return err
		}
		if _, err := client.DoOKRWrite(&client.OKRWriteRequest{Method: "DELETE", Path: client.OKRCommentPath(id)}, "删除 OKR 评论", token); err != nil {
			return err
		}
		if flagString(cmd, "output") == "json" {
			return printJSON(map[string]any{"success": true, "comment_id": id})
		}
		fmt.Printf("已删除评论 %s\n", id)
		return nil
	},
}

var okrObjectiveBatchCreateCmd = &cobra.Command{
	Use:   "batch-create",
	Short: "批量创建 OKR 目标与关键结果（失败自动回滚）",
	Long: `在周期下批量创建目标及其关键结果，逐条调用创建接口；任一步失败时逆序删除本次已创建的目标
（关键结果随目标删除），错误信息列出回滚结果与需要手动清理的残留。

参数:
  --cycle-id      用户周期 ID（必填）
  --input         JSON 数组（内联或 @文件）：
                  [{"text":"目标","notes":"备注","category_id":"7xxx","krs":[{"text":"关键结果"}]}]
  --category-id   未单独指定 category_id 的目标使用的默认分类
  --dry-run       只预览，不执行

权限: okr:okr.content:writeonly

示例:
  feishu-cli okr objective batch-create --cycle-id 7xxx --input @okr.json --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cycleID := strings.TrimSpace(flagString(cmd, "cycle-id"))
		if err := validateOKRPositiveID(cycleID, "cycle-id"); err != nil {
			return err
		}
		category := strings.TrimSpace(flagString(cmd, "category-id"))
		if category != "" {
			if err := validateOKRPositiveID(category, "category-id"); err != nil {
				return err
			}
		}
		userIDType := flagString(cmd, "user-id-type")
		if err := validateUserIDType(userIDType); err != nil {
			return err
		}
		raw, err := readJSONTextFlag(cmd, "input")
		if err != nil {
			return err
		}
		objs, err := client.ParseOKRBatchObjectives(raw)
		if err != nil {
			return clierr.Usage(err)
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			reqs, err := client.OKRBatchPlan(cycleID, category, userIDType, objs)
			if err != nil {
				return clierr.Usage(err)
			}
			steps := make([]dryRunStep, 0, len(reqs))
			for _, r := range reqs {
				steps = append(steps, dryRunStep{Method: r.Method, URL: r.Path, Params: r.Query, Body: r.Body})
			}
			return printDryRunPlan(cmd, "okr objective batch-create 预览（未执行；失败时逆序 DELETE /open-apis/okr/v2/objectives/{id} 回滚）", nil, steps)
		}
		token, err := okrResolveToken(cmd, false, "okr objective batch-create")
		if err != nil {
			return err
		}
		created, err := client.BatchCreateOKRObjectives(cycleID, category, userIDType, objs, token)
		if err != nil {
			return err
		}
		if flagString(cmd, "output") == "json" {
			return printJSON(map[string]any{"created": created})
		}
		fmt.Printf("已创建 %d 个目标:\n", len(created))
		for i, c := range created {
			fmt.Printf("  [%d] 目标 %s（%d 个关键结果）\n", i+1, c.ObjectiveID, len(c.KeyResultIDs))
		}
		return nil
	},
}

func okrUserIDTypeOrDefault(t string) string {
	if strings.TrimSpace(t) == "" {
		return "open_id"
	}
	return t
}

func init() {
	okrCmd.AddCommand(okrWeightCmd, okrReorderCmd, okrIndicatorCmd)
	okrIndicatorCmd.AddCommand(okrIndicatorUpdateCmd)
	okrCommentCmd.AddCommand(okrCommentGetCmd, okrCommentUpdateCmd, okrCommentDeleteCmd)
	okrObjectiveCmd.AddCommand(okrObjectiveBatchCreateCmd)

	for _, c := range []*cobra.Command{okrWeightCmd, okrReorderCmd} {
		c.Flags().String("level", "", "objective | key-result（必填）")
		c.Flags().String("cycle-id", "", "用户周期 ID（--level objective 必填）")
		c.Flags().String("objective-id", "", "目标 ID（--level key-result 必填）")
	}
	okrWeightCmd.Flags().String("weights", "", `权重 JSON 数组（内联或 @文件）：[{"id":"...","weight":0.4}]`)
	okrReorderCmd.Flags().String("ops", "", `排序 JSON 数组（内联或 @文件）：[{"id":"...","position":1}]`)
	okrIndicatorUpdateCmd.Flags().String("level", "", "objective | key-result（必填）")
	okrIndicatorUpdateCmd.Flags().String("id", "", "目标或关键结果 ID（必填）")
	okrIndicatorUpdateCmd.Flags().String("value", "", "新的当前值（数字，必填）")
	okrObjectiveBatchCreateCmd.Flags().String("cycle-id", "", "用户周期 ID（必填）")
	okrObjectiveBatchCreateCmd.Flags().String("input", "", "目标列表 JSON 数组（内联或 @文件）")
	okrObjectiveBatchCreateCmd.Flags().String("category-id", "", "默认目标分类 ID")
	okrCommentUpdateCmd.Flags().String("content", "", "新内容纯文本（自动包装为 v2 ContentBlock）")
	okrCommentUpdateCmd.Flags().String("content-json", "", "新内容 v2 ContentBlock JSON（与 --content 二选一）")

	for _, c := range []*cobra.Command{okrWeightCmd, okrReorderCmd, okrIndicatorUpdateCmd, okrObjectiveBatchCreateCmd, okrCommentUpdateCmd, okrCommentDeleteCmd} {
		c.Flags().Bool("dry-run", false, "只预览请求，不执行")
	}
	for _, c := range []*cobra.Command{okrObjectiveBatchCreateCmd, okrCommentGetCmd, okrCommentUpdateCmd} {
		c.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id / union_id / user_id")
	}
	for _, c := range []*cobra.Command{okrWeightCmd, okrReorderCmd, okrIndicatorUpdateCmd, okrObjectiveBatchCreateCmd, okrCommentGetCmd, okrCommentUpdateCmd, okrCommentDeleteCmd} {
		c.Flags().StringP("output", "o", "", "输出格式（json）")
	}
}
