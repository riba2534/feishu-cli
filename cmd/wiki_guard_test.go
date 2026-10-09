package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/spf13/cobra"
)

// wikiTestServer 是 wiki 命令测试的 httptest 服务端：统一处理 tenant token，按路径分发到 handler，记录调用。
type wikiTestServer struct {
	mu    sync.Mutex
	calls []string
}

func (s *wikiTestServer) record(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
}

func (s *wikiTestServer) called(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func newWikiTestServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request) bool) (*wikiTestServer, string) {
	t.Helper()
	ws := &wikiTestServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		ws.record(r)
		if !handler(w, r) {
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	initWikiNodeDeleteTestConfig(t, server.URL)
	return ws, server.URL
}

func resetFlags(cmd *cobra.Command, kv map[string]string) {
	for k, v := range kv {
		_ = cmd.Flags().Set(k, v)
		if f := cmd.Flags().Lookup(k); f != nil {
			f.Changed = false
		}
	}
}

// ---------------- wiki nodes / wiki spaces 分页 ----------------

func wikiNodesPageHandler(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/open-apis/wiki/v2/spaces/sp-1/nodes" {
		return false
	}
	switch r.URL.Query().Get("page_token") {
	case "":
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[{"node_token":"wikA","title":"A"}],"has_more":true,"page_token":"pt2"}}`)
	case "pt2":
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[{"node_token":"wikB","title":"B"}],"has_more":false,"page_token":""}}`)
	default:
		return false
	}
	return true
}

func TestWikiNodes_FirstPageKeepsArrayAndHintsPageToken(t *testing.T) {
	newWikiTestServer(t, wikiNodesPageHandler)
	var stderr bytes.Buffer
	listWikiNodesCmd.SetErr(&stderr)
	_ = listWikiNodesCmd.Flags().Set("output", "json")
	defer func() {
		listWikiNodesCmd.SetErr(nil)
		resetFlags(listWikiNodesCmd, map[string]string{"output": "", "page-all": "false", "page-token": ""})
	}()

	out, err := captureCmdStdout(t, func() error { return listWikiNodesCmd.RunE(listWikiNodesCmd, []string{"sp-1"}) })
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	var nodes []map[string]any
	if err := json.Unmarshal([]byte(out), &nodes); err != nil {
		t.Fatalf("JSON 输出必须保持数组形状: %v\n%s", err, out)
	}
	if len(nodes) != 1 || nodes[0]["node_token"] != "wikA" {
		t.Fatalf("默认只取首页，得到 %v", nodes)
	}
	if !strings.Contains(stderr.String(), "page_token=pt2") || !strings.Contains(stderr.String(), "--page-all") {
		t.Fatalf("has_more 时 stderr 必须提示续翻 page_token，得到 %q", stderr.String())
	}
}

func TestWikiNodes_PageAllAndPageToken(t *testing.T) {
	newWikiTestServer(t, wikiNodesPageHandler)
	var stderr bytes.Buffer
	listWikiNodesCmd.SetErr(&stderr)
	defer func() {
		listWikiNodesCmd.SetErr(nil)
		resetFlags(listWikiNodesCmd, map[string]string{"output": "", "page-all": "false", "page-token": ""})
	}()

	_ = listWikiNodesCmd.Flags().Set("output", "json")
	_ = listWikiNodesCmd.Flags().Set("page-all", "true")
	out, err := captureCmdStdout(t, func() error { return listWikiNodesCmd.RunE(listWikiNodesCmd, []string{"sp-1"}) })
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	var nodes []map[string]any
	_ = json.Unmarshal([]byte(out), &nodes)
	if len(nodes) != 2 {
		t.Fatalf("--page-all 应拉取全部 2 个节点，得到 %d: %s", len(nodes), out)
	}
	if stderr.Len() != 0 {
		t.Fatalf("拉全后不应再提示续翻，得到 %q", stderr.String())
	}

	_ = listWikiNodesCmd.Flags().Set("page-all", "false")
	_ = listWikiNodesCmd.Flags().Set("page-token", "pt2")
	out, err = captureCmdStdout(t, func() error { return listWikiNodesCmd.RunE(listWikiNodesCmd, []string{"sp-1"}) })
	if err != nil {
		t.Fatalf("续翻失败: %v", err)
	}
	nodes = nil
	_ = json.Unmarshal([]byte(out), &nodes)
	if len(nodes) != 1 || nodes[0]["node_token"] != "wikB" {
		t.Fatalf("--page-token 应从第二页开始，得到 %s", out)
	}
}

