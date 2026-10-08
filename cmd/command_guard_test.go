package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newGuardTestTree() *cobra.Command {
	root := &cobra.Command{Use: "root"}
	group := &cobra.Command{Use: "doc"}
	group.AddCommand(&cobra.Command{Use: "import", Run: func(*cobra.Command, []string) {}})
	group.AddCommand(&cobra.Command{Use: "export", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group)
	installUnknownSubcommandGuard(root)
	return root
}

func TestUnknownSubcommandGuardErrors(t *testing.T) {
	root := newGuardTestTree()
	root.SetArgs([]string{"doc", "importt"})
	root.SilenceUsage = true
	root.SilenceErrors = true
	err := root.Execute()
	if err == nil {
		t.Fatal("未知子命令应返回错误，实际返回 nil（静默成功）")
	}
	if !strings.Contains(err.Error(), "importt") {
		t.Errorf("错误信息应包含未知子命令名，实际: %v", err)
	}
	if !strings.Contains(err.Error(), "import") || !strings.Contains(err.Error(), "你是不是想用") {
		t.Errorf("错误信息应包含拼写建议 import，实际: %v", err)
	}
}

func TestBareGroupShowsHelpWithoutError(t *testing.T) {
	root := newGuardTestTree()
	root.SetArgs([]string{"doc"})
	if err := root.Execute(); err != nil {
		t.Fatalf("裸命令组应打印帮助且不报错，实际: %v", err)
	}
}

func TestGuardSkipsRunnableCommands(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	hybrid := &cobra.Command{Use: "hybrid", RunE: func(*cobra.Command, []string) error { return nil }}
	hybrid.AddCommand(&cobra.Command{Use: "sub", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(hybrid)
	installUnknownSubcommandGuard(root)
	if hybrid.Annotations[groupGuardAnnotation] == "1" {
		t.Error("自身可运行的命令不应被注入守卫")
	}
}

func TestParseUnknownFlagName(t *testing.T) {
	cases := map[string]string{
		"unknown flag: --filterr":                 "filterr",
		"unknown shorthand flag: 'x' in -xyz":     "x",
		"flag needs an argument: --output":        "",
		"invalid argument \"abc\" for \"--page\"": "",
	}
	for msg, want := range cases {
		if got := parseUnknownFlagName(msg); got != want {
			t.Errorf("parseUnknownFlagName(%q) = %q, want %q", msg, got, want)
		}
	}
}

func TestFlagSuggestion(t *testing.T) {
	cmd := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	cmd.Flags().String("filter-json", "", "")
	cmd.Flags().String("format", "", "")
	got := closestFlagNames(cmd, "fliter-json", 3)
	if len(got) == 0 || got[0] != "filter-json" {
		t.Errorf("fliter-json 应建议 filter-json，实际: %v", got)
	}
	// 前缀命中：--filter 提示 --filter-json
	got = closestFlagNames(cmd, "filter", 3)
	if len(got) == 0 || got[0] != "filter-json" {
		t.Errorf("filter 前缀应建议 filter-json，实际: %v", got)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"导出", "导入", 1},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// newMsgGuardTestTree 模拟 `msg` 纯分组命令：子命令带自己的 flag。
func newMsgGuardTestTree() *cobra.Command {
	root := &cobra.Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
	group := &cobra.Command{Use: "msg"}
	send := &cobra.Command{Use: "send", RunE: func(*cobra.Command, []string) error { return nil }}
	send.Flags().String("text", "", "")
	group.AddCommand(send)
	group.AddCommand(&cobra.Command{Use: "history", RunE: func(*cobra.Command, []string) error { return nil }})
	hybrid := &cobra.Command{Use: "hybrid", Args: cobra.ArbitraryArgs, RunE: func(*cobra.Command, []string) error { return nil }}
	hybrid.AddCommand(&cobra.Command{Use: "sub", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group, hybrid)
	installUnknownSubcommandGuard(root)
	root.SetFlagErrorFunc(flagSuggestionErrorFunc)
	return root
}

// TestMisspelledSubcommandWithFlags 回归：`msg send-messag --text hi` 过去只报 unknown flag: --text，
// 看不出是子命令拼错；现在应提示子命令拼写并按用法错误（退出码 2）退出。
func TestMisspelledSubcommandWithFlags(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantSub []string
	}{
		{"拼错子命令 + 子命令 flag", []string{"msg", "send-messag", "--text", "hi"}, []string{`未知子命令 "send-messag"`, "你是不是想用", "send"}},
		{"拼错子命令 + 等号形式 flag", []string{"msg", "histroy", "--text=hi"}, []string{`未知子命令 "histroy"`, "history"}},
		{"分组命令上只有未知 flag 仍报 unknown flag", []string{"msg", "--text", "hi"}, []string{"unknown flag: --text"}},
		{"可运行命令的位置参数不被当作子命令", []string{"hybrid", "arg1", "--nope"}, []string{"unknown flag: --nope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newMsgGuardTestTree()
			root.SetArgs(tc.args)
			err := root.Execute()
			if err == nil {
				t.Fatal("期望报错，实际成功")
			}
			for _, sub := range tc.wantSub {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("错误信息应包含 %q，实际: %v", sub, err)
				}
			}
			if strings.Contains(tc.name, "可运行") && strings.Contains(err.Error(), "未知子命令") {
				t.Errorf("可运行命令不应被误判为子命令拼错: %v", err)
			}
			if got := exitCodeFor(err); got != 2 {
				t.Errorf("退出码 = %d, want 2", got)
			}
		})
	}
}

func TestSuggestSubcommandsPrefixBoundary(t *testing.T) {
	root := newMsgGuardTestTree()
	group, _, err := root.Find([]string{"msg"})
	if err != nil {
		t.Fatalf("找不到 msg: %v", err)
	}
	got := suggestSubcommands(group, "send-message")
	if len(got) == 0 || got[0] != "send" {
		t.Fatalf("send-message 应建议 send，实际: %v", got)
	}
	if got := suggestSubcommands(group, "zzzz"); len(got) != 0 {
		t.Fatalf("无相近子命令时不应给出建议: %v", got)
	}
}
