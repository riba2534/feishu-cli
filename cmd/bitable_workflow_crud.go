package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// ==================== workflow create / get / update ====================
// 端点 ground truth（已实测印证，均走 base/v3）：
//   POST /open-apis/base/v3/bases/{base_token}/workflows                 body {"title":...,"steps":[...]}
//   GET  /open-apis/base/v3/bases/{base_token}/workflows/{workflow_id}   ?user_id_type=
//   PUT  /open-apis/base/v3/bases/{base_token}/workflows/{workflow_id}   body {"title":...,"steps":[...]}
// 注意 update 是 PUT（整体替换定义），不是 PATCH；启停才是 PATCH .../enable|disable。

// bitableWorkflowConfigBody 解析 --config/--config-file 为 workflow 定义体（POST/PUT 共用）。
func bitableWorkflowConfigBody(cmd *cobra.Command) (any, error) {
	configJSON, _ := cmd.Flags().GetString("config")
	configFile, _ := cmd.Flags().GetString("config-file")
	raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "workflow 定义")
	if err != nil {
		return nil, err
	}
	var body any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, fmt.Errorf("解析 --config 失败: %w", err)
	}
	return body, nil
}

var bitableWorkflowCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建工作流",
	Long: `POST /open-apis/base/v3/bases/{base_token}/workflows

用 --config/--config-file 传完整 workflow 定义，形如 {"title":"My Workflow","steps":[...]}。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := bitableWorkflowConfigBody(cmd)
		if err != nil {
			return err
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: client.BaseV3Path("bases", bt, "workflows"), body: body}
		})
	},
}

var bitableWorkflowGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取单个工作流定义（含 steps）",
	Long: `GET /open-apis/base/v3/bases/{base_token}/workflows/{workflow_id}

可选:
  --user-id-type   creator/updater 字段的用户 ID 类型（open_id/union_id/user_id）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		workflowID, _ := cmd.Flags().GetString("workflow-id")
		if workflowID == "" {
			return fmt.Errorf("--workflow-id 必填")
		}
		params := map[string]any{}
		if uit, _ := cmd.Flags().GetString("user-id-type"); uit != "" {
			params["user_id_type"] = uit
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "GET", path: client.BaseV3Path("bases", bt, "workflows", workflowID), params: params}
		})
	},
}

var bitableWorkflowUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "整体替换工作流定义（title 和/或 steps）",
	Long: `PUT /open-apis/base/v3/bases/{base_token}/workflows/{workflow_id}

用 --config/--config-file 传完整 workflow 定义，形如 {"title":"New Title","steps":[...]}。
注意：这是整体替换（PUT），未提供的字段不会保留。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		workflowID, _ := cmd.Flags().GetString("workflow-id")
		if workflowID == "" {
			return fmt.Errorf("--workflow-id 必填")
		}
		body, err := bitableWorkflowConfigBody(cmd)
		if err != nil {
			return err
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "PUT", path: client.BaseV3Path("bases", bt, "workflows", workflowID), body: body}
		})
	},
}

func init() {
	// 挂到 bitable_misc.go 中定义的 bitableWorkflowCmd 组
	bitableWorkflowCmd.AddCommand(bitableWorkflowCreateCmd)
	addBitableWriteFlags(bitableWorkflowCreateCmd)
	bitableWorkflowCreateCmd.Flags().String("config", "", "workflow 定义 JSON（与 --config-file 二选一）")
	bitableWorkflowCreateCmd.Flags().String("config-file", "", "workflow 定义 JSON 文件")

	bitableWorkflowCmd.AddCommand(bitableWorkflowGetCmd)
	// get 是只读命令，但注册 --dry-run 以便预览请求。
	addBitableWriteFlags(bitableWorkflowGetCmd)
	bitableWorkflowGetCmd.Flags().String("workflow-id", "", "workflow_id（wkf 前缀，必填）")
	bitableWorkflowGetCmd.Flags().String("user-id-type", "", "用户 ID 类型（open_id/union_id/user_id）")

	bitableWorkflowCmd.AddCommand(bitableWorkflowUpdateCmd)
	addBitableWriteFlags(bitableWorkflowUpdateCmd)
	bitableWorkflowUpdateCmd.Flags().String("workflow-id", "", "workflow_id（wkf 前缀，必填）")
	bitableWorkflowUpdateCmd.Flags().String("config", "", "workflow 定义 JSON（与 --config-file 二选一）")
	bitableWorkflowUpdateCmd.Flags().String("config-file", "", "workflow 定义 JSON 文件")
}
