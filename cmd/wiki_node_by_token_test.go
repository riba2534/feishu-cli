package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// newWikiNodeByTokenServer 模拟 node_by_token：输入 obj_token 时返回真实 node_token，
// 并记录所有请求路径，便于断言后续节点级操作用的是响应里的 node_token。
func newWikiNodeByTokenServer(t *testing.T, extra func(w http.ResponseWriter, r *http.Request) bool) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/get_node":
			t.Errorf("不应再调用 get_node（对 obj_token 一律 131005）")
			_, _ = fmt.Fprint(w, `{"code":131005,"msg":"not found"}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			if r.URL.Query().Has("obj_type") {
				t.Errorf("node_by_token 不应携带 obj_type: %s", r.URL.RawQuery)
			}
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"node":{
				"space_id":"sp-1","node_token":"wikRealNode","obj_token":"docObjToken","obj_type":"docx","title":"T"}}}`)
		default:
			if extra != nil && extra(w, r) {
				return
			}
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server, &paths
}

func TestWikiMove_ObjTokenUsesResolvedNodeToken(t *testing.T) {
	var movePath string
	server, _ := newWikiNodeByTokenServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/move") {
			movePath = r.URL.Path
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"node":{"node_token":"wikRealNode","space_id":"sp-2"}}}`)
			return true
		}
		return false
	})
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = moveWikiNodeCmd.Flags().Set("target-space", "sp-2")
	defer resetCmdFlag(moveWikiNodeCmd, "target-space")

	if _, err := captureCmdStdout(t, func() error { return moveWikiNodeCmd.RunE(moveWikiNodeCmd, []string{"docObjToken"}) }); err != nil {
		t.Fatalf("wiki move 失败: %v", err)
	}
	if movePath != "/open-apis/wiki/v2/spaces/sp-1/nodes/wikRealNode/move" {
		t.Fatalf("移动接口路径 = %q，期望使用 node_by_token 返回的 node_token（而非输入的 obj_token）", movePath)
	}
}

func TestWikiUpdate_ObjTokenUsesResolvedNodeToken(t *testing.T) {
	var updatePath string
	server, _ := newWikiNodeByTokenServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/update_title") {
			updatePath = r.URL.Path
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{}}`)
			return true
		}
		return false
	})
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = updateWikiNodeCmd.Flags().Set("title", "新标题")
	defer resetCmdFlag(updateWikiNodeCmd, "title")

	out, err := captureCmdStdout(t, func() error { return updateWikiNodeCmd.RunE(updateWikiNodeCmd, []string{"docObjToken"}) })
	if err != nil {
		t.Fatalf("wiki update 失败: %v", err)
	}
	if updatePath != "/open-apis/wiki/v2/spaces/sp-1/nodes/wikRealNode/update_title" {
		t.Fatalf("更新标题接口路径 = %q，期望使用 node_by_token 返回的 node_token", updatePath)
	}
	if !strings.Contains(out, "wikRealNode") {
		t.Fatalf("输出应展示真实 node_token: %s", out)
	}
}

func TestWikiGet_ObjTokenResolvesThroughNodeByToken(t *testing.T) {
	server, paths := newWikiNodeByTokenServer(t, nil)
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = getWikiNodeCmd.Flags().Set("output", "json")
	defer resetCmdFlag(getWikiNodeCmd, "output")

	out, err := captureCmdStdout(t, func() error { return getWikiNodeCmd.RunE(getWikiNodeCmd, []string{"docObjToken"}) })
	if err != nil {
		t.Fatalf("wiki get <obj_token> 失败: %v", err)
	}
	if !strings.Contains(out, `"node_token": "wikRealNode"`) || !strings.Contains(out, `"obj_token": "docObjToken"`) {
		t.Fatalf("输出应包含响应中的 node_token 与 obj_token: %s", out)
	}
	found := false
	for _, p := range *paths {
		if strings.HasPrefix(p, "GET /open-apis/wiki/v2/spaces/node_by_token?token=docObjToken") {
			found = true
		}
	}
	if !found {
		t.Fatalf("未通过 node_by_token 解析: %v", *paths)
	}
}
