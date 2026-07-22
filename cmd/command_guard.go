package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// groupGuardAnnotation 标记"命令组守卫"注入的 RunE，供 PersistentPreRunE 识别：
// 带此标记的命令组仍视为纯分组命令，跳过 config 初始化。
const groupGuardAnnotation = "feishu-cli/group-guard"

// installUnknownSubcommandGuard 递归给所有"有子命令但自身不可运行"的命令组注入 RunE：
//   - 不带参数（如 `feishu-cli doc`）→ 打印帮助，退出码 0（保持原行为）
//   - 带未知子命令（如 `feishu-cli doc importt`）→ 返回错误 + 拼写建议，退出码非 0
//
// 背景：cobra 默认只在根命令层对未知子命令报错；嵌套命令组会静默打印帮助并以 0 退出，
// AI Agent / 脚本会把这种"静默成功"误判为命令执行成功，属于隐性正确性缺陷。
func installUnknownSubcommandGuard(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		installUnknownSubcommandGuard(child)
	}
	// 根命令保留 cobra 内建的 unknown command 处理（本身就报错 + 建议）
	if cmd.Parent() == nil || !cmd.HasSubCommands() || cmd.Run != nil || cmd.RunE != nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[groupGuardAnnotation] = "1"
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			return c.Help()
		}
		return unknownSubcommandError(c, args[0])
	}
}

// unknownSubcommandError 构造带拼写建议的未知子命令错误。
func unknownSubcommandError(cmd *cobra.Command, name string) error {
	// cobra 仅在根命令上默认启用建议距离（ExecuteC 里设 2）；命令组上需显式设置
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2
	}
	msg := fmt.Sprintf("未知子命令 %q（命令组 %q）", name, cmd.CommandPath())
	if suggestions := cmd.SuggestionsFor(name); len(suggestions) > 0 {
		msg += fmt.Sprintf("\n\n你是不是想用:\n\t%s", strings.Join(suggestions, "\n\t"))
	}
	msg += fmt.Sprintf("\n\n运行 `%s --help` 查看全部可用子命令", cmd.CommandPath())
	return fmt.Errorf("%s", msg)
}

// flagSuggestionErrorFunc 是 cobra FlagErrorFunc：flag 解析失败（未知 flag / 拼写错误）时
// 在原错误上追加"最相近 flag"建议，帮助用户 / Agent 自行纠错。
func flagSuggestionErrorFunc(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	name := parseUnknownFlagName(err.Error())
	if name == "" {
		return err
	}
	suggestions := closestFlagNames(cmd, name, 3)
	if len(suggestions) == 0 {
		return err
	}
	for i := range suggestions {
		suggestions[i] = "--" + suggestions[i]
	}
	return fmt.Errorf("%w\n\n你是不是想用: %s", err, strings.Join(suggestions, ", "))
}

// parseUnknownFlagName 从 pflag 错误信息里提取未知 flag 名。
// 已知格式："unknown flag: --xyz"、"unknown shorthand flag: 'x' in -xyz"。
func parseUnknownFlagName(msg string) string {
	if idx := strings.Index(msg, "unknown flag: --"); idx >= 0 {
		return strings.TrimSpace(msg[idx+len("unknown flag: --"):])
	}
	if idx := strings.Index(msg, "unknown shorthand flag: '"); idx >= 0 {
		rest := msg[idx+len("unknown shorthand flag: '"):]
		if len(rest) > 0 {
			return string(rest[0])
		}
	}
	return ""
}

// closestFlagNames 在命令可用 flag（本地 + 继承）中找到与 name 编辑距离 ≤ 2 的候选，
// 按距离升序返回最多 limit 个。
func closestFlagNames(cmd *cobra.Command, name string, limit int) []string {
	type cand struct {
		name string
		dist int
	}
	var cands []cand
	seen := map[string]bool{}
	collect := func(f *pflag.Flag) {
		if seen[f.Name] || f.Hidden {
			return
		}
		seen[f.Name] = true
		d := levenshtein(strings.ToLower(name), strings.ToLower(f.Name))
		// 阈值：短名字容忍 2，且前缀命中也算候选（如 --filter 提示 --filter-json）
		if d <= 2 || strings.HasPrefix(strings.ToLower(f.Name), strings.ToLower(name)) {
			cands = append(cands, cand{f.Name, d})
		}
	}
	cmd.Flags().VisitAll(collect)
	cmd.InheritedFlags().VisitAll(collect)

	// 按距离升序、同距离按名字排序
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		return cands[i].name < cands[j].name
	})
	var out []string
	for _, c := range cands {
		if len(out) >= limit {
			break
		}
		out = append(out, c.name)
	}
	return out
}

// levenshtein 计算两个字符串的编辑距离（按 rune）。
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := 0; j <= len(rb); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}
