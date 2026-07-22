package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// wikiMoveToDrivePollInterval 是轮询 move_wiki_to_docs 任务状态的间隔。
const wikiMoveToDrivePollInterval = 2 * time.Second

var wikiMoveToDriveCmd = &cobra.Command{
	Use:   "move-to-drive",
	Short: "将知识库节点移出知识空间到云盘文件夹（异步任务，自动轮询）",
	Long: `将知识库（wiki）节点移出知识空间，转存到云盘（我的空间/共享空间）文件夹。这是
wiki move-docs（云盘 → 知识库）的反向操作。

移动后：
  - 节点从知识库树中移除，出现在目标云盘文件夹
  - 节点原本继承的知识库权限被目标云盘文件夹的权限模型替换
  - 该接口始终异步执行，返回 task_id；本命令默认轮询直至成功 / 失败 / 超时

必填:
  --node-token          要移出的知识库节点 node_token（不是底层文档 obj_token）

可选:
  --folder-token        目标云盘文件夹 token；省略则移动到调用方个人空间根目录
  --wait                是否轮询等待任务完成（默认 true；传 --wait=false 时提交后立即返回 task_id）
  --timeout             轮询最长等待秒数（默认 60）
  --output / -o         输出格式（json）
  --user-access-token   User Access Token（覆盖登录态；移动到个人空间根目录通常需要用户身份）

权限:
  - wiki:node:move 或 wiki:wiki（tenant 或 user 身份）
  - wiki:space:read（查询任务状态）
  - 调用方须对源节点有编辑权限、对目标文件夹有编辑权限

提示:
  - --node-token 必须是知识库 node_token（形如 wikcnXXXX），不是底层文档 token；
    不确定时先用 feishu-cli wiki nodes 查询
  - 移动到个人空间根目录（省略 --folder-token）通常需要用户身份，请配合 --user-access-token

示例:
  # 移动到指定云盘文件夹
  feishu-cli wiki move-to-drive --node-token wikcnXXXX --folder-token fldcnYYYY

  # 移动到个人空间根目录（用户身份）
  feishu-cli wiki move-to-drive --node-token wikcnXXXX --user-access-token u-xxx

  # 只提交不等待，拿到 task_id 后自行查询
  feishu-cli wiki move-to-drive --node-token wikcnXXXX --folder-token fldcnYYYY --wait=false`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		nodeToken, _ := cmd.Flags().GetString("node-token")
		folderToken, _ := cmd.Flags().GetString("folder-token")
		wait, _ := cmd.Flags().GetBool("wait")
		timeout, _ := cmd.Flags().GetInt("timeout")
		output, _ := cmd.Flags().GetString("output")

		if nodeToken == "" {
			return fmt.Errorf("--node-token 必填")
		}
		if timeout <= 0 {
			timeout = 60
		}

		userToken := resolveOptionalUserToken(cmd)

		folderLabel := "个人空间根目录"
		if folderToken != "" {
			folderLabel = folderToken
		}
		fmt.Fprintf(os.Stderr, "移动知识库节点 %s → 云盘 %s ...\n", nodeToken, folderLabel)

		taskID, err := client.MoveWikiNodeToDrive(nodeToken, folderToken, userToken)
		if err != nil {
			return err
		}

		result := map[string]any{
			"node_token":   nodeToken,
			"folder_token": folderToken,
			"task_id":      taskID,
			"ready":        false,
			"failed":       false,
			"status":       client.WikiMoveToDriveStatusProcessing,
		}

		if !wait {
			result["status_msg"] = "processing"
			fmt.Fprintf(os.Stderr, "已提交异步任务 task_id=%s（--wait=false，未等待）\n", taskID)
			return printWikiMoveToDriveResult(result, output)
		}

		fmt.Fprintf(os.Stderr, "已提交异步任务 task_id=%s，开始轮询（最长 %ds）...\n", taskID, timeout)
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		status, ready, err := pollWikiMoveToDriveTask(ctx, taskID, userToken, timeout)
		if err != nil {
			return err
		}
		result["ready"] = ready
		result["failed"] = status.Failed()
		result["status"] = status.Status
		result["status_msg"] = status.StatusMsg
		result["obj_token"] = status.ObjToken
		result["obj_type"] = status.ObjType
		result["url"] = status.URL
		if !ready {
			result["timed_out"] = true
		}
		return printWikiMoveToDriveResult(result, output)
	},
}

