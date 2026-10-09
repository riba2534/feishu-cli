package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
)

// P1-7：perm 不传 --as 保持默认 Bot（不被 FEISHU_USER_ACCESS_TOKEN 静默切换）；--as / --user-access-token 显式切换。
func TestPermIdentitySelection(t *testing.T) {
	_, reqs := newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[]}}`)
	})
	cases := []struct {
		name     string
		env      string
		flags    []string
		wantAuth string
		wantErr  bool
	}{
		{name: "默认 Bot", wantAuth: "Bearer t-test-token"},
		{name: "环境变量不改变默认身份", env: "u-env", wantAuth: "Bearer t-test-token"},
		{name: "显式 --user-access-token", flags: []string{"--user-access-token", "u-flag"}, wantAuth: "Bearer u-flag"},
		{name: "--as bot 强制 App", flags: []string{"--as", "bot", "--user-access-token", "u-flag"}, wantAuth: "Bearer t-test-token"},
		{name: "--as user", flags: []string{"--as", "user", "--user-access-token", "u-flag"}, wantAuth: "Bearer u-flag"},
		{name: "--as 非法", flags: []string{"--as", "root"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FEISHU_USER_ACCESS_TOKEN", tc.env)
			before := len(reqs())
			_, _, err := runCmdWithFlags(t, listPermissionCmd, []string{"doc1"}, tc.flags...)
			if tc.wantErr {
				if !clierr.HasKind(err, clierr.KindUsage) || len(reqs()) != before {
					t.Fatalf("期望用法错误且不发请求: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := reqs()[len(reqs())-1]
			if got.Auth != tc.wantAuth || got.Path != "/open-apis/drive/v1/permissions/doc1/members" {
				t.Fatalf("auth=%q path=%q, want %q", got.Auth, got.Path, tc.wantAuth)
			}
		})
	}
}

func TestPermErrorHints(t *testing.T) {
	base := fmt.Errorf("获取权限列表失败: code=1063004, msg=User has no share permission")
	if err := wrapPermError(base, ""); !strings.Contains(err.Error(), "--as user") || !client.HasAPICode(err, 1063004) {
		t.Fatalf("Bot 身份 1063004 应提示 --as user 并保留错误码: %v", err)
	}
	if err := wrapPermError(base, "u-x"); strings.Contains(err.Error(), "--as user") || !strings.Contains(err.Error(), "所有者") {
		t.Fatalf("User 身份 1063004 提示不对: %v", err)
	}
	denied := fmt.Errorf("x: code=1063002, msg=Permission denied")
	if err := wrapPermError(denied, ""); !strings.Contains(err.Error(), "--as user") {
		t.Fatalf("Bot 身份 1063002 应提示 --as user: %v", err)
	}
	other := fmt.Errorf("x: code=1063001, msg=bad")
	if err := wrapPermError(other, ""); err != other {
		t.Fatalf("其他错误原样返回: %v", err)
	}
}

func TestPermPublicGetMergesV2AndV1(t *testing.T) {
	v1Fails := false
	_, reqs := newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		switch r.URL.Path {
		case "/open-apis/drive/v2/permissions/doc1/public":
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"permission_public":{"external_access_entity":"closed","copy_entity":"anyone_can_view","manage_collaborator_entity":"collaborator_can_view","link_share_entity":"closed","security_entity":"anyone_can_view","comment_entity":"anyone_can_view","share_entity":"same_tenant","lock_switch":false}}}`)
		case "/open-apis/drive/v1/permissions/doc1/public":
			if v1Fails {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"code":1063002,"msg":"denied"}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"permission_public":{"external_access":false,"invite_external":true,"link_share_entity":"closed","security_entity":"anyone_can_view","comment_entity":"anyone_can_view","share_entity":"same_tenant","lock_switch":false}}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	})

	out, _, err := runCmdWithFlags(t, getPublicPermissionCmd, []string{"doc1"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"copy_entity", "manage_collaborator_entity", "external_access_entity", "external_access", "invite_external", "link_share_entity", "security_entity", "comment_entity", "share_entity", "lock_switch"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("输出缺少字段 %s: %v", k, got)
		}
	}
	if got["invite_external"] != true || got["external_access"] != false {
		t.Fatalf("v1 旧字段取值不对: %v", got)
	}
	if q := reqs()[0].Query; q != "type=docx" {
		t.Fatalf("v2 query = %q", q)
	}

	// v1 失败时 external_access 由 external_access_entity 推断
	v1Fails = true
	out, errOut, err := runCmdWithFlags(t, getPublicPermissionCmd, []string{"doc1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got["external_access"] != false || !strings.Contains(errOut, "补读 v1") {
		t.Fatalf("v1 失败时应推断 external_access 并告警: %v / %q", got, errOut)
	}
}

