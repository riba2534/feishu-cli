package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/cobra"
)

var profileRenameJSON bool

var profileRenameCmd = &cobra.Command{
	Use:   "rename <old> <new>",
	Short: "重命名一个 profile",
	Long: `把 ~/.feishu-cli/profiles/<old>/ 重命名为 ~/.feishu-cli/profiles/<new>/。
若被重命名的 profile 是当前激活的，active-profile 指针会自动更新到新名。

示例:
  feishu-cli profile rename work tt-work
  feishu-cli profile rename personal lark`,
	Aliases: []string{"mv"},
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		oldName, newName := args[0], args[1]
		if err := profile.Rename(oldName, newName); err != nil {
			return err
		}
		dir, err := profile.ProfileDir(newName)
		if err != nil {
			return err
		}
		if profileRenameJSON {
			out := map[string]any{
				"ok":  true,
				"old": oldName,
				"new": newName,
				"dir": dir,
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "已重命名 profile %q → %q\n  新目录: %s\n", oldName, newName, dir)
		return nil
	},
}

func init() {
	profileRenameCmd.Flags().BoolVar(&profileRenameJSON, "json", false, "JSON 输出")
	profileCmd.AddCommand(profileRenameCmd)
}