func TestWikiSpaces_PageAll(t *testing.T) {
	newWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/open-apis/wiki/v2/spaces" {
			return false
		}
		if r.URL.Query().Get("page_token") == "" {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[{"space_id":"1","name":"S1"}],"has_more":true,"page_token":"n2"}}`)
		} else {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[{"space_id":"2","name":"S2"}],"has_more":false}}`)
		}
		return true
	})
	var stderr bytes.Buffer
	listWikiSpacesCmd.SetErr(&stderr)
	defer func() {
		listWikiSpacesCmd.SetErr(nil)
		resetFlags(listWikiSpacesCmd, map[string]string{"output": "", "page-all": "false"})
	}()
	_ = listWikiSpacesCmd.Flags().Set("output", "json")

	out, err := captureCmdStdout(t, func() error { return listWikiSpacesCmd.RunE(listWikiSpacesCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var spaces []map[string]any
	_ = json.Unmarshal([]byte(out), &spaces)
	if len(spaces) != 1 || !strings.Contains(stderr.String(), "page_token=n2") {
		t.Fatalf("默认一页 + stderr 提示，得到 %s / %q", out, stderr.String())
	}

	stderr.Reset()
	_ = listWikiSpacesCmd.Flags().Set("page-all", "true")
	out, err = captureCmdStdout(t, func() error { return listWikiSpacesCmd.RunE(listWikiSpacesCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	spaces = nil
	_ = json.Unmarshal([]byte(out), &spaces)
	if len(spaces) != 2 {
		t.Fatalf("--page-all 应得到 2 个空间，得到 %s", out)
	}
}

// ---------------- wiki move 目标父节点空间校验 ----------------

func wikiMoveHandler(moved *bool) func(w http.ResponseWriter, r *http.Request) bool {
	return func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			switch r.URL.Query().Get("token") {
			case "wikSrc":
				_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"node":{"space_id":"sp-src","node_token":"wikSrc","obj_token":"doxSrc","obj_type":"docx"}}}`)
			case "doxParent", "wikParent":
				_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"node":{"space_id":"sp-parent","node_token":"wikParent","obj_token":"doxParent","obj_type":"docx"}}}`)
			default:
				return false
			}
			return true
		case r.URL.Path == "/open-apis/wiki/v2/spaces/my_library":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"space":{"space_id":"sp-parent","name":"我的文档库"}}}`)
			return true
		case r.Method == http.MethodPost && r.URL.Path == "/open-apis/wiki/v2/spaces/sp-src/nodes/wikSrc/move":
			*moved = true
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["target_space_id"] != "sp-parent" || body["target_parent_token"] != "wikParent" {
				http.Error(w, fmt.Sprintf("bad body %v", body), http.StatusBadRequest)
				return true
			}
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"node":{"node_token":"wikSrc"}}}`)
			return true
		}
		return false
	}
}

func TestWikiMove_TargetParentSpaceValidation(t *testing.T) {
	cases := []struct {
		name        string
		targetSpace string
		parent      string
		wantMove    bool
		wantUsage   bool
		errContains string
	}{
		{name: "目标空间与父节点空间不一致必须拒绝", targetSpace: "sp-other", parent: "wikParent", wantUsage: true, errContains: "不一致"},
		{name: "只给父节点（obj_token）时自动推断空间并换算 node_token", targetSpace: "", parent: "doxParent", wantMove: true},
		{name: "目标空间与父节点一致时放行", targetSpace: "sp-parent", parent: "wikParent", wantMove: true},
		{name: "my_library 别名先解析再比较", targetSpace: "my_library", parent: "wikParent", wantMove: true},
		{name: "两者都不传必须报用法错误", targetSpace: "", parent: "", wantUsage: true, errContains: "至少传一个"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			moved := false
			newWikiTestServer(t, wikiMoveHandler(&moved))
			_ = moveWikiNodeCmd.Flags().Set("target-space", tc.targetSpace)
			_ = moveWikiNodeCmd.Flags().Set("target-parent", tc.parent)
			defer resetFlags(moveWikiNodeCmd, map[string]string{"target-space": "", "target-parent": ""})

			_, err := captureCmdStdout(t, func() error { return moveWikiNodeCmd.RunE(moveWikiNodeCmd, []string{"wikSrc"}) })
			if tc.wantMove {
				if err != nil || !moved {
					t.Fatalf("应成功移动: err=%v moved=%v", err, moved)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("期望错误含 %q，得到 %v", tc.errContains, err)
			}
			if tc.wantUsage && !clierr.HasKind(err, clierr.KindUsage) {
				t.Fatalf("应为用法错误（exit 2），得到 %v", err)
			}
			if moved {
				t.Fatal("校验失败时绝不能发起移动请求")
			}
		})
	}
}