func TestPermAddPermTypeAndBodyType(t *testing.T) {
	_, reqs := newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"member":{}}}`)
	})
	if _, _, err := runCmdWithFlags(t, addPermissionCmd, []string{"wikcn1"}, "--doc-type", "wiki", "--member-type", "openid", "--member-id", "ou_x", "--perm", "view", "--perm-type", "single_page"); err != nil {
		t.Fatal(err)
	}
	if body := reqs()[0].Body; !strings.Contains(body, `"perm_type":"single_page"`) {
		t.Fatalf("wiki 应下发 perm_type: %s", body)
	}
	if _, _, err := runCmdWithFlags(t, addPermissionCmd, []string{"doc1"}, "--member-type", "openid", "--member-id", "ou_x", "--perm", "view"); err != nil {
		t.Fatal(err)
	}
	if body := reqs()[1].Body; strings.Contains(body, "perm_type") {
		t.Fatalf("未指定 --perm-type 时不应下发: %s", body)
	}
	n := len(reqs())
	if _, _, err := runCmdWithFlags(t, addPermissionCmd, []string{"doc1"}, "--member-type", "openid", "--member-id", "ou_x", "--perm", "view", "--perm-type", "container"); !clierr.HasKind(err, clierr.KindUsage) || len(reqs()) != n {
		t.Fatalf("非 wiki 指定 --perm-type 应为用法错误且不发请求: %v", err)
	}
	if _, err := validatePermType("all", "wiki"); !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("非法 perm_type 应为用法错误: %v", err)
	}
}

// P2：apply-permission 先解析业务信封——HTTP 200 + code!=0 不能再 exit 0；1063006/1063007 给中文指引。
func TestApplyPermissionBusinessErrors(t *testing.T) {
	status, payload := http.StatusOK, `{"code":1063006,"msg":"quota"}`
	newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, payload)
	})
	_, _, err := runCmdWithFlags(t, drivePermApplyCmd, nil, "--token", "doxcn1", "--type", "docx", "--user-access-token", "u-x")
	if err == nil || !client.HasAPICode(err, 1063006) || !strings.Contains(err.Error(), "5 次") {
		t.Fatalf("HTTP 200 + 1063006 必须失败并提示额度: %v", err)
	}

	status, payload = http.StatusBadRequest, `{"code":1063007,"msg":"not allowed"}`
	_, _, err = runCmdWithFlags(t, drivePermApplyCmd, nil, "--token", "doxcn1", "--type", "docx", "--user-access-token", "u-x")
	if _, ok := client.AsAPIError(err); !ok || !strings.Contains(err.Error(), "不接受权限申请") {
		t.Fatalf("HTTP 400 + 1063007 应解析业务码并提示: %v", err)
	}

	status, payload = http.StatusOK, `{"code":0,"data":{}}`
	out, _, err := runCmdWithFlags(t, drivePermApplyCmd, nil, "--token", "doxcn1", "--type", "docx", "--user-access-token", "u-x")
	if err != nil || !strings.Contains(out, `"code":0`) {
		t.Fatalf("成功时应打印响应: %v %s", err, out)
	}
}
