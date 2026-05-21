package cmd

import (
	"encoding/json"
	"fmt"

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

var profileCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "显示当前激活的 profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := profile.ActiveName()
		if err != nil {
			return err
		}
		if name == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "(未启用 profile 系统，使用旧布局 ~/.feishu-cli/)")
			return nil
		}
		dir, err := profile.ProfileDir(name)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", name, dir)
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
	profileCmd.AddCommand(profileUseCmd)
	profileCmd.AddCommand(profileCurrentCmd)

	profileMigrateCmd.Flags().StringVar(&profileMigrateTarget, "name", "default", "迁移目标 profile 名")
	profileMigrateCmd.Flags().BoolVar(&profileMigrateForce, "force", false, "目标 profile 已存在时覆盖")
	profileMigrateCmd.Flags().BoolVar(&profileMigrateJSON, "json", false, "JSON 输出")
	profileCmd.AddCommand(profileMigrateCmd)
}