// ---------------- wiki delete：--space-id 也必须核对 ----------------

func TestWikiDelete_AlwaysVerifiesNode(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		objType     string
		spaceID     string
		node        string
		wantDelete  string // 期望的 DELETE 路径；空表示不应删除
		errContains string
	}{
		{
			name: "--space-id 与节点实际空间不一致必须拒绝", input: "wikN", objType: "wiki", spaceID: "sp-wrong",
			node:        `{"space_id":"sp-real","node_token":"wikN","obj_token":"doxN","obj_type":"docx","node_type":"origin"}`,
			errContains: "不一致",
		},
		{
			name: "--space-id 一致时放行，并使用 node_token", input: "doxN", objType: "wiki", spaceID: "sp-real",
			node:       `{"space_id":"sp-real","node_token":"wikN","obj_token":"doxN","obj_type":"docx","node_type":"origin"}`,
			wantDelete: "/open-apis/wiki/v2/spaces/sp-real/nodes/wikN",
		},
		{
			name: "--obj-type 与节点实际类型不一致必须拒绝", input: "wikN", objType: "sheet",
			node:        `{"space_id":"sp-real","node_token":"wikN","obj_token":"doxN","obj_type":"docx","node_type":"origin"}`,
			errContains: "不一致",
		},
		{
			name: "快捷方式节点按文档类型删除必须拒绝", input: "wikS", objType: "docx",
			node:        `{"space_id":"sp-real","node_token":"wikS","obj_token":"doxOrigin","obj_type":"docx","node_type":"shortcut"}`,
			errContains: "快捷方式",
		},
		{
			name: "非 wiki 类型用 obj_token 删除", input: "wikN", objType: "docx",
			node:       `{"space_id":"sp-real","node_token":"wikN","obj_token":"doxN","obj_type":"docx","node_type":"origin"}`,
			wantDelete: "/open-apis/wiki/v2/spaces/sp-real/nodes/doxN",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var deletedPath string
			newWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
				switch {
				case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
					_, _ = fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"node":%s}}`, tc.node)
					return true
				case r.Method == http.MethodDelete:
					deletedPath = r.URL.Path
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task_id":""}}`)
					return true
				}
				return false
			})
			_ = deleteWikiNodeCmd.Flags().Set("obj-type", tc.objType)
			_ = deleteWikiNodeCmd.Flags().Set("space-id", tc.spaceID)
			_ = deleteWikiNodeCmd.Flags().Set("force", "true")
			defer resetFlags(deleteWikiNodeCmd, map[string]string{"obj-type": "", "space-id": "", "force": "false"})

			_, err := captureCmdStdout(t, func() error { return deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{tc.input}) })
			if tc.wantDelete != "" {
				if err != nil || deletedPath != tc.wantDelete {
					t.Fatalf("期望删除 %s，得到 path=%q err=%v", tc.wantDelete, deletedPath, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("期望错误含 %q，得到 %v", tc.errContains, err)
			}
			if !clierr.HasKind(err, clierr.KindUsage) {
				t.Fatalf("校验失败应为用法错误（exit 2）: %v", err)
			}
			if deletedPath != "" {
				t.Fatal("校验失败时绝不能发起 DELETE")
			}
		})
	}
}

