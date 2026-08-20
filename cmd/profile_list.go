package cmd

import (
	"fmt"
	"io"

	"github.com/riba2534/feishu-cli/internal/profile"

	"github.com/spf13/cobra"
)

var profileListJSON bool

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出可操作的 Bot / profile",
	Long: `列出本机所有飞书 Bot（profile），以及当前真正会生效的那一套。

即使还没执行过 profile add，也会输出环境变量 / 旧布局里正在用的那一套，
避免 Agent 误以为「一个 Bot 都没有」。

JSON（--json）是 Agent 契约：看 profiles[] 识别磁盘上的 Bot，
看 effective 确认叠加 FEISHU_APP_ID 后实际会打到哪个 App。

示例:
  feishu-cli profile list
  feishu-cli profile list --json
  feishu-cli --profile alert profile list --json`,
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		inv, err := buildProfileInventory()
		if err != nil {
			return err
		}
		if profileListJSON {
			return printJSON(inv)
		}
		return printProfileInventory(cmd, inv)
	},
}

func printProfileInventory(cmd *cobra.Command, inv profileInventory) error {
	out := cmd.OutOrStdout()
	if inv.Active != "" {
		fmt.Fprintf(out, "当前: %s（source=%s, mode=%s）\n", inv.Active, inv.ActiveSource, inv.Mode)
	} else {
		fmt.Fprintf(out, "当前: 旧布局/环境变量（source=%s, mode=%s）\n", inv.ActiveSource, inv.Mode)
	}

	// active 为空表示生效的是旧布局，它不在 profiles[] 里。哪怕磁盘上另有 profile，
	// 也必须把这行摆进表格——否则「列出真正会生效的那一套」就成了空话。
	rows := inv.Profiles
	if inv.Active == "" {
		row := inv.Effective
		row.Name = displayProfileName(row.Name)
		row.Active = true
		rows = append([]profileSnapshot{row}, rows...)
	}

	table := make([][]string, 0, len(rows))
	for _, info := range rows {
		marker := " "
		if info.Active {
			marker = "*"
		}
		appID := info.AppID
		if appID == "" {
			appID = "-"
		}
		token := info.TokenStatus
		if token == "" {
			token = "-"
		}
		user := info.UserName
		if user == "" {
			user = "-"
		}
		sel := info.SelectWith
		if sel == "" {
			sel = "-"
		}
		table = append(table, []string{marker, info.Name, appID, token, user, sel})
	}
	// 用户名可能是中文，按显示宽度对齐（tabwriter 按字节算会顶歪 SELECT 列）
	if err := renderColumns(out, []string{"ACTIVE", "NAME", "APP_ID", "TOKEN", "USER", "SELECT"}, table); err != nil {
		return err
	}

	// 表格里的 * 行是磁盘上的 YAML；被覆盖时必须点明本次真正生效的 app_id，
	// 否则用户会把 * 行当成本次实际用的凭证。
	switch {
	case inv.FlagOverrides.AppID && inv.Effective.AppID != "":
		fmt.Fprintf(out, "\n生效 app_id: %s（被 --bot-app-id 覆盖）\n", inv.Effective.AppID)
	case inv.EnvOverrides.AppID && inv.Effective.AppID != "":
		fmt.Fprintf(out, "\n生效 app_id: %s（被 FEISHU_APP_ID 覆盖）\n", inv.Effective.AppID)
	case cfgFile != "":
		if inv.Effective.AppID != "" {
			fmt.Fprintf(out, "\n生效 app_id: %s（来自 --config %s）\n", inv.Effective.AppID, cfgFile)
		} else {
			fmt.Fprintf(out, "\n生效配置: --config %s\n", cfgFile)
		}
	}
	if inv.EffectiveError != "" {
		fmt.Fprintf(out, "⚠️  当前生效的这套有问题: %s\n", inv.EffectiveError)
	}
	// 容错不等于静默：坏掉的条目必须点名，否则用户只会看到一行 "-"
	var broken []profileSnapshot
	for _, info := range rows {
		if info.problems() != "" {
			broken = append(broken, info)
		}
	}
	if len(broken) > 0 {
		fmt.Fprintln(out)
		for _, info := range broken {
			fmt.Fprintf(out, "⚠️  %s: %s\n", displayProfileName(info.Name), info.problems())
		}
	}
	if inv.Hint != "" {
		fmt.Fprintf(out, "\n提示: %s\n", inv.Hint)
	}
	printPointerNote(out, inv)
	return nil
}

// printPointerNote 解释指针状态。指针失效与指针缺失是两回事，
// 且回退结果此时已经确定，不该再说"或字典序第一个"。
func printPointerNote(out io.Writer, inv profileInventory) {
	if inv.Mode != "profile" {
		return
	}
	ptr, err := profile.ReadActive()
	if err != nil {
		return
	}
	switch inv.ActiveSource {
	case profile.SourceLegacy:
		if ptr != "" {
			fmt.Fprintf(out, "\nactive-profile 指针指向 %q，该 profile 已不存在，已回退到旧布局 ~/.feishu-cli。\n", ptr)
			return
		}
		fmt.Fprintln(out, "\n没有 active-profile 指针，旧布局仍在，已回退到旧布局 ~/.feishu-cli。")
	case profile.SourceFallback:
		if ptr != "" {
			fmt.Fprintf(out, "\nactive-profile 指针指向 %q，该 profile 已不存在，已回退到字典序第一个（%s）。\n", ptr, inv.Active)
			return
		}
		fmt.Fprintf(out, "\n没有 active-profile 指针，已回退到字典序第一个（%s）。\n", inv.Active)
	}
}

func init() {
	profileListCmd.Flags().BoolVar(&profileListJSON, "json", false, "JSON 输出（适合脚本/AI Agent）")
	profileCmd.AddCommand(profileListCmd)
}
