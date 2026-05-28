package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// initDashboardTestConfig 初始化最小 config，使 bitableRun 通过 config.Validate。
func initDashboardTestConfig(t *testing.T) {
	t.Helper()
	viper.Reset()
	tmp := t.TempDir()
	cfgFile := tmp + "/config.yaml"
	if err := os.WriteFile(cfgFile, []byte("app_id: a\napp_secret: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(cfgFile); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
}

// captureBitableRunDryRun 构造一个独立命令、注册指定 flag、设 --dry-run 后跑 bitableRun，
// 捕获其 dry-run 预览 stdout。这样不复用全局命令树，避免 flag 重复注册 panic，
// 直接验证 build 闭包产出的 method/path/body。
func captureBitableRunDryRun(t *testing.T, flags map[string]string, build func(bt string) bitableReq) string {
	t.Helper()
	initDashboardTestConfig(t)

	cmd := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	addBitableWriteFlags(cmd)
	_ = cmd.Flags().Set("base-token", "bascn1")
	_ = cmd.Flags().Set("dry-run", "true")
	for k, v := range flags {
		cmd.Flags().String(k, "", "")
		_ = cmd.Flags().Set(k, v)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := bitableRun(cmd, build)
	_ = w.Close()
	os.Stdout = old
	outBytes, _ := io.ReadAll(r)
	out := string(outBytes)
	if err != nil {
		t.Fatalf("dry-run 不应报错: %v\n输出: %s", err, out)
	}
	return out
}

func mustContain(t *testing.T, out string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(out, s) {
			t.Errorf("输出应含 %q，实际:\n%s", s, out)
		}
	}
}

func mustNotContain(t *testing.T, out string, sub string) {
	t.Helper()
	if strings.Contains(out, sub) {
		t.Errorf("输出不应含 %q，实际:\n%s", sub, out)
	}
}

// ---- dry-run path/method/body 断言（端点 ground truth = lark-cli dry-run）----

func TestDashboardCreateDryRun(t *testing.T) {
	out := captureBitableRunDryRun(t, nil, func(bt string) bitableReq {
		return bitableReq{method: "POST", path: dashboardPath(bt, ""),
			body: map[string]any{"name": "看板A", "theme": map[string]any{"theme_style": "dark"}}}
	})
	mustContain(t, out,
		`"method": "POST"`,
		`/open-apis/base/v3/bases/bascn1/dashboards`,
		`"base/v3"`,
		`"name": "看板A"`,
		`"theme_style": "dark"`)
	mustNotContain(t, out, "bitable/v1")
}

func TestDashboardGetDryRun(t *testing.T) {
	out := captureBitableRunDryRun(t, nil, func(bt string) bitableReq {
		return bitableReq{method: "GET", path: dashboardPath(bt, "dsbX")}
	})
	mustContain(t, out, `"method": "GET"`, `/open-apis/base/v3/bases/bascn1/dashboards/dsbX`)
}

func TestDashboardUpdateDryRun(t *testing.T) {
	out := captureBitableRunDryRun(t, nil, func(bt string) bitableReq {
		return bitableReq{method: "PATCH", path: dashboardPath(bt, "dsbX"),
			body: map[string]any{"name": "新名"}}
	})
	mustContain(t, out, `"method": "PATCH"`, `/open-apis/base/v3/bases/bascn1/dashboards/dsbX`, `"name": "新名"`)
}

func TestDashboardDeleteDryRun(t *testing.T) {
	out := captureBitableRunDryRun(t, nil, func(bt string) bitableReq {
		return bitableReq{method: "DELETE", path: dashboardPath(bt, "dsbX")}
	})
	mustContain(t, out, `"method": "DELETE"`, `/open-apis/base/v3/bases/bascn1/dashboards/dsbX`)
}

func TestDashboardArrangeDryRun(t *testing.T) {
	out := captureBitableRunDryRun(t, nil, func(bt string) bitableReq {
		return bitableReq{method: "POST", path: dashboardPath(bt, "dsbX", "arrange"), body: map[string]any{}}
	})
	mustContain(t, out, `"method": "POST"`, `/open-apis/base/v3/bases/bascn1/dashboards/dsbX/arrange`)
}

func TestDashboardBlockCreateDryRun(t *testing.T) {
	out := captureBitableRunDryRun(t, nil, func(bt string) bitableReq {
		return bitableReq{method: "POST", path: dashboardPath(bt, "dsbX", "blocks"),
			body: map[string]any{"name": "图1", "type": "pie",
				"data_config": map[string]any{"table_name": "t1", "count_all": true}}}
	})
	mustContain(t, out,
		`"method": "POST"`,
		`/open-apis/base/v3/bases/bascn1/dashboards/dsbX/blocks`,
		`"type": "pie"`,
		`"table_name": "t1"`,
		`"count_all": true`)
}

func TestDashboardBlockGetDryRun(t *testing.T) {
	out := captureBitableRunDryRun(t, nil, func(bt string) bitableReq {
		return bitableReq{method: "GET", path: dashboardPath(bt, "dsbX", "blocks", "blkY")}
	})
	mustContain(t, out, `"method": "GET"`, `/open-apis/base/v3/bases/bascn1/dashboards/dsbX/blocks/blkY`)
}

func TestDashboardBlockListDryRun(t *testing.T) {
	out := captureBitableRunDryRun(t, nil, func(bt string) bitableReq {
		return bitableReq{method: "GET", path: dashboardPath(bt, "dsbX", "blocks"),
			params: map[string]any{"page_size": 50, "page_token": "PT"}}
	})
	mustContain(t, out,
		`"method": "GET"`,
		`/open-apis/base/v3/bases/bascn1/dashboards/dsbX/blocks`,
		`"page_size": 50`,
		`"page_token": "PT"`)
}

func TestDashboardBlockUpdateDryRun(t *testing.T) {
	out := captureBitableRunDryRun(t, nil, func(bt string) bitableReq {
		return bitableReq{method: "PATCH", path: dashboardPath(bt, "dsbX", "blocks", "blkY"),
			body: map[string]any{"name": "图1改", "data_config": map[string]any{"text": "hello"}}}
	})
	mustContain(t, out,
		`"method": "PATCH"`,
		`/open-apis/base/v3/bases/bascn1/dashboards/dsbX/blocks/blkY`,
		`"name": "图1改"`,
		`"text": "hello"`)
}

func TestDashboardBlockDeleteDryRun(t *testing.T) {
	out := captureBitableRunDryRun(t, nil, func(bt string) bitableReq {
		return bitableReq{method: "DELETE", path: dashboardPath(bt, "dsbX", "blocks", "blkY")}
	})
	mustContain(t, out, `"method": "DELETE"`, `/open-apis/base/v3/bases/bascn1/dashboards/dsbX/blocks/blkY`)
}

// ---- dashboardPath 构造单测 ----

func TestDashboardPath(t *testing.T) {
	cases := []struct {
		bt, id string
		extra  []string
		want   string
	}{
		{"bascn1", "", nil, "/open-apis/base/v3/bases/bascn1/dashboards"},
		{"bascn1", "dsbX", nil, "/open-apis/base/v3/bases/bascn1/dashboards/dsbX"},
		{"bascn1", "dsbX", []string{"arrange"}, "/open-apis/base/v3/bases/bascn1/dashboards/dsbX/arrange"},
		{"bascn1", "dsbX", []string{"blocks"}, "/open-apis/base/v3/bases/bascn1/dashboards/dsbX/blocks"},
		{"bascn1", "dsbX", []string{"blocks", "blkY"}, "/open-apis/base/v3/bases/bascn1/dashboards/dsbX/blocks/blkY"},
	}
	for _, c := range cases {
		if got := dashboardPath(c.bt, c.id, c.extra...); got != c.want {
			t.Errorf("dashboardPath(%q,%q,%v) = %q, want %q", c.bt, c.id, c.extra, got, c.want)
		}
	}
}

// ---- body builder 单测 ----

func newDashboardBodyTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	c.Flags().String("name", "", "")
	c.Flags().String("theme-style", "", "")
	c.Flags().String("config", "", "")
	c.Flags().String("config-file", "", "")
	return c
}

func TestDashboardBuildCreateOrUpdateBodyConvenience(t *testing.T) {
	c := newDashboardBodyTestCmd()
	_ = c.Flags().Set("name", "看板A")
	_ = c.Flags().Set("theme-style", "dark")
	body, err := dashboardBuildCreateOrUpdateBody(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["name"] != "看板A" {
		t.Errorf("name = %v", body["name"])
	}
	theme, ok := body["theme"].(map[string]any)
	if !ok || theme["theme_style"] != "dark" {
		t.Errorf("theme 应嵌套 theme_style=dark: %v", body["theme"])
	}
}

func TestDashboardBuildCreateOrUpdateBodyConfigOverride(t *testing.T) {
	c := newDashboardBodyTestCmd()
	_ = c.Flags().Set("config", `{"name":"from-config"}`)
	body, err := dashboardBuildCreateOrUpdateBody(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["name"] != "from-config" {
		t.Errorf("config 解析不对: %v", body)
	}
}

func TestDashboardBuildCreateOrUpdateBodyEmpty(t *testing.T) {
	c := newDashboardBodyTestCmd()
	body, err := dashboardBuildCreateOrUpdateBody(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("无字段时应返回空 body: %v", body)
	}
}

func newDashboardBlockBodyTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	c.Flags().String("name", "", "")
	c.Flags().String("type", "", "")
	c.Flags().String("data-config", "", "")
	c.Flags().String("config", "", "")
	c.Flags().String("config-file", "", "")
	return c
}

func TestDashboardBuildBlockBodyCreateRequiresType(t *testing.T) {
	c := newDashboardBlockBodyTestCmd()
	_ = c.Flags().Set("name", "图1")
	if _, err := dashboardBuildBlockBody(c, true); err == nil {
		t.Error("create 模式缺 --type 应报错")
	}
}

func TestDashboardBuildBlockBodyCreateInvalidType(t *testing.T) {
	c := newDashboardBlockBodyTestCmd()
	_ = c.Flags().Set("type", "nope")
	if _, err := dashboardBuildBlockBody(c, true); err == nil {
		t.Error("非法 --type 应报错")
	}
}

func TestDashboardBuildBlockBodyCreateValid(t *testing.T) {
	c := newDashboardBlockBodyTestCmd()
	_ = c.Flags().Set("type", "pie")
	_ = c.Flags().Set("data-config", `{"table_name":"t1"}`)
	body, err := dashboardBuildBlockBody(c, true)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["type"] != "pie" {
		t.Errorf("type = %v", body["type"])
	}
	dc, ok := body["data_config"].(map[string]any)
	if !ok || dc["table_name"] != "t1" {
		t.Errorf("data_config 解析不对: %v", body["data_config"])
	}
}

func TestDashboardBuildBlockBodyUpdateNoType(t *testing.T) {
	c := newDashboardBlockBodyTestCmd()
	_ = c.Flags().Set("name", "改名")
	body, err := dashboardBuildBlockBody(c, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["name"] != "改名" {
		t.Errorf("name = %v", body["name"])
	}
	if _, ok := body["type"]; ok {
		t.Errorf("update 模式不应注入 type: %v", body)
	}
}

func TestDashboardBuildBlockBodyConfigOverride(t *testing.T) {
	c := newDashboardBlockBodyTestCmd()
	_ = c.Flags().Set("config", `{"name":"c","type":"text","data_config":{"text":"hi"}}`)
	body, err := dashboardBuildBlockBody(c, true)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["type"] != "text" || body["name"] != "c" {
		t.Errorf("config 直传不对: %v", body)
	}
}