// 业务码随 HTTP 400 下发时，按业务码给出的领域提示必须可达。
func TestWikiDelete_BusinessCodeOnHTTP400ReachesHint(t *testing.T) {
	newWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			writeWikiNodeByTokenEcho(w, r, "sp-1")
			return true
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"code":131011,"msg":"approval required"}`)
			return true
		}
		return false
	})
	_ = deleteWikiNodeCmd.Flags().Set("obj-type", "wiki")
	_ = deleteWikiNodeCmd.Flags().Set("force", "true")
	defer resetFlags(deleteWikiNodeCmd, map[string]string{"obj-type": "", "force": "false"})

	_, err := captureCmdStdout(t, func() error { return deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"wikN"}) })
	if err == nil || !strings.Contains(err.Error(), "删除审批") || !client.HasAPICode(err, 131011) {
		t.Fatalf("HTTP 400 + 131011 必须给出审批提示且保留业务码，得到 %v", err)
	}
}

// ---------------- delete-space / node-copy：写操作身份 ----------------

func TestWikiDeleteSpace_ResumeCommandAndIdentity(t *testing.T) {
	newWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/open-apis/wiki/v2/spaces/7001":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task_id":"task-ds-1"}}`)
			return true
		case strings.HasPrefix(r.URL.Path, "/open-apis/wiki/v2/tasks/"):
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task":{"task_id":"task-ds-1","delete_space_result":{"status":"processing"}}}}`)
			return true
		}
		return false
	})
	origAttempts, origInterval := wikiDeleteSpacePollAttempts, wikiDeleteSpacePollInterval
	wikiDeleteSpacePollAttempts, wikiDeleteSpacePollInterval = 2, time.Millisecond
	_ = wikiDeleteSpaceCmd.Flags().Set("yes", "true")
	_ = wikiDeleteSpaceCmd.Flags().Set("output", "json")
	_ = wikiDeleteSpaceCmd.Flags().Set("as", "bot")
	defer func() {
		wikiDeleteSpacePollAttempts, wikiDeleteSpacePollInterval = origAttempts, origInterval
		resetFlags(wikiDeleteSpaceCmd, map[string]string{"yes": "false", "output": "", "as": "auto"})
	}()

	out, err := captureCmdStdout(t, func() error { return wikiDeleteSpaceCmd.RunE(wikiDeleteSpaceCmd, []string{"7001"}) })
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("JSON 解析失败: %v\n%s", err, out)
	}
	want := "feishu-cli drive task-result --scenario wiki_delete_space --task-id 'task-ds-1' --as bot"
	if res["timed_out"] != true || res["resume_command"] != want {
		t.Fatalf("超时时必须输出续查命令 %q，得到 %v", want, res)
	}

	// --as user 但没有任何 User Token：写操作必须报错，不能静默改用 Bot
	_ = wikiDeleteSpaceCmd.Flags().Set("as", "user")
	_, err = captureCmdStdout(t, func() error { return wikiDeleteSpaceCmd.RunE(wikiDeleteSpaceCmd, []string{"7001"}) })
	if err == nil || !strings.Contains(err.Error(), "--as user") {
		t.Fatalf("--as user 缺少 User Token 时必须报错，得到 %v", err)
	}
}

func TestWikiDeleteSpace_AllPollsFailedIsError(t *testing.T) {
	newWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodDelete:
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task_id":"task-ds-2"}}`)
			return true
		case strings.HasPrefix(r.URL.Path, "/open-apis/wiki/v2/tasks/"):
			http.Error(w, "boom", http.StatusBadGateway)
			return true
		}
		return false
	})
	origAttempts, origInterval := wikiDeleteSpacePollAttempts, wikiDeleteSpacePollInterval
	wikiDeleteSpacePollAttempts, wikiDeleteSpacePollInterval = 2, time.Millisecond
	_ = wikiDeleteSpaceCmd.Flags().Set("yes", "true")
	defer func() {
		wikiDeleteSpacePollAttempts, wikiDeleteSpacePollInterval = origAttempts, origInterval
		resetFlags(wikiDeleteSpaceCmd, map[string]string{"yes": "false"})
	}()
	_, err := captureCmdStdout(t, func() error { return wikiDeleteSpaceCmd.RunE(wikiDeleteSpaceCmd, []string{"7001"}) })
	if err == nil || !strings.Contains(err.Error(), "task-ds-2") || !strings.Contains(err.Error(), "--scenario wiki_delete_space") {
		t.Fatalf("状态查询全部失败必须报错并给出续查命令，得到 %v", err)
	}
}

