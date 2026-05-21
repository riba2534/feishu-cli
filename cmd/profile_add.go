package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/cobra"
)

var (
	profileAddAppID     string
	profileAddAppSecret string
	profileAddBaseURL   string
	profileAddUse       bool
	profileAddJSON      bool
)

var profileAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "新建一个 profile",
	Long: `创建 ~/.feishu-cli/profiles/<name>/ 目录并写入初始 config.yaml。

不会自动迁移旧布局（~/.feishu-cli/config.yaml），如需迁移请用
'feishu-cli profile migrate'。

示例:
  feishu-cli profile add work --app-id cli_xxx --app-secret xxx --use
  feishu-cli profile add personal --base-url https://open.larksuite.com
  feishu-cli profile add temp                                 # 留空待手动填`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		opts := profile.CreateOpts{
			AppID:     profileAddAppID,
			AppSecret: profileAddAppSecret,
			BaseURL:   profileAddBaseURL,
			SwitchTo:  profileAddUse,
		}
		if err := profile.Create(name, opts); err != nil {
			return err
		}

		dir, err := profile.ProfileDir(name)
		if err != nil {
			return err
		}

		if profileAddJSON {
			out := map[string]any{
				"ok":     true,
				"name":   name,
				"dir":    dir,
				"active": profileAddUse,
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "已创建 profile %q\n  目录: %s\n", name, dir)
		if profileAddUse {
			fmt.Fprintf(cmd.OutOrStdout(), "  已切换为当前 profile\n")
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "  下一步: feishu-cli profile use %s\n", name)
		}
		return nil
	},
}

func init() {
	profileAddCmd.Flags().StringVar(&profileAddAppID, "app-id", "", "飞书应用 app_id（可后续手动写 config.yaml）")
	profileAddCmd.Flags().StringVar(&profileAddAppSecret, "app-secret", "", "飞书应用 app_secret")
	profileAddCmd.Flags().StringVar(&profileAddBaseURL, "base-url", "", "飞书 OpenAPI base URL（默认 https://open.feishu.cn）")
	profileAddCmd.Flags().BoolVar(&profileAddUse, "use", false, "创建后立即切换为当前 profile")
	profileAddCmd.Flags().BoolVar(&profileAddJSON, "json", false, "JSON 输出（适合脚本/AI Agent）")
	profileCmd.AddCommand(profileAddCmd)
}
