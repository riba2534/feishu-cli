package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/v2/internal/profile"
	"github.com/spf13/cobra"
)

var (
	profileRemoveForce bool
	profileRemoveJSON  bool
)

var profileRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "删除一个 profile",
	Long: `删除 ~/.feishu-cli/profiles/<name>/ 整个目录（含 config / token / 用户信息缓存）。

默认会提示二次确认，加 --force 跳过提示。
若删除的是当前激活 profile，active-profile 指针会被清空，下次访问回退到字典序第一个 profile。

示例:
  feishu-cli profile remove temp
  feishu-cli profile remove old --force`,
	Aliases: []string{"rm", "delete"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		// 二次确认（除非 --force / --yes 或非交互；非交互直接执行是既有行为，保持兼容）。
		// 交互式拒绝返回"已取消"错误（非 0 退出），提示写 stderr 不污染 --json 输出。
		if !profileRemoveForce && isTerminal(os.Stdin) {
			if err := confirmDangerousAction(cmd, fmt.Sprintf("确认删除 profile %q？该操作不可恢复", name)); err != nil {
				return err
			}
		}

		if err := profile.Remove(name); err != nil {
			return err
		}

		if profileRemoveJSON {
			out := map[string]any{"ok": true, "removed": name}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "已删除 profile %q\n", name)
		return nil
	},
}

func init() {
	profileRemoveCmd.Flags().BoolVarP(&profileRemoveForce, "force", "f", false, "跳过二次确认")
	profileRemoveCmd.Flags().BoolVar(&profileRemoveJSON, "json", false, "JSON 输出")
	profileCmd.AddCommand(profileRemoveCmd)
}
