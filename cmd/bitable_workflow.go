package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// ==================== workflow enable/disable/update ====================
// base/v3 仅暴露 workflows/list；启停走 bitable/v1 PUT apps/{app_token}/workflows/{workflow_id}
// （文档明确：body status="Enable"|"Disable"，SDK appWorkflow.Update 同此），app_token 即 base_token。

func bitableWorkflowUpdate(cmd *cobra.Command, status string) error {
	workflowID, _ := cmd.Flags().GetString("workflow-id")
	if workflowID == "" {
		return fmt.Errorf("--workflow-id 必填")
	}
	body := map[string]any{"status": status}
	return bitableRun(cmd, func(bt string) bitableReq {
		return bitableReq{method: "PUT", path: client.BitableV1Path("apps", bt, "workflows", workflowID), body: body, useV1: true}
	})
}

var bitableWorkflowEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "启用工作流",
	Long:  `PUT /open-apis/bitable/v1/apps/{app_token}/workflows/{workflow_id}（status=Enable）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return bitableWorkflowUpdate(cmd, "Enable")
	},
}

var bitableWorkflowDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "停用工作流",
	Long:  `PUT /open-apis/bitable/v1/apps/{app_token}/workflows/{workflow_id}（status=Disable）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return bitableWorkflowUpdate(cmd, "Disable")
	},
}

func init() {
	// 挂到 bitable_misc.go 中定义的 bitableWorkflowCmd 组
	bitableWorkflowCmd.AddCommand(bitableWorkflowEnableCmd)
	addBitableWriteFlags(bitableWorkflowEnableCmd)
	bitableWorkflowEnableCmd.Flags().String("workflow-id", "", "workflow_id（必填）")

	bitableWorkflowCmd.AddCommand(bitableWorkflowDisableCmd)
	addBitableWriteFlags(bitableWorkflowDisableCmd)
	bitableWorkflowDisableCmd.Flags().String("workflow-id", "", "workflow_id（必填）")
}
