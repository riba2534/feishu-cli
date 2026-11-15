package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// 非法 --as 在 config.Validate 之前报用法错误：无凭据、不带 --dry-run 也必须 exit 2（而不是缺 app_id 的 exit 3）。
func TestVCBotInvalidAsExit2BeforeConfigValidate(t *testing.T) {
	clearCmdCredentials(t)
	const mid = "6911188411932033028"
	cases := [][]string{
		{"vc", "bot", "meeting-events", "--meeting-id", mid, "--as", "foo"},
		{"vc", "bot", "meeting-countdown", "--meeting-id", mid, "--action", "set", "--duration", "5", "--as", "foo"},
		{"vc", "bot", "meeting-message-send", "--meeting-id", mid, "--text", "hi", "--as", "foo"},
		{"vc", "bot", "meeting-screenshot", "--meeting-id", mid, "--output", "/tmp/fix-im-meet-x.jpg", "--as", "foo"},
	}
	for _, args := range cases {
		_, _, err := runCLI(t, args...)
		if err == nil || exitCodeFor(err) != clierr.ExitUsage || !strings.Contains(err.Error(), "bot|user|auto") {
			t.Errorf("%v 应 exit 2 且提示 --as 取值，got %d: %v", args, exitCodeFor(err), err)
		}
	}
}

func TestValidateDrivePreviewType(t *testing.T) {
	for _, ok := range []string{"pdf", "pdf_lin", "html", "text", "image", "source_file", "16", "0", "png", "jpg"} {
		if err := validateDrivePreviewType(ok); err != nil {
			t.Errorf("--type %q 应合法: %v", ok, err)
		}
	}
	for _, bad := range []string{"foo", "pdf2", "-1", "1.5"} {
		if err := validateDrivePreviewType(bad); err == nil || exitCodeFor(err) != clierr.ExitUsage {
			t.Errorf("--type %q 应为用法错误 exit 2，got %v", bad, err)
		}
	}
}

// drive preview 非法 --type：--dry-run 与实调都在联网前 exit 2。
func TestDrivePreviewInvalidTypeExit2(t *testing.T) {
	clearCmdCredentials(t)
	for _, extra := range [][]string{nil, {"--dry-run"}} {
		args := append([]string{"drive", "preview", "boxAbcdef", "--type", "foo", "-o", t.TempDir() + "/x"}, extra...)
		_, _, err := runCLI(t, args...)
		if err == nil || exitCodeFor(err) != clierr.ExitUsage || !strings.Contains(err.Error(), "不支持的 --type") {
			t.Errorf("%v 应 exit 2，got %d: %v", args, exitCodeFor(err), err)
		}
	}
}

func newDriveSearchQueryTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "search"}
	c.Flags().String("query", "", "")
	return c
}

func TestResolveDriveSearchQuery(t *testing.T) {
	c := newDriveSearchQueryTestCmd()
	if q, err := resolveDriveSearchQuery(c, []string{"季度报告"}); err != nil || q != "季度报告" {
		t.Fatalf("单个位置参数应作为 query，got %q %v", q, err)
	}
	if q, err := resolveDriveSearchQuery(c, nil); err != nil || q != "" {
		t.Fatalf("无参数应为空 query，got %q %v", q, err)
	}
	if _, err := resolveDriveSearchQuery(c, []string{"a", "b"}); err == nil || exitCodeFor(err) != clierr.ExitUsage {
		t.Fatalf("多个位置参数应 exit 2，got %v", err)
	}
	mustSetFlag(t, c, "query", "x")
	if q, err := resolveDriveSearchQuery(c, nil); err != nil || q != "x" {
		t.Fatalf("--query 应生效，got %q %v", q, err)
	}
	if _, err := resolveDriveSearchQuery(c, []string{"y"}); err == nil || exitCodeFor(err) != clierr.ExitUsage {
		t.Fatalf("位置参数与 --query 同用应 exit 2，got %v", err)
	}
	// 显式传空 --query 也算同时使用
	c2 := newDriveSearchQueryTestCmd()
	mustSetFlag(t, c2, "query", "")
	if _, err := resolveDriveSearchQuery(c2, []string{"y"}); err == nil {
		t.Fatalf("显式 --query \"\" 与位置参数同用应报错")
	}
}

// 端到端：无凭据下，多个位置参数 / 与 --query 同用先报 exit 2。
func TestDriveSearchPositionalUsageErrors(t *testing.T) {
	clearCmdCredentials(t)
	for _, args := range [][]string{
		{"drive", "search", "a", "b"},
		{"drive", "search", "a", "--query", "b"},
	} {
		if _, _, err := runCLI(t, args...); err == nil || exitCodeFor(err) != clierr.ExitUsage {
			t.Errorf("%v 应 exit 2，got %d: %v", args, exitCodeFor(err), err)
		}
	}
}
