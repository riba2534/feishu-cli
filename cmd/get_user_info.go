package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getUserInfoCmd = &cobra.Command{
	Use:   "info <user_id>",
	Short: "获取用户信息",
	Long: `获取飞书用户的详细信息。

参数:
  <user_id>              用户 ID（必填）
  --user-id-type         用户 ID 类型 (open_id/union_id/user_id)，默认 open_id
  --department-id-type   部门 ID 类型 (department_id/open_department_id)
  --output, -o           输出格式 (json)

用户 ID 类型:
  open_id     Open ID（默认）
  union_id    Union ID
  user_id     用户 ID

示例:
  # 获取用户信息
  feishu-cli user info ou_xxx

  # 使用 user_id 类型
  feishu-cli user info xxx --user-id-type user_id

  # JSON 格式输出
  feishu-cli user info ou_xxx -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		userID := args[0]
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		departmentIDType, _ := cmd.Flags().GetString("department-id-type")
		output, _ := cmd.Flags().GetString("output")

		opts := client.GetUserInfoOptions{
			UserIDType:       userIDType,
			DepartmentIDType: departmentIDType,
		}

		info, err := client.GetUserInfo(userID, opts)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(info, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("用户信息:\n")
			if info.Name != "" {
				fmt.Printf("  姓名: %s\n", info.Name)
			}
			if info.EnName != "" {
				fmt.Printf("  英文名: %s\n", info.EnName)
			}
			if info.Nickname != "" {
				fmt.Printf("  昵称: %s\n", info.Nickname)
			}
			if info.OpenID != "" {
				fmt.Printf("  Open ID: %s\n", info.OpenID)
			}
			if info.UnionID != "" {
				fmt.Printf("  Union ID: %s\n", info.UnionID)
			}
			if info.UserID != "" {
				fmt.Printf("  User ID: %s\n", info.UserID)
			}
			if info.Email != "" {
				fmt.Printf("  邮箱: %s\n", info.Email)
			}
			if info.Mobile != "" {
				fmt.Printf("  手机: %s\n", info.Mobile)
			}
			if info.EmployeeNo != "" {
				fmt.Printf("  工号: %s\n", info.EmployeeNo)
			}
			if info.JobTitle != "" {
				fmt.Printf("  职位: %s\n", info.JobTitle)
			}
			if info.Status != "" {
				fmt.Printf("  状态: %s\n", info.Status)
			}
			if info.City != "" || info.Country != "" {
				location := ""
				if info.Country != "" {
					location = info.Country
				}
				if info.City != "" {
					if location != "" {
						location += " "
					}
					location += info.City
				}
				fmt.Printf("  位置: %s\n", location)
			}
			if info.Avatar != "" {
				fmt.Printf("  头像: %s\n", info.Avatar)
			}
		}

		return nil
	},
}

func init() {
	userCmd.AddCommand(getUserInfoCmd)
	getUserInfoCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型 (open_id/union_id/user_id)")
	getUserInfoCmd.Flags().String("department-id-type", "", "部门 ID 类型 (department_id/open_department_id)")
	getUserInfoCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
