package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// isolateBitableNoCredEnv 临时 HOME、无任何 App 凭证 / User Token，base_url 指向计数桩。
// 静态 dry-run 与用法错误都不应发出任何请求。
func isolateBitableNoCredEnv(t *testing.T) *int32 {
	t.Helper()
	viper.Reset() // 清掉前序用例 config.Init 留下的 app_id/base_url
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_PROFILE", "")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	t.Setenv("FEISHU_CLI_REMOTE_META", "off")
	config.SetBotFlagCredentials("", "")
	if err := os.MkdirAll(filepath.Join(home, ".feishu-cli"), 0o700); err != nil {
		t.Fatal(err)
	}
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "不应发起网络请求", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FEISHU_BASE_URL", srv.URL)
	return &hits
}

// 存量 bitable 写命令（以及读命令）补齐 --dry-run：无凭据下 exit 0、不联网，预览含 method/path。
func TestBitableLegacyDryRunWithoutCredentials(t *testing.T) {
	bt := []string{"--base-token", "bascnxxx"}
	tb := append(append([]string{}, bt...), "--table-id", "tblxxx")
	cases := []struct {
		args   []string
		method string
		path   string
	}{
		{append([]string{"bitable", "table", "delete"}, tb...), "DELETE", "/open-apis/base/v3/bases/bascnxxx/tables/tblxxx"},
		{append([]string{"bitable", "table", "create", "--name", "t"}, bt...), "POST", "/open-apis/base/v3/bases/bascnxxx/tables"},
		{append([]string{"bitable", "table", "update", "--name", "n"}, tb...), "PATCH", "/open-apis/base/v3/bases/bascnxxx/tables/tblxxx"},
		{append([]string{"bitable", "field", "delete", "--field-id", "fldx"}, tb...), "DELETE", "/fields/fldx"},
		{append([]string{"bitable", "field", "create", "--config", `{"name":"a","type":"text"}`}, tb...), "POST", "/tables/tblxxx/fields"},
		{append([]string{"bitable", "field", "update", "--field-id", "fldx", "--config", `{"name":"a","type":"text"}`}, tb...), "PUT", "/fields/fldx"},
		{append([]string{"bitable", "record", "upsert", "--config", `{"fields":{"名称":"x"}}`}, tb...), "POST", "/tables/tblxxx/records"},
		{append([]string{"bitable", "record", "delete", "--record-id", "recx"}, tb...), "DELETE", "/records/recx"},
		{append([]string{"bitable", "record", "batch-delete", "--record-ids", "rec1,rec2"}, tb...), "POST", "/records/batch_delete"},
		{append([]string{"bitable", "record", "batch-create", "--config", `{"create_records":[{"名称":"x"}]}`}, tb...), "POST", "/records/batch_create"},
		{append([]string{"bitable", "view", "delete", "--view-id", "vewx"}, tb...), "DELETE", "/views/vewx"},
		{append([]string{"bitable", "view", "view-sort-set", "--view-id", "vewx", "--config", `[{"field":"a","desc":true}]`}, tb...), "PUT", "/views/vewx/sort"},
		{append([]string{"bitable", "role", "delete", "--role-id", "rolx"}, bt...), "DELETE", "/roles/rolx"},
		{append([]string{"bitable", "advperm", "disable"}, bt...), "PUT", "/advperm/enable"},
		{append([]string{"bitable", "copy", "--name", "副本", "--time-zone", "Asia/Shanghai"}, bt...), "POST", "/bases/bascnxxx/copy"},
		{[]string{"bitable", "create", "--name", "e2e"}, "POST", "/open-apis/base/v3/bases"},
		{append([]string{"bitable", "record", "list"}, tb...), "GET", "/tables/tblxxx/records"},
		{append([]string{"bitable", "workflow", "list"}, bt...), "POST", "/workflows/list"},
		{append([]string{"bitable", "record", "upload-attachment", "--record-id", "recx", "--field-id", "fldx", "--file", "/tmp/not-exist.pdf"}, tb...), "POST", "append_attachments"},
	}
	for _, tc := range cases {
		hits := isolateBitableNoCredEnv(t)
		args := append(append([]string{}, tc.args...), "--dry-run")
		stdout, stderr, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v: 无凭据 dry-run 应成功，exit=%d err=%v stderr=%s", tc.args, exitCodeFor(err), err, stderr)
		}
		if !strings.Contains(stdout, `"`+tc.method+`"`) || !strings.Contains(stdout, tc.path) {
			t.Errorf("%v: 预览缺少 %s %s:\n%s", tc.args, tc.method, tc.path, stdout)
		}
		if n := atomic.LoadInt32(hits); n != 0 {
			t.Errorf("%v: dry-run 不应联网，命中 %d 次", tc.args, n)
		}
	}
}

