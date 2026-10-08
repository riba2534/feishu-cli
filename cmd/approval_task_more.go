package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var approvalTaskRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "退回审批任务到指定节点",
	Long: `把审批任务退回到指定节点（POST /open-apis/approval/v4/tasks/rollback）。

高风险写操作：会通知相关审批人/发起人。先 --dry-run 预览；执行需确认（非交互环境加 --yes）。

权限:
  User Token，scope: approval:task:write

参数:
  --instance-code   审批实例 code（必填，task query 返回）
  --task-id         审批任务 ID（必填，与 instance-code 成对）
  --node-ids        退回目标节点 ID，逗号分隔（必填；发起节点为 START，可先 approval instance get 查节点）
  --comment         退回说明（可选，≤500 字）
  --dry-run         只预览请求，不执行

示例:
  feishu-cli approval task rollback --instance-code <ic> --task-id <task> \
    --node-ids START --comment "请补充附件后重新提交" --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ic, _ := cmd.Flags().GetString("instance-code")
		tid, _ := cmd.Flags().GetString("task-id")
		nodes, _ := cmd.Flags().GetString("node-ids")
		comment, _ := cmd.Flags().GetString("comment")
		opts := client.RollbackApprovalTaskOptions{InstanceCode: ic, TaskID: tid, NodeIDs: splitAndTrim(nodes), Comment: comment}
		body, err := client.BuildRollbackApprovalTaskBody(opts)
		if err != nil {
			return clierr.Usage(err)
		}
		return runApprovalHighRiskWrite(cmd, "approval task rollback", client.ApprovalTaskRollbackPath(), nil, body,
			fmt.Sprintf("将把审批任务 %s 退回到节点 %s（会通知相关人员）", tid, strings.Join(opts.NodeIDs, ",")),
			func(token string) error { return client.RollbackApprovalTask(opts, token) },
			fmt.Sprintf("已退回审批任务: %s", tid))
	},
}

var approvalTaskAddSignCmd = &cobra.Command{
	Use:   "add-sign",
	Short: "审批任务加签",
	Long: `给审批任务加签（POST /open-apis/approval/v4/tasks/add_sign）。

高风险写操作：会通知被加签人。先 --dry-run 预览；执行需确认（非交互环境加 --yes）。

权限:
  User Token，scope: approval:task:write

参数:
  --instance-code     审批实例 code（必填）
  --task-id           审批任务 ID（必填）
  --type              加签类型（必填）：before(1 前加签) | after(2 后加签) | parallel(3 并加签)
  --user-ids          被加签人 ID，逗号分隔（必填，类型由 --user-id-type 决定，默认 open_id）
  --approval-method   审批方式：or(1 或签) | and(2 会签) | sequential(3 依次审批)；
                      仅前/后加签需要，单人可省略（自动或签），多人必须指定；并加签不能传
  --comment           加签说明（可选）
  --user-id-type      open_id | user_id | union_id（默认 open_id）
  --dry-run           只预览请求，不执行

选择建议（对齐官方技能）:
  "先让某人审再我审"→before；"我审完再让某人审"→after；"拉进来一起审"→parallel。
  需要"加签后再把我这一环转交"时必须用 parallel，否则当前任务可能已流转、无法再转交。
  依次审批按 --user-ids 的顺序逐一审批。

示例:
  feishu-cli approval task add-sign --instance-code <ic> --task-id <task> \
    --type parallel --user-ids ou_xxx --comment "请一起审核" --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ic, _ := cmd.Flags().GetString("instance-code")
		tid, _ := cmd.Flags().GetString("task-id")
		typ, _ := cmd.Flags().GetString("type")
		users, _ := cmd.Flags().GetString("user-ids")
		method, _ := cmd.Flags().GetString("approval-method")
		comment, _ := cmd.Flags().GetString("comment")
		userIDType, _ := cmd.Flags().GetString("user-id-type")

		signType, ok := map[string]int{"before": 1, "1": 1, "after": 2, "2": 2, "parallel": 3, "3": 3}[strings.ToLower(strings.TrimSpace(typ))]
		if !ok {
			return clierr.Usagef("--type 仅支持 before | after | parallel（或 1/2/3），得到 %q", typ)
		}
		approvalMethod := 0
		if strings.TrimSpace(method) != "" {
			m, ok := map[string]int{"or": 1, "1": 1, "and": 2, "2": 2, "sequential": 3, "3": 3}[strings.ToLower(strings.TrimSpace(method))]
			if !ok {
				return clierr.Usagef("--approval-method 仅支持 or | and | sequential（或 1/2/3），得到 %q", method)
			}
			approvalMethod = m
		}
		opts := client.AddSignApprovalTaskOptions{
			InstanceCode: ic, TaskID: tid, AddSignType: signType, AddSignUserIDs: splitAndTrim(users),
			ApprovalMethod: approvalMethod, Comment: comment, UserIDType: userIDType,
		}
		body, idType, err := client.BuildAddSignApprovalTaskBody(opts)
		if err != nil {
			return clierr.Usage(err)
		}
		return runApprovalHighRiskWrite(cmd, "approval task add-sign", client.ApprovalTaskAddSignPath(), map[string]any{"user_id_type": idType}, body,
			fmt.Sprintf("将给审批任务 %s 加签 %d 人（会通知被加签人）", tid, len(opts.AddSignUserIDs)),
			func(token string) error { return client.AddSignApprovalTask(opts, token) },
			fmt.Sprintf("已为审批任务 %s 加签", tid))
	},
}

