package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// deleteCommentUnsupportedMsg 说明飞书没有「删除整条评论」的 Open API，并给出可行替代方案。
// 飞书的「评论」是一条回复线程：只能删除其中的单条回复（且仅回复作者本人可删），
// 没有删除整条评论的端点（SDK fileComment 仅有 BatchQuery/Create/Get/List/Patch，无 Delete）。
const deleteCommentUnsupportedMsg = `飞书 Open API 不支持删除整条评论线程，本命令不会执行任何删除操作。

可用替代方案:
  • 删除自己发布的回复（评论线程仅含你这一条回复时，删除它即等于删掉该评论）:
      feishu-cli comment reply list <file_token> <comment_id> --type <type>
      feishu-cli comment reply delete <file_token> <comment_id> <reply_id> --type <type>
  • 将评论标记为已解决（保留记录、折叠显示）:
      feishu-cli comment resolve <file_token> <comment_id> --type <type>

注意: 飞书只允许回复作者身份删除自己的回复。用户发布的回复需显式传入该作者的
--user-access-token；Bot 发布的回复使用创建它的同一 App 身份（默认，省略 User Token）。`

var deleteCommentCmd = &cobra.Command{
	Use:   "delete <file_token> <comment_id>",
	Short: "删除评论（飞书不支持整条删除，改用 reply delete / resolve）",
	Long: `删除文档评论。

⚠ 飞书 Open API 没有「删除整条评论」的端点，因此本命令不执行删除，仅给出替代方案指引。
飞书的「评论」本质是一条回复线程，只能删除其中的单条回复（且仅回复作者本人可删），
或将整条评论标记为已解决。

替代命令:
  feishu-cli comment reply delete <file_token> <comment_id> <reply_id> --type docx
  feishu-cli comment resolve <file_token> <comment_id> --type docx`,
	// 不限制参数个数：无论是否带 <file_token> <comment_id>，都给出统一指引而非参数报错。
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%s", deleteCommentUnsupportedMsg)
	},
}

func init() {
	commentCmd.AddCommand(deleteCommentCmd)
	// --type 保留为可选项，仅为兼容历史调用写法（如 `comment delete X Y --type docx`），
	// 避免老脚本因 "unknown flag: --type" 报错，看不到上面的替代方案指引。
	deleteCommentCmd.Flags().String("type", "", "文件类型（doc/docx/sheet/bitable）；本命令仅作兼容占位，不生效")
}