// 去掉 --dry-run 后仍需凭据：无凭据 exit 3。
func TestBitableLegacyWithoutDryRunStillNeedsCredentials(t *testing.T) {
	for _, args := range [][]string{
		{"bitable", "table", "delete", "--base-token", "bascnxxx", "--table-id", "tblxxx"},
		{"bitable", "copy", "--base-token", "bascnxxx", "--name", "副本"},
		{"bitable", "record", "upload-attachment", "--base-token", "bascnxxx", "--table-id", "tblx", "--record-id", "recx", "--field-id", "fldx", "--file", "/tmp/not-exist.pdf"},
	} {
		hits := isolateBitableNoCredEnv(t)
		_, _, err := runCLI(t, args...)
		if got := exitCodeFor(err); got != clierr.ExitAuth {
			t.Errorf("%v: exit=%d want %d (err=%v)", args, got, clierr.ExitAuth, err)
		}
		if n := atomic.LoadInt32(hits); n != 0 {
			t.Errorf("%v: 缺凭据不应联网，命中 %d 次", args, n)
		}
	}
}

// 参数错误一律 exit 2（文本不变），且先于凭据检查：有凭据、无凭据都是 2，均不联网。
func TestBitableParamErrorsAreUsageErrors(t *testing.T) {
	bt := []string{"--base-token", "bascnxxx"}
	tb := append(append([]string{}, bt...), "--table-id", "tblxxx")
	cases := []struct {
		args []string
		msg  string
	}{
		{append([]string{"bitable", "record", "delete"}, tb...), "--record-id 必填"},
		{append([]string{"bitable", "record", "get"}, tb...), "--record-id 必填"},
		{append([]string{"bitable", "view", "get"}, tb...), "--view-id 必填"},
		{append([]string{"bitable", "field", "delete"}, tb...), "--field-id 必填"},
		{append([]string{"bitable", "dashboard", "delete", "--dry-run"}, bt...), "--dashboard-id 必填"},
		{append([]string{"bitable", "role", "get"}, bt...), "--role-id 必填"},
		{append([]string{"bitable", "table", "update", "--table-id", "tblx"}, bt...), "需要 --name 或 --config 至少一个"},
		{append([]string{"bitable", "field", "create", "--config", "{bad"}, tb...), "解析 --config 失败"},
		{append([]string{"bitable", "record", "search", "--keyword", "x"}, tb...), "--keyword 必须配合至少一个 --search-field"},
		{append([]string{"bitable", "record", "batch-delete"}, tb...), "--record-ids 或 --from-file 至少需要提供一个"},
		{append([]string{"bitable", "record", "upload-attachment", "--record-id", "r"}, tb...), "--table-id / --record-id / --field-id 必填"},
		{append([]string{"bitable", "copy", "--name", " "}, bt...), "--name 必填"},
		{append([]string{"bitable", "workflow", "get"}, bt...), "--workflow-id 必填"},
		{append([]string{"bitable", "field", "list", "--format", "xml"}, tb...), "不支持的 --format"},
	}
	for _, env := range []string{"fake-creds", "no-creds"} {
		for _, tc := range cases {
			var hits *int32
			if env == "fake-creds" {
				hits = isolatePlatformCLIEnv(t)
			} else {
				hits = isolateBitableNoCredEnv(t)
			}
			_, stderr, err := runCLI(t, tc.args...)
			if got := exitCodeFor(err); got != clierr.ExitUsage {
				t.Errorf("[%s] %v: exit=%d want 2 (err=%v)", env, tc.args, got, err)
				continue
			}
			if !strings.Contains(err.Error()+stderr, tc.msg) {
				t.Errorf("[%s] %v: 错误应包含 %q，得到 %v", env, tc.args, tc.msg, err)
			}
			if n := atomic.LoadInt32(hits); n != 0 {
				t.Errorf("[%s] %v: 用法错误不应联网，命中 %d 次", env, tc.args, n)
			}
		}
	}
}

