package cmd

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TestFormPatchRoutesToBitableV1 锁住 B2 修复：form patch 必须走 bitable/v1 端点。
// 原因：base/v3 的 forms patch 只收 name/description，shared/shared_limit/submit_limit_once
// 是 bitable/v1 字段——之前整体发到 base/v3 导致 shared 等被拒（Unrecognized key 'shared'）。
// 现整体路由到 bitable/v1（5 字段同源），dry-run 断言路径 + method + body 同时含 name 与 shared。
func TestFormPatchRoutesToBitableV1(t *testing.T) {
	viper.Reset()
	tmp := t.TempDir()
	cfgFile := tmp + "/config.yaml"
	if err := os.WriteFile(cfgFile, []byte("app_id: a\napp_secret: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(cfgFile); err != nil {
		t.Fatalf("config.Init: %v", err)
	}

	cmd := &cobra.Command{Use: "patch", Run: func(*cobra.Command, []string) {}}
	addBitableWriteFlags(cmd)
	cmd.Flags().String("table-id", "", "")
	cmd.Flags().String("form-id", "", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().Bool("shared", false, "")
	cmd.Flags().String("shared-limit", "", "")
	cmd.Flags().Bool("submit-limit-once", false, "")
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("config-file", "", "")
	_ = cmd.Flags().Set("base-token", "bascn1")
	_ = cmd.Flags().Set("table-id", "tblX")
	_ = cmd.Flags().Set("form-id", "vewF")
	_ = cmd.Flags().Set("name", "新表单")
	_ = cmd.Flags().Set("shared", "true")
	_ = cmd.Flags().Set("dry-run", "true")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := bitableFormPatchCmd.RunE(cmd, nil)
	_ = w.Close()
	os.Stdout = old
	outBytes, _ := io.ReadAll(r)
	out := string(outBytes)

	if err != nil {
		t.Fatalf("dry-run 不应报错: %v", err)
	}
	if !strings.Contains(out, "/open-apis/bitable/v1/apps/bascn1/tables/tblX/forms/vewF") {
		t.Errorf("form patch 路径应为 bitable/v1 forms 端点: %s", out)
	}
	if !strings.Contains(out, `"method": "PATCH"`) {
		t.Errorf("form patch 应为 PATCH: %s", out)
	}
	if !strings.Contains(out, `"bitable/v1"`) {
		t.Errorf("api 版本应为 bitable/v1: %s", out)
	}
	if !strings.Contains(out, `"shared": true`) || !strings.Contains(out, "新表单") {
		t.Errorf("body 应同时含 shared:true 与 name: %s", out)
	}
}

// TestFormListUsesBaseV3FormsPath 锁住 B4：form list 走 base/v3 forms collection 端点
// GET /open-apis/base/v3/bases/{bt}/tables/{tid}/forms（带 X-App-Id），page_size 透传。
// 用真 httptest 服务器跑通整条 RunE（list 只读、无 dry-run，与 form get 一致）。
func TestFormListUsesBaseV3FormsPath(t *testing.T) {
	var gotPath, gotMethod, gotAppID, gotPageSize string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAppID = r.Header.Get("X-App-Id")
		gotPageSize = r.URL.Query().Get("page_size")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"","data":{"items":[{"form_id":"vewF","name":"问卷"}],"has_more":false}}`)
	})
	defer cleanup()

	cmd := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	addBitableCommonFlags(cmd) // 含 --base-token / --user-access-token / --format / --jq
	cmd.Flags().String("table-id", "", "")
	cmd.Flags().Int("page-size", 0, "")
	cmd.Flags().String("page-token", "", "")
	_ = cmd.Flags().Set("base-token", "bascn1")
	_ = cmd.Flags().Set("table-id", "tblX")
	_ = cmd.Flags().Set("page-size", "50")
	_ = cmd.Flags().Set("user-access-token", "u-test")

	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w
	err := bitableFormListCmd.RunE(cmd, nil)
	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("form list RunE 报错: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/open-apis/base/v3/bases/bascn1/tables/tblX/forms" {
		t.Errorf("path = %s, want base/v3 forms collection", gotPath)
	}
	if gotAppID == "" {
		t.Errorf("base/v3 请求应带 X-App-Id header")
	}
	if gotPageSize != "50" {
		t.Errorf("page_size 透传 = %q, want 50", gotPageSize)
	}
}
