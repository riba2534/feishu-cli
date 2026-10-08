package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var taskUploadAttachmentCmd = &cobra.Command{
	Use:   "upload-attachment",
	Short: "上传本地文件作为任务附件（单文件 ≤ 50MB）",
	Long: `把本地文件作为附件挂到指定 task_guid 下，省去先 drive upload 再手动关联的两步流程。
底层调用 /open-apis/task/v2/attachments/upload。

必填:
  --task-guid   目标任务 GUID
  --file        本地文件路径

可选:
  --resource-type   归属资源类型（默认 task；如挂到 task agent 用 task_delivery）
  --output / -o     输出格式（json）
  --user-access-token  覆盖登录态

权限:
  - User Access Token 或 Tenant Token
  - task:attachment:write

示例:
  feishu-cli task upload-attachment --task-guid xxxx --file ./report.pdf
  feishu-cli task upload-attachment --task-guid xxxx --file ./img.png -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		taskGuid, _ := cmd.Flags().GetString("task-guid")
		filePath, _ := cmd.Flags().GetString("file")
		resourceType, _ := cmd.Flags().GetString("resource-type")
		output, _ := cmd.Flags().GetString("output")

		if taskGuid == "" {
			return fmt.Errorf("--task-guid 必填")
		}
		if filePath == "" {
			return fmt.Errorf("--file 必填")
		}
		if resourceType == "" {
			resourceType = "task"
		}

		stat, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("读取文件失败: %w", err)
		}
		fmt.Fprintf(os.Stderr, "上传附件: %s (%d bytes) → task=%s\n", filepath.Base(filePath), stat.Size(), taskGuid)

		userToken, tokenErr := resolveIdentityToken(cmd)
		if tokenErr != nil {
			return tokenErr
		}
		info, err := client.UploadTaskAttachment(resourceType, taskGuid, filePath, userToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(info)
		}

		fmt.Printf("附件上传成功!\n")
		fmt.Printf("  名称:     %s\n", info.Name)
		fmt.Printf("  GUID:     %s\n", info.Guid)
		if info.Size > 0 {
			fmt.Printf("  大小:     %d bytes\n", info.Size)
		}
		if info.ResourceType != "" {
			fmt.Printf("  归属:     %s/%s\n", info.ResourceType, taskGuid)
		}
		if info.UploadedAt != "" {
			fmt.Printf("  上传时间: %s\n", info.UploadedAt)
		}
		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskUploadAttachmentCmd)
	taskUploadAttachmentCmd.Flags().String("task-guid", "", "任务 GUID（必填）")
	taskUploadAttachmentCmd.Flags().String("file", "", "本地文件路径（必填）")
	taskUploadAttachmentCmd.Flags().String("resource-type", "task", "归属资源类型（task / task_delivery）")
	taskUploadAttachmentCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskUploadAttachmentCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addWriteAsFlag(taskUploadAttachmentCmd)
	mustMarkFlagRequired(taskUploadAttachmentCmd, "task-guid")
	mustMarkFlagRequired(taskUploadAttachmentCmd, "file")
}