// 不传 --format/--jq 时输出与 printJSON 逐字节一致；传了才走 output 包。
func TestBitableLegacyDefaultOutputUnchanged(t *testing.T) {
	resp := map[string]any{"id": "fld1", "name": "<名称>&", "type": "text", "n": json.Number("12345678901234567890")}
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		bitableWriteJSON(w, map[string]any{"code": 0, "data": resp})
	})
	defer cleanup()
	newCmd := func() *cobra.Command {
		c := newBitableTestCmd(func(c *cobra.Command) {
			c.Flags().String("table-id", "", "")
			c.Flags().String("field-id", "", "")
		})
		addBitablePreviewFlags(c)
		_ = c.Flags().Set("base-token", "bascn1")
		_ = c.Flags().Set("table-id", "tbl1")
		_ = c.Flags().Set("field-id", "fld1")
		_ = c.Flags().Set("user-access-token", "u-test")
		return c
	}
	out, _, err := runCaptured(t, func() error { return bitableFieldGetCmd.RunE(newCmd(), nil) })
	if err != nil {
		t.Fatal(err)
	}
	want, _, _ := runCaptured(t, func() error {
		return printJSON(map[string]any{"id": "fld1", "name": "<名称>&", "type": "text", "n": json.Number("12345678901234567890")})
	})
	if out != want {
		t.Errorf("默认输出应与 printJSON 一致:\n got: %q\nwant: %q", out, want)
	}

	c := newCmd()
	_ = c.Flags().Set("jq", ".name")
	out, _, err = runCaptured(t, func() error { return bitableFieldGetCmd.RunE(c, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != `"<名称>&"` {
		t.Errorf("--jq .name 输出不对: %q", out)
	}
}

// 带 -o/--output（json 开关）的旧命令只补 --dry-run，不挂 --format，避免 -o 被当作结果文件。
func TestBitablePreviewFlagsSkipFormatWhenOutputFlagExists(t *testing.T) {
	for _, c := range []*cobra.Command{bitableCreateCmd, bitableGetCmd, bitableCopyCmd} {
		if c.Flags().Lookup("dry-run") == nil {
			t.Errorf("%s 应注册 --dry-run", c.CommandPath())
		}
		if c.Flags().Lookup("format") != nil {
			t.Errorf("%s 不应注册 --format（-o 是 json 开关）", c.CommandPath())
		}
	}
	for _, c := range []*cobra.Command{bitableFieldListCmd, bitableRecordListCmd, bitableTableDeleteCmd, bitableRoleListCmd, bitableWorkflowListCmd, bitableDataQueryCmd} {
		for _, name := range []string{"dry-run", "format", "jq"} {
			if c.Flags().Lookup(name) == nil {
				t.Errorf("%s 缺少 --%s", c.CommandPath(), name)
			}
		}
	}
	// --format 只在注册时生效：未注册的命令 bitableOutputOptions 不读 -o
	c := &cobra.Command{Use: "x"}
	c.Flags().StringP("output", "o", "", "")
	_ = c.Flags().Set("output", "json")
	o, err := bitableOutputOptions(c)
	if err != nil || o.OutputFile != "" || o.Format != output.FormatJSON {
		t.Errorf("未注册 --format 时不应读取 -o: %+v %v", o, err)
	}
}
