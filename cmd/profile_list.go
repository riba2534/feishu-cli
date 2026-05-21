package cmd

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/cobra"
)

var profileListJSON bool

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有 profile",
	Long: `列出 ~/.feishu-cli/profiles/ 下所有 profile，并标注当前激活的 profile。

示例:
  feishu-cli profile list
  feishu-cli profile list --json`,
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		infos, err := profile.Describe()
		if err != nil {
			return err
		}
		active, err := profile.ActiveName()
		if err != nil {
			return err
		}

		if profileListJSON {
			out := map[string]any{
				"active":   active,
				"profiles": infos,
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		}

		if len(infos) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "尚未创建任何 profile。")
			fmt.Fprintln(cmd.OutOrStdout(), "提示: feishu-cli profile add <name>            # 新建")
			fmt.Fprintln(cmd.OutOrStdout(), "      feishu-cli profile migrate                # 旧布局 → default profile")
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ACTIVE\tNAME\tCONFIG\tTOKEN\tPATH")
		for _, info := range infos {
			marker := " "
			if info.Active {
				marker = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				marker, info.Name, yesNo(info.HasConfig), yesNo(info.HasToken), info.Path)
		}
		if err := w.Flush(); err != nil {
			return err
		}
		if active == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "\n当前无激活 profile（active-profile 指针缺失），首次使用会回退到字典序第一个。")
		}
		return nil
	},
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func init() {
	profileListCmd.Flags().BoolVar(&profileListJSON, "json", false, "JSON 输出（适合脚本/AI Agent）")
	profileCmd.AddCommand(profileListCmd)
}
