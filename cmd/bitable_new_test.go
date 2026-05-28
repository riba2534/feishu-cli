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

// TestBitableRunDryRunNoTokenNeeded 验证 bitableRun 的 dry-run 分支：
// ① 不要求 User Token（dry-run 不发请求，不应因缺登录态失败）
// ② 预览内容含 method/path/api 版本，且不实调 API
func TestBitableRunDryRunNoTokenNeeded(t *testing.T) {
	viper.Reset()
	tmp := t.TempDir()
	cfgFile := tmp + "/config.yaml"
	if err := os.WriteFile(cfgFile, []byte("app_id: a\napp_secret: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(cfgFile); err != nil {
		t.Fatalf("config.Init: %v", err)
	}

	cmd := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	addBitableWriteFlags(cmd)
	_ = cmd.Flags().Set("base-token", "bascn1")
	_ = cmd.Flags().Set("dry-run", "true")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := bitableRun(cmd, func(bt string) bitableReq {
		return bitableReq{method: "PUT", path: "/open-apis/bitable/v1/apps/" + bt, body: map[string]any{"name": "x"}, useV1: true}
	})
	_ = w.Close()
	os.Stdout = old
	outBytes, _ := io.ReadAll(r)
	out := string(outBytes)

	if err != nil {
		t.Fatalf("dry-run 不应报错（即使无 User Token）: %v", err)
	}
	if !strings.Contains(out, `"method": "PUT"`) || !strings.Contains(out, "bascn1") || !strings.Contains(out, "bitable/v1") {
		t.Errorf("dry-run 预览内容不对: %s", out)
	}
}

// newFormPatchTestCmd 构造一个带 form patch flag 的命令用于测试 buildFormPatchBody。
func newFormPatchTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "patch", Run: func(*cobra.Command, []string) {}}
	c.Flags().String("name", "", "")
	c.Flags().String("description", "", "")
	c.Flags().Bool("shared", false, "")
	c.Flags().String("shared-limit", "", "")
	c.Flags().Bool("submit-limit-once", false, "")
	c.Flags().String("config", "", "")
	c.Flags().String("config-file", "", "")
	return c
}

func TestBuildFormPatchBodyConvenienceFlags(t *testing.T) {
	c := newFormPatchTestCmd()
	_ = c.Flags().Set("name", "新表单名")
	_ = c.Flags().Set("shared", "true")
	body, err := buildFormPatchBody(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["name"] != "新表单名" {
		t.Errorf("name = %v", body["name"])
	}
	if body["shared"] != true {
		t.Errorf("shared = %v", body["shared"])
	}
	// 未设置的字段不应出现（避免覆盖为空）
	if _, ok := body["description"]; ok {
		t.Errorf("未设置的 description 不应出现")
	}
	if _, ok := body["submit_limit_once"]; ok {
		t.Errorf("未设置的 submit_limit_once 不应出现")
	}
}

func TestBuildFormPatchBodySharedLimitEnum(t *testing.T) {
	c := newFormPatchTestCmd()
	_ = c.Flags().Set("shared-limit", "invalid_value")
	if _, err := buildFormPatchBody(c); err == nil {
		t.Errorf("非法 shared-limit 应报错")
	}

	c2 := newFormPatchTestCmd()
	_ = c2.Flags().Set("shared-limit", "tenant_editable")
	body, err := buildFormPatchBody(c2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["shared_limit"] != "tenant_editable" {
		t.Errorf("shared_limit = %v", body["shared_limit"])
	}
}

func TestBuildFormPatchBodyConfigOverride(t *testing.T) {
	c := newFormPatchTestCmd()
	_ = c.Flags().Set("config", `{"name":"from-config","description":"d"}`)
	body, err := buildFormPatchBody(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["name"] != "from-config" || body["description"] != "d" {
		t.Errorf("config 解析不对: %v", body)
	}
}
