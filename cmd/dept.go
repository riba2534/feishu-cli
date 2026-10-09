package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var deptCmd = &cobra.Command{
	Use:   "dept",
	Short: "部门操作命令",
	Long: `部门操作命令，用于查看部门信息和子部门列表。

子命令:
  get        获取部门详情
  children   获取子部门列表

部门 ID 类型 (--department-id-type):
  open_department_id   Open 部门 ID（默认）
  department_id        部门 ID

示例:
  feishu-cli dept get od_xxx
  feishu-cli dept children 0`,
}

var deptGetCmd = &cobra.Command{
	Use:   "get <department_id>",
	Short: "获取部门详情",
	Long: `获取指定部门的详细信息。

参数:
  department_id             部门 ID（位置参数）
  --department-id-type      部门 ID 类型（默认 open_department_id）

示例:
  feishu-cli dept get od_xxx
  feishu-cli dept get DEPT_ID --department-id-type department_id`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		departmentID := args[0]
		departmentIDType, _ := cmd.Flags().GetString("department-id-type")
		output, _ := cmd.Flags().GetString("output")

		dept, err := client.GetDepartment(departmentID, "open_id", departmentIDType)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(dept)
		}

		fmt.Printf("部门名称:      %s\n", dept.Name)
		if dept.OpenDepartmentID != "" {
			fmt.Printf("Open 部门 ID:  %s\n", dept.OpenDepartmentID)
		}
		if dept.DepartmentID != "" {
			fmt.Printf("部门 ID:       %s\n", dept.DepartmentID)
		}
		if dept.ParentDepartmentID != "" {
			fmt.Printf("父部门 ID:     %s\n", dept.ParentDepartmentID)
		}
		if dept.LeaderUserID != "" {
			fmt.Printf("主管 ID:       %s\n", dept.LeaderUserID)
		}
		if dept.ChatID != "" {
			fmt.Printf("部门群 ID:     %s\n", dept.ChatID)
		}
		if dept.MemberCount > 0 {
			fmt.Printf("成员数:        %d\n", dept.MemberCount)
		}

		return nil
	},
}

var deptChildrenCmd = &cobra.Command{
	Use:   "children <department_id>",
	Short: "获取子部门列表",
	Long: `获取指定部门的下级子部门列表。使用 "0" 作为部门 ID 获取根部门下的子部门。

参数:
  department_id             部门 ID（位置参数），根部门使用 "0"
  --department-id-type      部门 ID 类型（默认 open_department_id）

示例:
  feishu-cli dept children 0
  feishu-cli dept children od_xxx
  feishu-cli dept children od_xxx -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		parentDeptID := args[0]
		departmentIDType, _ := cmd.Flags().GetString("department-id-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		depts, nextPageToken, hasMore, err := client.ListDepartments(parentDeptID, "open_id", departmentIDType, pageSize, pageToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]interface{}{
				"departments":     depts,
				"next_page_token": nextPageToken,
				"has_more":        hasMore,
			})
		}

		if len(depts) == 0 {
			fmt.Println("暂无子部门")
			return nil
		}

		fmt.Printf("子部门列表（共 %d 个）:\n\n", len(depts))
		for i, dept := range depts {
			fmt.Printf("[%d] %s\n", i+1, dept.Name)
			if dept.OpenDepartmentID != "" {
				fmt.Printf("    Open 部门 ID: %s\n", dept.OpenDepartmentID)
			}
			if dept.MemberCount > 0 {
				fmt.Printf("    成员数: %d\n", dept.MemberCount)
			}
			if dept.LeaderUserID != "" {
				fmt.Printf("    主管 ID: %s\n", dept.LeaderUserID)
			}
			fmt.Println()
		}

		if hasMore {
			fmt.Printf("下一页 token: %s\n", nextPageToken)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(deptCmd)

	deptCmd.AddCommand(deptGetCmd)
	deptGetCmd.Flags().String("department-id-type", "open_department_id", "部门 ID 类型")
	deptGetCmd.Flags().StringP("output", "o", "", "输出格式（json）")

	deptCmd.AddCommand(deptChildrenCmd)
	deptChildrenCmd.Flags().String("department-id-type", "open_department_id", "部门 ID 类型")
	deptChildrenCmd.Flags().Int("page-size", 0, "每页数量")
	deptChildrenCmd.Flags().String("page-token", "", "分页标记")
	deptChildrenCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
