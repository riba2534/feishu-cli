package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// ==================== workflow enable/disable ====================
// 启停工作流走 base/v3 动作端点：
//   PATCH /open-apis/base/v3/bases/{base_token}/workflows/{workflow_id}/enable
//   PATCH /open-apis/base/v3/bases/{base_token}/workflows/{workflow_id}/disable
// 无请求体；bitable/v1 没有对应的启停端点（早前用 PUT apps/{token}/workflows/{id}+status 是错误路径）。

func bitableWorkflowToggle(cmd *cobra.Command, action string) error {
	workflowID, _ := cmd.Flags().GetString("workflow-id")
	if workflowID == "" {
		return fmt.Errorf("--workflow-id 必填")
	}
	return bitableRun(cmd, func(bt string) bitableReq {
		return bitableReq{method: "PATCH", path: client.BaseV3Path("bases", bt, "workflows", workflowID, action)}
	})
}

var bitableWorkflowEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "启用工作流",
	Long:  `PATCH /open-apis/base/v3/bases/{base_token}/workflows/{workflow_id}/enable`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return bitableWorkflowToggle(cmd, "enable")
	},
}

var bitableWorkflowDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "停用工作流",
	Long:  `PATCH /open-apis/base/v3/bases/{base_token}/workflows/{workflow_id}/disable`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return bitableWorkflowToggle(cmd, "disable")
	},
}

func init() {
	// 挂到 bitable_misc.go 中定义的 bitableWorkflowCmd 组
	bitableWorkflowCmd.AddCommand(bitableWorkflowEnableCmd)
	addBitableWriteFlags(bitableWorkflowEnableCmd)
	bitableWorkflowEnableCmd.Flags().String("workflow-id", "", "workflow_id（wkf 前缀，必填）")

	bitableWorkflowCmd.AddCommand(bitableWorkflowDisableCmd)
	addBitableWriteFlags(bitableWorkflowDisableCmd)
	bitableWorkflowDisableCmd.Flags().String("workflow-id", "", "workflow_id（wkf 前缀，必填）")
}
