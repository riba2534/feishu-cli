package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var bitableCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建多维表格",
	Long:  "创建一个新的多维表格",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		folder, _ := cmd.Flags().GetString("folder")
		output, _ := cmd.Flags().GetString("output")
		userToken := resolveOptionalUserToken(cmd)

		app, err := client.CreateBitableApp(name, folder, userToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(app)
		}

		fmt.Printf("创建成功！\n")
		fmt.Printf("  App Token: %s\n", app.AppToken)
		fmt.Printf("  名称: %s\n", app.Name)
		if app.URL != "" {
			fmt.Printf("  URL: %s\n", app.URL)
		}

		// Auto-grant owner permission
		if err := grantOwnerPermission(app.AppToken, "bitable"); err != nil {
			fmt.Fprintf(os.Stderr, "警告: %v\n", err)
		}

		return nil
	},
}

func init() {
	bitableCmd.AddCommand(bitableCreateCmd)

	bitableCreateCmd.Flags().StringP("name", "n", "新建多维表格", "多维表格名称")
	bitableCreateCmd.Flags().StringP("folder", "f", "", "目标文件夹 Token（可选）")
	bitableCreateCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	bitableCreateCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
}