var approvalTaskRemindCmd = &cobra.Command{
	Use:   "remind",
	Short: "催办审批任务",
	Long: `催办审批实例中的指定任务（POST /open-apis/approval/v4/instances/remind）。

高风险写操作：会给审批人发催办通知。先 --dry-run 预览；执行需确认（非交互环境加 --yes）。

权限:
  User Token，scope: approval:instance:write

参数:
  --instance-code   审批实例 code（必填）
  --task-ids        被催办的任务 ID，逗号分隔（必填，须属于同一实例）
  --comment         催办说明（可选）
  --dry-run         只预览请求，不执行

示例:
  feishu-cli approval task remind --instance-code <ic> --task-ids <task> --comment "请尽快处理" --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ic, _ := cmd.Flags().GetString("instance-code")
		ids, _ := cmd.Flags().GetString("task-ids")
		comment, _ := cmd.Flags().GetString("comment")
		opts := client.RemindApprovalTaskOptions{InstanceCode: ic, TaskIDs: splitAndTrim(ids), Comment: comment}
		body, err := client.BuildRemindApprovalTaskBody(opts)
		if err != nil {
			return clierr.Usage(err)
		}
		return runApprovalHighRiskWrite(cmd, "approval task remind", client.ApprovalTaskRemindPath(), nil, body,
			fmt.Sprintf("将催办审批实例 %s 的 %d 个任务（会通知审批人）", ic, len(opts.TaskIDs)),
			func(token string) error { return client.RemindApprovalTask(opts, token) },
			fmt.Sprintf("已催办 %d 个审批任务", len(opts.TaskIDs)))
	},
}

// runApprovalHighRiskWrite 统一处理审批高风险写：dry-run（不联网、不解析身份）→ 确认门禁 → User Token → 执行
func runApprovalHighRiskWrite(cmd *cobra.Command, name, path string, params, body map[string]any, prompt string, do func(token string) error, success string) error {
	if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
		return printDryRunPlan(cmd, name+" 预览（未执行）", nil, []dryRunStep{{Method: "POST", URL: path, Params: params, Body: body}})
	}
	if err := confirmDangerousAction(cmd, prompt); err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	token, err := requireUserToken(cmd, name)
	if err != nil {
		return err
	}
	if err := do(token); err != nil {
		return err
	}
	fmt.Println(success)
	return nil
}

func init() {
	for _, c := range []*cobra.Command{approvalTaskRollbackCmd, approvalTaskAddSignCmd, approvalTaskRemindCmd} {
		approvalTaskCmd.AddCommand(c)
		c.Flags().String("instance-code", "", "审批实例 code（必填）")
		c.Flags().String("comment", "", "说明（可选，≤500 字）")
		c.Flags().Bool("dry-run", false, "只预览请求，不执行")
		c.Flags().StringP("output", "o", "", "dry-run 预览格式（json）")
		c.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	}
	approvalTaskRollbackCmd.Flags().String("task-id", "", "审批任务 ID（必填）")
	approvalTaskRollbackCmd.Flags().String("node-ids", "", "退回目标节点 ID，逗号分隔（发起节点为 START）")
	mustMarkFlagRequired(approvalTaskRollbackCmd, "instance-code", "task-id", "node-ids")

	approvalTaskAddSignCmd.Flags().String("task-id", "", "审批任务 ID（必填）")
	approvalTaskAddSignCmd.Flags().String("type", "", "加签类型：before | after | parallel（必填）")
	approvalTaskAddSignCmd.Flags().String("user-ids", "", "被加签人 ID，逗号分隔（必填）")
	approvalTaskAddSignCmd.Flags().String("approval-method", "", "审批方式：or | and | sequential（仅前/后加签）")
	approvalTaskAddSignCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id/user_id/union_id")
	mustMarkFlagRequired(approvalTaskAddSignCmd, "instance-code", "task-id", "type", "user-ids")

	approvalTaskRemindCmd.Flags().String("task-ids", "", "被催办的任务 ID，逗号分隔（必填）")
	mustMarkFlagRequired(approvalTaskRemindCmd, "instance-code", "task-ids")
}

// approvalWriteDryRun 审批写操作的预览：只构造请求，不联网、不解析身份
func approvalWriteDryRun(cmd *cobra.Command, name, action string, opts any) error {
	p, err := client.PreviewApprovalWrite(action, opts)
	if err != nil {
		return clierr.Usage(err)
	}
	return printDryRunPlan(cmd, name+" 预览（未执行）", nil, []dryRunStep{{Method: "POST", URL: p.Path, Params: p.Params, Body: p.Body}})
}
