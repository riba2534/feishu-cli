package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var moveDocsToWikiCmd = &cobra.Command{
	Use:   "move-docs <obj_token>",
	Short: "移动云空间文档至知识空间",
	Long: `将已存在于云空间（我的空间/共享空间）的文档挂载到指定知识空间。

移动后：
  - 文档从云空间相关入口（快速访问、我的空间、共享空间）消失
  - 文档权限默认继承父页面
  - 大文档会异步执行，返回 task_id 供查询
  - 无权限但 --apply 时会提交迁入申请

参数:
  obj_token             云空间文档 token（必填）
  --space-id            目标知识空间 ID（必填）
  --obj-type            文档类型：docx/doc/sheet/mindnote/bitable/file（默认 docx）
  --parent-node         目标父节点 Token（可选，不指定则挂在空间根目录）
  --apply               无权限时提交迁入申请（默认 false）
  --user-access-token   User Access Token（可选，用于访问个人知识库）

权限要求:
  - 需要 wiki:node:move 或 wiki:wiki scope（tenant 或 user 身份）
  - 调用方须是源文档编辑者 + 目标知识空间成员（或目标父节点容器编辑者）
  - 企业版 wiki 空间只接受 userid/openid/openchat/department 作为成员，
    不接受 app 作为成员——此时必须使用 --user-access-token 走用户身份

示例:
  # 把 drive 里的 docx 移入知识空间根目录
  feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567

  # 移入指定父节点下
  feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567 --parent-node wikcnYYYYYY

  # 移动电子表格
  feishu-cli wiki move-docs shtcnXXXXXX --space-id 7012345678901234567 --obj-type sheet

  # 无权限时自动提交迁入申请
  feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567 --apply

  # 使用用户身份（app 不是知识空间成员时需要）
  feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567 --user-access-token u-xxx

  # JSON 格式输出
  feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		objToken := args[0]
		spaceID, _ := cmd.Flags().GetString("space-id")
		objType, _ := cmd.Flags().GetString("obj-type")
		parentNode, _ := cmd.Flags().GetString("parent-node")
		apply, _ := cmd.Flags().GetBool("apply")
		output, _ := cmd.Flags().GetString("output")

		result, err := client.MoveDocsToWiki(
			spaceID, objType, objToken, parentNode, apply,
			resolveOptionalUserToken(cmd),
		)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		switch {
		case result.WikiToken != "":
			fmt.Printf("云空间文档已移入知识空间！\n")
			fmt.Printf("  Wiki Token:   %s\n", result.WikiToken)
			fmt.Printf("  空间 ID:      %s\n", spaceID)
			if parentNode != "" {
				fmt.Printf("  父节点:       %s\n", parentNode)
			} else {
				fmt.Printf("  目标位置:     空间根目录\n")
			}
		case result.TaskID != "":
			fmt.Printf("文档较大，已提交异步任务。\n")
			fmt.Printf("  Task ID:      %s\n", result.TaskID)
			fmt.Printf("  说明:         可通过飞书开放平台任务查询接口轮询执行结果\n")
		case result.Applied:
			fmt.Printf("权限不足，已提交迁入申请。\n")
			fmt.Printf("  说明:         等待知识空间管理员审批\n")
		default:
			fmt.Printf("调用成功但响应为空，请稍后检查目标知识空间。\n")
		}

		return nil
	},
}

func init() {
	wikiCmd.AddCommand(moveDocsToWikiCmd)
	moveDocsToWikiCmd.Flags().String("space-id", "", "目标知识空间 ID（必填）")
	moveDocsToWikiCmd.Flags().String("obj-type", "docx", "文档类型：docx/doc/sheet/mindnote/bitable/file")
	moveDocsToWikiCmd.Flags().String("parent-node", "", "目标父节点 Token（可选）")
	moveDocsToWikiCmd.Flags().Bool("apply", false, "无权限时提交迁入申请")
	moveDocsToWikiCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	moveDocsToWikiCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问个人知识库）")
	mustMarkFlagRequired(moveDocsToWikiCmd, "space-id")
}
