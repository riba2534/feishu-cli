package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var secureLabelCmd = &cobra.Command{
	Use:   "secure-label",
	Short: "云文档密级标签（查看可用标签 / 设置文档密级）",
	Long: `管理云文档的密级（安全）标签。

子命令:
  list    查询当前用户可用的密级标签（先拿到目标标签 id）
  set     把指定云文档设置为某个密级标签

两个子命令都必须使用用户身份（User Access Token）。设置密级前通常先执行 list 确认可用标签 ID，
不要用标签显示名（如 "内部(D)"）当作 --label-id。

权限:
  - list：docs:secure_label:readonly
  - set：docs:secure_label:write_only`,
}

var secureLabelListCmd = &cobra.Command{
	Use:   "list",
	Short: "查询当前用户可用的密级标签",
	Long: `查询当前用户可用的密级标签，返回每个标签的 id 与名称。
把返回的 id 作为 secure-label set 的 --label-id，不要使用显示名。

可选:
  --page-size    分页大小，范围 1-10（默认 10）
  --page-token   上一页响应里的 page_token
  --lang         标签语言：zh / en / ja
  --output / -o  输出格式（json）

底层接口：GET /open-apis/drive/v2/my_secure_labels

示例:
  feishu-cli drive secure-label list --page-size 10 --lang zh
  feishu-cli drive secure-label list --output json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		lang, _ := cmd.Flags().GetString("lang")
		output, _ := cmd.Flags().GetString("output")

		if pageSize < 1 || pageSize > 10 {
			return fmt.Errorf("--page-size 取值范围为 1-10")
		}

		token, err := requireUserToken(cmd, "drive secure-label list")
		if err != nil {
			return err
		}

		labels, nextPageToken, hasMore, err := client.ListSecureLabels(pageSize, pageToken, lang, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]any{
				"labels":     labels,
				"page_token": nextPageToken,
				"has_more":   hasMore,
			})
		}

		if len(labels) == 0 {
			fmt.Println("当前用户暂无可用密级标签")
			return nil
		}

		fmt.Printf("共 %d 个密级标签:\n\n", len(labels))
		for _, l := range labels {
			fmt.Printf("  %s  %s\n", l.ID, l.Name)
		}
		if hasMore {
			fmt.Printf("\n还有更多，下一页 page_token: %s\n", nextPageToken)
		}
		return nil
	},
}

var secureLabelSetCmd = &cobra.Command{
	Use:   "set <file_token>",
	Short: "设置云文档的密级标签",
	Long: `把指定云文档设置为某个密级标签。

参数:
  file_token    目标文档 token（位置参数）

可选:
  --type         文档类型：doc/docx/sheet/file/bitable/mindnote/slides（默认 docx）
  --output / -o  输出格式（json）

必填:
  --label-id     要设置的密级标签 ID（从 secure-label list 获取，不要用显示名）

底层接口：PATCH /open-apis/drive/v2/files/{file_token}/secure_label?type={type}

注意:
  - 密级降级可能需要在文档界面完成审批，重试 API 不会绕过审批（错误码 1063013）

示例:
  feishu-cli drive secure-label set doxcnXXXX --type docx --label-id 7217780879644737539`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		docType, _ := cmd.Flags().GetString("type")
		labelID, _ := cmd.Flags().GetString("label-id")
		output, _ := cmd.Flags().GetString("output")

		if labelID == "" {
			return fmt.Errorf("--label-id 必填（从 secure-label list 获取标签 ID，不要使用显示名）")
		}

		token, err := requireUserToken(cmd, "drive secure-label set")
		if err != nil {
			return err
		}

		if err := client.SetSecureLabel(fileToken, docType, labelID, token); err != nil {
			return err
		}

		result := map[string]any{
			"file_token": fileToken,
			"type":       docType,
			"label_id":   labelID,
		}
		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("密级标签设置成功！\n")
		fmt.Printf("  文件 Token: %s\n", fileToken)
		fmt.Printf("  类型:       %s\n", docType)
		fmt.Printf("  标签 ID:    %s\n", labelID)
		return nil
	},
}

func init() {
	driveCmd.AddCommand(secureLabelCmd)
	secureLabelCmd.PersistentFlags().String("user-access-token", "", "User Access Token（必需，密级标签接口仅支持用户身份）")

	secureLabelCmd.AddCommand(secureLabelListCmd)
	secureLabelListCmd.Flags().Int("page-size", 10, "分页大小（1-10）")
	secureLabelListCmd.Flags().String("page-token", "", "上一页响应里的 page_token")
	secureLabelListCmd.Flags().String("lang", "", "标签语言：zh / en / ja")
	secureLabelListCmd.Flags().StringP("output", "o", "", "输出格式（json）")

	secureLabelCmd.AddCommand(secureLabelSetCmd)
	secureLabelSetCmd.Flags().String("type", "docx", "文档类型：doc/docx/sheet/file/bitable/mindnote/slides")
	secureLabelSetCmd.Flags().String("label-id", "", "要设置的密级标签 ID（必填）")
	secureLabelSetCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(secureLabelSetCmd, "label-id")
}