// pollWikiMoveToDriveTask 在 timeoutSeconds 秒内轮询任务状态，直至成功 / 失败 / 超时。
// 至少查询一次；超时后返回最后一次已知状态且 ready=false。
func pollWikiMoveToDriveTask(ctx context.Context, taskID, userToken string, timeoutSeconds int) (*client.WikiMoveToDriveTaskStatus, bool, error) {
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	var last *client.WikiMoveToDriveTaskStatus
	attempt := 0
	for {
		attempt++
		st, err := client.GetMoveWikiToDriveTask(taskID, userToken)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [%d] 查询失败: %v\n", attempt, err)
		} else {
			last = st
			if st.Ready() {
				fmt.Fprintf(os.Stderr, "任务完成 ✅\n")
				return st, true, nil
			}
			if st.Failed() {
				return st, false, fmt.Errorf("move_wiki_to_docs 任务失败: status=%d, msg=%s", st.Status, st.StatusMsg)
			}
			fmt.Fprintf(os.Stderr, "  [%d] status=%d %s\n", attempt, st.Status, st.StatusMsg)
		}
		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return last, false, ctx.Err()
		case <-time.After(wikiMoveToDrivePollInterval):
		}
	}
	if last == nil {
		last = &client.WikiMoveToDriveTaskStatus{TaskID: taskID, Status: client.WikiMoveToDriveStatusProcessing}
	}
	return last, false, nil
}

func printWikiMoveToDriveResult(result map[string]any, output string) error {
	if output == "json" {
		return printJSON(result)
	}
	fmt.Printf("node_token:   %s\n", result["node_token"])
	if v, ok := result["folder_token"]; ok && v != "" {
		fmt.Printf("folder_token: %s\n", v)
	} else {
		fmt.Printf("folder_token: (个人空间根目录)\n")
	}
	fmt.Printf("task_id:      %s\n", result["task_id"])
	fmt.Printf("ready:        %v\n", result["ready"])
	fmt.Printf("status:       %v\n", result["status"])
	if v, ok := result["status_msg"]; ok && v != "" {
		fmt.Printf("status_msg:   %v\n", v)
	}
	if v, ok := result["obj_token"]; ok && v != "" {
		fmt.Printf("obj_token:    %v\n", v)
	}
	if v, ok := result["url"]; ok && v != "" {
		fmt.Printf("url:          %v\n", v)
	}
	if v, ok := result["timed_out"]; ok {
		if b, _ := v.(bool); b {
			fmt.Printf("⚠ 轮询超时，任务可能仍在执行，可稍后凭 task_id 重新查询状态\n")
		}
	}
	return nil
}

func init() {
	wikiCmd.AddCommand(wikiMoveToDriveCmd)
	wikiMoveToDriveCmd.Flags().String("node-token", "", "要移出的知识库节点 node_token（必填）")
	wikiMoveToDriveCmd.Flags().String("folder-token", "", "目标云盘文件夹 token（省略则移动到个人空间根目录）")
	wikiMoveToDriveCmd.Flags().Bool("wait", true, "是否轮询等待任务完成（默认 true）")
	wikiMoveToDriveCmd.Flags().Int("timeout", 60, "轮询最长等待秒数（默认 60）")
	wikiMoveToDriveCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	wikiMoveToDriveCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(wikiMoveToDriveCmd, "node-token")
}
