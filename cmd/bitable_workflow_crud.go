package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
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
	if m, ok := body.(map[string]any); ok {
		if err := validateWorkflowAISteps(m); err != nil {
			return nil, err
		}
	}
	return body, nil
}

// validateWorkflowAISteps 本地校验 AI 步骤里最常写错的结构（对齐官方 workflow_json_validation.go 的轻量子集），
// 在服务端报晦涩的校验错误之前给出明确提示：
//   - AIAnalysisAction：data.analysis_table_names 必须是字符串数组；data.identity_type ∈ maker|triggerPersonal
//   - AIClassificationBranch：data 必须是对象、不支持 data.mode（只有互斥模式）、data.classes 至少 2 项，
//     每项 name 非空不重复不含换行、desc 为字符串
func validateWorkflowAISteps(body map[string]any) error {
	steps, ok := body["steps"].([]any)
	if !ok {
		return nil
	}
	for i, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		stepType, _ := step["type"].(string)
		path := fmt.Sprintf("steps[%d].data", i)
		switch stepType {
		case "AIAnalysisAction":
			data, ok := step["data"].(map[string]any)
			if !ok {
				continue
			}
			if v, exists := data["analysis_table_names"]; exists {
				items, ok := v.([]any)
				if !ok {
					return clierr.Usagef("%s.analysis_table_names 必须是字符串数组", path)
				}
				for j, item := range items {
					if _, ok := item.(string); !ok {
						return clierr.Usagef("%s.analysis_table_names[%d] 必须是字符串", path, j)
					}
				}
			}
			if v, exists := data["identity_type"]; exists {
				if s, _ := v.(string); s != "maker" && s != "triggerPersonal" {
					return clierr.Usagef("%s.identity_type 只能是 maker 或 triggerPersonal", path)
				}
			}
		case "AIClassificationBranch":
			data, ok := step["data"].(map[string]any)
			if !ok || data == nil {
				return clierr.Usagef("%s 必须是对象（AIClassificationBranch）", path)
			}
			if _, exists := data["mode"]; exists {
				return clierr.Usagef("%s.mode 不受支持：AI 分类只有互斥模式，请去掉 mode", path)
			}
			classes, ok := data["classes"].([]any)
			if !ok {
				return clierr.Usagef("%s.classes 必须是数组", path)
			}
			if len(classes) < 2 {
				return clierr.Usagef("%s.classes 至少需要 2 个分类", path)
			}
			seen := map[string]int{}
			for j, rc := range classes {
				item, ok := rc.(map[string]any)
				if !ok {
					return clierr.Usagef("%s.classes[%d] 必须是对象", path, j)
				}
				name, _ := item["name"].(string)
				name = strings.TrimSpace(name)
				if name == "" {
					return clierr.Usagef("%s.classes[%d].name 不能为空", path, j)
				}
				if strings.ContainsAny(name, "\r\n") {
					return clierr.Usagef("%s.classes[%d].name 不能包含换行", path, j)
				}
				if prev, dup := seen[name]; dup {
					return clierr.Usagef("%s.classes[%d].name 与 classes[%d] 重复: %q", path, j, prev, name)
				}
				if _, ok := item["desc"].(string); !ok {
					return clierr.Usagef("%s.classes[%d].desc 必须是字符串", path, j)
				}
				seen[name] = j
			}
		}
	}
	return nil
}

var bitableWorkflowCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建工作流",
	Long: `POST /open-apis/base/v3/bases/{base_token}/workflows

用 --config/--config-file 传完整 workflow 定义，形如 {"title":"My Workflow","steps":[...]}。

本地预检 AI 步骤：AIAnalysisAction 的 analysis_table_names / identity_type，
AIClassificationBranch 的 classes（≥2 个、名称唯一）与不支持的 mode。
提醒触发器 ReminderTrigger 的 offset：触发时间 = 日期字段 + offset × unit，负数=提前、正数=延后。`,
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
