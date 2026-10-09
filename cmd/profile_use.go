package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/cobra"
)

var profileUseJSON bool

var profileUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "切换当前 profile",
	Long: `把 ~/.feishu-cli/active-profile 指针写为 <name>，后续所有 feishu-cli
命令默认从该 profile 读 config + token。

特殊参数 '-' 表示切回上一个 profile。

示例:
  feishu-cli profile use work
  feishu-cli profile use -                # 切回上一个 profile`,
	Aliases: []string{"switch", "checkout"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		previous, _ := profile.ReadActive()

		newActive, err := profile.Use(name)
		if err != nil {
			return err
		}

		dir, err := profile.ProfileDir(newActive)
		if err != nil {
			return err
		}

		// 显式针对切换后的 profile 判定，不受本次 --profile 影响；只有真错配才出声
		if msg := appCredentialWarningForProfile(newActive, dir); msg != "" {
			fmt.Fprintln(os.Stderr, msg)
			fmt.Fprintf(os.Stderr, "    profile use 只切换目录和 User Token；要改用 profile %q 自带的 App 凭证，请先 unset FEISHU_APP_ID/FEISHU_APP_SECRET 或去掉 --bot-app-* flag。\n", newActive)
		}

		if profileUseJSON {
			out := map[string]any{
				"ok":       true,
				"active":   newActive,
				"previous": previous,
				"dir":      dir,
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		}

		if previous == newActive {
			fmt.Fprintf(cmd.OutOrStdout(), "当前已是 profile %q，无需切换\n", newActive)
			return nil
		}
		if previous == "" {
			fmt.Fprintf(cmd.OutOrStdout(), "已切换到 profile %q\n  目录: %s\n", newActive, dir)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "已切换 profile %q → %q\n  目录: %s\n", previous, newActive, dir)
		}
		return nil
	},
}

var profileCurrentJSON bool

var profileCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "显示当前真正生效的 Bot / profile",
	Long: `显示叠加 --profile / FEISHU_PROFILE / App 凭证覆盖后，当前命令会使用的完整身份上下文。

JSON 输出与 profile list --json 同形的完整 inventory，包含 effective、active_source、
token_from_profile、env_overrides、flag_overrides 和 hint，便于 Agent 确认身份。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		inv, err := buildProfileInventory()
		if err != nil {
			return err
		}
		if profileCurrentJSON {
			return printJSON(inv)
		}
		eff := inv.Effective
		name := eff.Name
		if name == "" {
			name = "(legacy)"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", name, eff.Path)
		if eff.AppID != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "app_id:\t%s\n", eff.AppID)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "source:\t%s\n", inv.ActiveSource)
		if inv.EffectiveError != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "⚠️ 问题:\t%s\n", inv.EffectiveError)
		}
		if inv.Hint != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "提示:\t%s\n", inv.Hint)
		}
		return nil
	},
}

var (
	profileMigrateTarget string
	profileMigrateForce  bool
	profileMigrateJSON   bool
)

var profileMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "把旧布局 ~/.feishu-cli/{config.yaml,token.json} 迁移到 profiles/<name>/",
	Long: `把旧布局的配置和 token 迁移到一个新的 profile 目录，并把指针指向它。
原文件不会被删除——用户自己确认无误后可手动 rm。

示例:
  feishu-cli profile migrate                          # → profiles/default/
  feishu-cli profile migrate --name work              # → profiles/work/
  feishu-cli profile migrate --force                  # 覆盖已存在的同名 profile`,
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := profile.MigrateLegacy(profile.MigrateLegacyOpts{
			TargetName: profileMigrateTarget,
			Force:      profileMigrateForce,
		})
		if err != nil {
			return err
		}
		dir, err := profile.ProfileDir(target)
		if err != nil {
			return err
		}
		if profileMigrateJSON {
			out := map[string]any{
				"ok":     true,
				"target": target,
				"dir":    dir,
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "已迁移旧布局到 profile %q\n  目录: %s\n", target, dir)
		fmt.Fprintln(cmd.OutOrStdout(), "  原文件未删除，确认无误后可手动清理：")
		fmt.Fprintln(cmd.OutOrStdout(), "    rm ~/.feishu-cli/config.yaml ~/.feishu-cli/token.json ~/.feishu-cli/user_profile.json")
		return nil
	},
}

func init() {
	profileUseCmd.Flags().BoolVar(&profileUseJSON, "json", false, "JSON 输出")
	profileCurrentCmd.Flags().BoolVar(&profileCurrentJSON, "json", false, "JSON 输出（完整 inventory，适合 Agent）")
	profileCmd.AddCommand(profileUseCmd)
	profileCmd.AddCommand(profileCurrentCmd)

	profileMigrateCmd.Flags().StringVar(&profileMigrateTarget, "name", "default", "迁移目标 profile 名")
	profileMigrateCmd.Flags().BoolVar(&profileMigrateForce, "force", false, "目标 profile 已存在时覆盖")
	profileMigrateCmd.Flags().BoolVar(&profileMigrateJSON, "json", false, "JSON 输出")
	profileCmd.AddCommand(profileMigrateCmd)
}
