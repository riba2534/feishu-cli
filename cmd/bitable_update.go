package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// ==================== base update 更新多维表格元信息 ====================
// base/v3 无 PUT bases/{base_token}，更新（重命名/开高级权限）走 bitable/v1 PUT apps/{app_token}。
var bitableUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新多维表格元信息（重命名 / 高级权限开关）",
	Long: `PUT /open-apis/bitable/v1/apps/{app_token}

base/v3 无更新多维表格本体的端点，走 bitable/v1（app_token 即 base_token）。
仅支持云空间文件夹内的多维表格。

可选（仅显式设置的才提交）:
  --name          新名称
  --is-advanced   是否开启高级权限（true/false）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]any{}
		if cmd.Flags().Changed("name") {
			v, _ := cmd.Flags().GetString("name")
			body["name"] = v
		}
		if cmd.Flags().Changed("is-advanced") {
			v, _ := cmd.Flags().GetBool("is-advanced")
			body["is_advanced"] = v
		}
		if len(body) == 0 {
			return fmt.Errorf("未提供任何更新字段（用 --name 或 --is-advanced）")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "PUT", path: client.BitableV1Path("apps", bt), body: body, useV1: true}
		})
	},
}

func init() {
	bitableCmd.AddCommand(bitableUpdateCmd)
	addBitableWriteFlags(bitableUpdateCmd)
	bitableUpdateCmd.Flags().String("name", "", "新名称")
	bitableUpdateCmd.Flags().Bool("is-advanced", false, "是否开启高级权限")
}