func TestWikiNodeCopy_DryRunSkipsIdentityAndParses400(t *testing.T) {
	ws, _ := newWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/copy") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"code":131006,"msg":"permission denied"}`)
			return true
		}
		return false
	})
	_ = wikiNodeCopyCmd.Flags().Set("space-id", "sp-1")
	_ = wikiNodeCopyCmd.Flags().Set("node-token", "wikN")
	_ = wikiNodeCopyCmd.Flags().Set("target-space-id", "sp-2")
	defer resetFlags(wikiNodeCopyCmd, map[string]string{"space-id": "", "node-token": "", "target-space-id": "", "dry-run": "false", "as": "auto"})

	// 非法 --as 在 dry-run 前即被拒绝
	_ = wikiNodeCopyCmd.Flags().Set("as", "robot")
	_ = wikiNodeCopyCmd.Flags().Set("dry-run", "true")
	if _, err := captureCmdStdout(t, func() error { return wikiNodeCopyCmd.RunE(wikiNodeCopyCmd, nil) }); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("非法 --as 必须是用法错误，得到 %v", err)
	}

	// dry-run 不解析身份、不发请求（--as user 且无 token 也不应报错）
	_ = wikiNodeCopyCmd.Flags().Set("as", "user")
	out, err := captureCmdStdout(t, func() error { return wikiNodeCopyCmd.RunE(wikiNodeCopyCmd, nil) })
	if err != nil || !strings.Contains(out, `"dry_run": true`) {
		t.Fatalf("dry-run 应直接输出预览，得到 err=%v out=%s", err, out)
	}
	if ws.called("POST") {
		t.Fatal("dry-run 绝不能发请求")
	}

	// 实际执行：HTTP 400 + 业务码必须解析为 *APIError
	_ = wikiNodeCopyCmd.Flags().Set("dry-run", "false")
	_ = wikiNodeCopyCmd.Flags().Set("as", "bot")
	_, err = captureCmdStdout(t, func() error { return wikiNodeCopyCmd.RunE(wikiNodeCopyCmd, nil) })
	if err == nil || !client.HasAPICode(err, 131006) {
		t.Fatalf("HTTP 400 + 131006 必须保留业务码，得到 %v", err)
	}
}

// ---------------- wiki member add：通知可选 ----------------

func TestWikiMemberAdd_NeedNotificationFlag(t *testing.T) {
	var gotQuery string
	newWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == "/open-apis/wiki/v2/spaces/sp-1/members" {
			gotQuery = r.URL.RawQuery
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"member":{}}}`)
			return true
		}
		return false
	})
	_ = wikiMemberAddCmd.Flags().Set("member-type", "openid")
	_ = wikiMemberAddCmd.Flags().Set("member-id", "ou_x")
	_ = wikiMemberAddCmd.Flags().Set("role", "member")
	defer resetFlags(wikiMemberAddCmd, map[string]string{"member-type": "", "member-id": "", "role": "", "need-notification": "true"})

	if _, err := captureCmdStdout(t, func() error { return wikiMemberAddCmd.RunE(wikiMemberAddCmd, []string{"sp-1"}) }); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "need_notification=true" {
		t.Fatalf("默认必须保持发送通知（兼容），query=%q", gotQuery)
	}
	_ = wikiMemberAddCmd.Flags().Set("need-notification", "false")
	if _, err := captureCmdStdout(t, func() error { return wikiMemberAddCmd.RunE(wikiMemberAddCmd, []string{"sp-1"}) }); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "need_notification=false" {
		t.Fatalf("--need-notification=false 必须透传，query=%q", gotQuery)
	}
}

// ---------------- wiki move-to-drive：超时给出续查命令 ----------------

func TestWikiMoveToDrive_TimeoutResumeCommand(t *testing.T) {
	newWikiTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/move_wiki_to_docs"):
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task_id":"task-m2d"}}`)
			return true
		}
		return false
	})
	_ = wikiMoveToDriveCmd.Flags().Set("node-token", "wikN")
	_ = wikiMoveToDriveCmd.Flags().Set("wait", "false")
	_ = wikiMoveToDriveCmd.Flags().Set("output", "json")
	defer resetFlags(wikiMoveToDriveCmd, map[string]string{"node-token": "", "wait": "true", "output": ""})

	out, err := captureCmdStdout(t, func() error { return wikiMoveToDriveCmd.RunE(wikiMoveToDriveCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "drive task-result --scenario wiki_move_to_drive --task-id 'task-m2d' --as bot") {
		t.Fatalf("--wait=false 必须给出续查命令，得到 %s", out)
	}
}
