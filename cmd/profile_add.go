package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/profile"
	"github.com/spf13/cobra"
)

var (
	profileAddAppID          string
	profileAddAppSecret      string
	profileAddAppSecretStdin bool
	profileAddProbe          bool
	profileAddBaseURL        string
	profileAddUse            bool
	profileAddJSON           bool
)

var profileAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "新建一个 profile",
	Long: `创建 ~/.feishu-cli/profiles/<name>/ 目录并写入初始 config.yaml。

不会自动迁移旧布局（~/.feishu-cli/config.yaml），如需迁移请用
'feishu-cli profile migrate'。

--app-secret-stdin 从 stdin 读取 App Secret（终端下不回显），避免 secret 进入 shell 历史与
ps 进程列表；--probe 在写入前用该凭证换取一次 tenant_access_token：服务端明确拒绝
（secret 错误、应用不存在）时报错退出（退出码 3）且不创建 profile，网络/临时错误只告警。

示例:
  printf '%s' "$SECRET" | feishu-cli profile add work --app-id cli_xxx --app-secret-stdin --probe --use
  feishu-cli profile add work --app-id cli_xxx --app-secret secret_xxx --use
  feishu-cli profile add personal --base-url https://open.larksuite.com
  feishu-cli profile add temp                                 # 留空待手动填`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := profile.ValidateName(name); err != nil {
			return clierr.Usage(err)
		}
		// 先确认名字可用，再读 stdin / 探测凭证，避免白做网络请求
		if exists, err := profile.Exists(name); err != nil {
			return err
		} else if exists {
			return fmt.Errorf("%w: %q", profile.ErrAlreadyExists, name)
		}

		secret, err := resolveSecretFlags(profileAddAppSecret, profileAddAppSecretStdin, cmd.InOrStdin())
		if err != nil {
			return err
		}

		// 凭证探测放在写盘之前：明确被拒绝的凭证不留下坏 profile
		probe := ""
		if profileAddProbe {
			if profileAddAppID == "" || secret == "" {
				return clierr.Usagef("--probe 需要同时提供 --app-id 与 App Secret（--app-secret 或 --app-secret-stdin）")
			}
			warning, err := probeAppCredentials(profileAddAppID, secret, profileAddBaseURL)
			if err != nil {
				return err
			}
			probe = "ok"
			if warning != "" {
				probe = "skipped"
				fmt.Fprintf(os.Stderr, "⚠ %s\n", warning)
			}
		}

		opts := profile.CreateOpts{
			AppID:     profileAddAppID,
			AppSecret: secret,
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
			if probe != "" {
				out["probe"] = probe
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "已创建 profile %q\n  目录: %s\n", name, dir)
		switch probe {
		case "ok":
			fmt.Fprintln(cmd.OutOrStdout(), "  凭证校验: 通过")
		case "skipped":
			fmt.Fprintln(cmd.OutOrStdout(), "  凭证校验: 未完成（见上方告警）")
		}
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
	profileAddCmd.Flags().StringVar(&profileAddAppSecret, "app-secret", "", "飞书应用 app_secret（会进入 shell 历史与 ps 输出，推荐改用 --app-secret-stdin）")
	profileAddCmd.Flags().BoolVar(&profileAddAppSecretStdin, "app-secret-stdin", false, "从 stdin 读取 app_secret（终端下不回显；与 --app-secret 互斥）")
	profileAddCmd.Flags().BoolVar(&profileAddProbe, "probe", false, "写入前换取一次 tenant_access_token 校验凭证（明确被拒绝时不创建 profile）")
	profileAddCmd.Flags().StringVar(&profileAddBaseURL, "base-url", "", "飞书 OpenAPI base URL（默认 https://open.feishu.cn）")
	profileAddCmd.Flags().BoolVar(&profileAddUse, "use", false, "创建后立即切换为当前 profile")
	profileAddCmd.Flags().BoolVar(&profileAddJSON, "json", false, "JSON 输出（适合脚本/AI Agent）")
	profileCmd.AddCommand(profileAddCmd)
}
