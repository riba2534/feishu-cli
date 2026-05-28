package cmd

import (
	"strconv"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// TestVCBotCmdRegistered 验证 bot 父组与三个子命令注册
func TestVCBotCmdRegistered(t *testing.T) {
	if vcBotCmd.Use != "bot" {
		t.Fatalf("Use = %q, want bot", vcBotCmd.Use)
	}
	found := false
	for _, sub := range vcCmd.Commands() {
		if sub == vcBotCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("vcBotCmd should be child of vcCmd")
	}

	got := map[string]bool{}
	for _, sub := range vcBotCmd.Commands() {
		got[sub.Use] = true
	}
	for _, use := range []string{"meeting-join", "meeting-leave", "meeting-events"} {
		if !got[use] {
			t.Errorf("missing bot subcommand %q", use)
		}
	}
}

func botSub(use string) *cobra.Command {
	for _, sub := range vcBotCmd.Commands() {
		if sub.Use == use {
			return sub
		}
	}
	return nil
}

// TestVCBotFlagsRequired 验证三个子命令的 flag 注册与必填项
func TestVCBotFlagsRequired(t *testing.T) {
	cases := []struct {
		name     string
		flags    []string
		required string
	}{
		{"meeting-join", []string{"meeting-number", "password", "dry-run", "output"}, "meeting-number"},
		{"meeting-leave", []string{"meeting-id", "dry-run", "output"}, "meeting-id"},
		{"meeting-events", []string{"meeting-id", "start", "end", "page-size", "page-token", "dry-run", "output"}, "meeting-id"},
	}
	for _, tc := range cases {
		c := botSub(tc.name)
		if c == nil {
			t.Fatalf("subcommand %q not found", tc.name)
		}
		for _, f := range tc.flags {
			if c.Flags().Lookup(f) == nil {
				t.Errorf("%s: --%s missing", tc.name, f)
			}
		}
		req := c.Flags().Lookup(tc.required)
		if req == nil {
			t.Fatalf("%s: required flag --%s missing", tc.name, tc.required)
		}
		ann := req.Annotations["cobra_annotation_bash_completion_one_required_flag"]
		if len(ann) == 0 || ann[0] != "true" {
			t.Errorf("%s: --%s should be required, ann=%v", tc.name, tc.required, ann)
		}
		if out := c.Flags().Lookup("output"); out != nil && out.Shorthand != "o" {
			t.Errorf("%s: --output shorthand=%q, want o", tc.name, out.Shorthand)
		}
	}
}

// TestValidateVCPageSize 验证 page-size 校验放行 0（默认）与 20-100，拒绝 1-19 与 >100
func TestValidateVCPageSize(t *testing.T) {
	cases := []struct {
		pageSize int
		wantErr  bool
	}{
		{0, false},   // 未传，回落默认
		{1, true},    // 下界以下
		{5, true},    // lark/help 声明 20-100，1-19 应拒绝
		{19, true},   // 边界
		{20, false},  // 下界
		{50, false},  // 中间
		{100, false}, // 上界
		{101, true},  // 上界以上
		{-1, true},   // 负数
	}
	for _, tc := range cases {
		err := validateVCPageSize(tc.pageSize)
		if tc.wantErr && err == nil {
			t.Errorf("page-size=%d: expected error, got nil", tc.pageSize)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("page-size=%d: unexpected error: %v", tc.pageSize, err)
		}
	}
}

// TestVCStartAfterEnd 验证 start/end 用 int64 数值比较（而非字符串字典序）。
func TestVCStartAfterEnd(t *testing.T) {
	cases := []struct {
		name       string
		start, end string
		wantAfter  bool
		wantErr    bool
	}{
		// 位数不同：字典序 "999999999" > "1000000000"（误判 start 晚于 end），数值序则 start 早于 end
		{"位数不同数值序正确-start早于end", "999999999", "1000000000", false, false},
		// 位数不同反向：start 数值确实大于 end
		{"位数不同start确实晚", "1000000000", "999999999", true, false},
		{"同位数start晚", "200", "100", true, false},
		{"同位数start早", "100", "200", false, false},
		{"相等", "100", "100", false, false},
		{"start空跳过", "", "100", false, false},
		{"end空跳过", "100", "", false, false},
		{"非数值报错", "abc", "100", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			after, err := vcStartAfterEnd(tc.start, tc.end)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got after=%v", after)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if after != tc.wantAfter {
				t.Fatalf("vcStartAfterEnd(%q,%q) = %v, want %v", tc.start, tc.end, after, tc.wantAfter)
			}
		})
	}
}

// TestVCParseTimeToUnixSec 验证时间字符串到 Unix 秒的转换
func TestVCParseTimeToUnixSec(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got, err := vcParseTimeToUnixSec("", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("empty input should yield empty string, got %q", got)
		}
	})

	t.Run("rfc3339", func(t *testing.T) {
		in := "2026-03-01T00:00:00+08:00"
		got, err := vcParseTimeToUnixSec(in, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.FixedZone("", 8*3600)).Unix()
		if got != strconv.FormatInt(want, 10) {
			t.Fatalf("got %q, want %d", got, want)
		}
	})

	t.Run("date end aligns later than start", func(t *testing.T) {
		startSec, err := vcParseTimeToUnixSec("2026-03-01", false)
		if err != nil {
			t.Fatalf("start err: %v", err)
		}
		endSec, err := vcParseTimeToUnixSec("2026-03-01", true)
		if err != nil {
			t.Fatalf("end err: %v", err)
		}
		if startSec == "" || endSec == "" {
			t.Fatal("expected non-empty seconds")
		}
		s, _ := strconv.ParseInt(startSec, 10, 64)
		e, _ := strconv.ParseInt(endSec, 10, 64)
		if e-s != 86399 {
			t.Fatalf("end-start = %d, want 86399 (23:59:59 alignment)", e-s)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		if _, err := vcParseTimeToUnixSec("nonsense", false); err == nil {
			t.Fatal("expected error for invalid input")
		}
	})
}
