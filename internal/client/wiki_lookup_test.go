package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// writeTenantToken 响应 SDK 的 tenant_access_token 请求，供 Bot 身份用例使用。
func writeTenantToken(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
}

const nodeByTokenOKBody = `{"code":0,"msg":"success","data":{"node":{
	"space_id":"sp-1","node_token":"wikNode123","obj_token":"docObj456","obj_type":"docx",
	"parent_node_token":"wikParent","node_type":"origin","origin_node_token":"wikNode123",
	"origin_space_id":"sp-1","title":"标题","has_child":true,"creator":"ou_c","owner":"ou_o",
	"node_create_time":"1700000000","obj_create_time":"1700000001","obj_edit_time":"1700000002"}}}`

func TestResolveWikiNode_UsesNodeByTokenAndResponseNodeToken(t *testing.T) {
	var gotPath, gotToken, gotObjType, gotAuth string
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			t.Fatalf("显式 User Token 时不应请求 tenant token")
		}
		calls++
		gotPath = r.URL.Path
		gotToken = r.URL.Query().Get("token")
		gotObjType = r.URL.Query().Get("obj_type")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, nodeByTokenOKBody)
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	// 传入的是文档 obj_token：旧实现会把输入回填为 NodeToken（wiki.go:73 的 bug）
	node, err := ResolveWikiNode("docObj456", "u-token")
	if err != nil {
		t.Fatalf("ResolveWikiNode 失败: %v", err)
	}
	if calls != 1 {
		t.Fatalf("请求次数 = %d, 期望 1", calls)
	}
	if gotPath != WikiNodeByTokenPath {
		t.Fatalf("请求路径 = %q, 期望 %q（不能再走 get_node）", gotPath, WikiNodeByTokenPath)
	}
	if gotToken != "docObj456" || gotObjType != "" {
		t.Fatalf("query token=%q obj_type=%q, 期望 token=docObj456 且不带 obj_type", gotToken, gotObjType)
	}
	if gotAuth != "Bearer u-token" {
		t.Fatalf("Authorization = %q, 期望 User Token", gotAuth)
	}
	if node.NodeToken != "wikNode123" {
		t.Fatalf("NodeToken = %q, 期望取响应里的 wikNode123 而不是输入的 obj_token", node.NodeToken)
	}
	want := WikiNode{
		SpaceID: "sp-1", NodeToken: "wikNode123", ObjToken: "docObj456", ObjType: "docx",
		ParentNodeToken: "wikParent", NodeType: "origin", OriginNodeToken: "wikNode123", OriginSpaceID: "sp-1",
		Title: "标题", HasChild: true, Creator: "ou_c", Owner: "ou_o",
		NodeCreateTime: "1700000000", ObjCreateTime: "1700000001", ObjEditTime: "1700000002",
	}
	if *node != want {
		t.Fatalf("节点字段映射不符:\n got=%+v\nwant=%+v", *node, want)
	}
}

func TestResolveWikiNode_BotIdentity(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			writeTenantToken(w)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, nodeByTokenOKBody)
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	if _, err := ResolveWikiNode("wikNode123", ""); err != nil {
		t.Fatalf("ResolveWikiNode(Bot) 失败: %v", err)
	}
	if gotAuth != "Bearer t-test" {
		t.Fatalf("Authorization = %q, 期望 Tenant Token", gotAuth)
	}
}

func TestResolveWikiNode_ErrorClassification(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		code       int
		msg        string
		hint       string
		notFound   bool
		invalid    bool
		notInWiki  bool
		wantNoRetr bool
	}{
		{"131012 节点不存在", http.StatusOK, 131012, "node not found", "已被删除或不存在", true, false, false, true},
		{"131013 token 无效", http.StatusBadRequest, 131013, "token is invalid", "token 无效", false, true, false, true},
		{"131016 token 被截断", http.StatusBadRequest, 131016, "invalid token length", "被截断", false, true, false, true},
		{"131014 不在知识库", http.StatusBadRequest, 131014, "document is not in wiki", "不在知识库中", false, false, true, true},
		{"131006 无权限", http.StatusForbidden, 131006, "forbidden", "无权读取", false, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				// 业务码随 HTTP 4xx 下发时也必须解析出 code，不能只报 HTTP 状态
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `{"code":%d,"msg":%q,"error":{"log_id":"LOG123"}}`, tc.code, tc.msg)
			}))
			defer server.Close()
			setupTestConfig(t, server.URL)

			_, err := ResolveWikiNode("tok", "u-token")
			if err == nil {
				t.Fatal("期望返回错误")
			}
			var lookupErr *WikiLookupError
			if !errors.As(err, &lookupErr) {
				t.Fatalf("错误类型 = %T, 期望 *WikiLookupError: %v", err, err)
			}
			if lookupErr.Code != tc.code || lookupErr.LogID != "LOG123" {
				t.Fatalf("code=%d log_id=%q, 期望 code=%d log_id=LOG123", lookupErr.Code, lookupErr.LogID, tc.code)
			}
			if !HasAPICode(err, tc.code) {
				t.Fatalf("HasAPICode(err, %d) = false; err=%v", tc.code, err)
			}
			if !strings.Contains(err.Error(), tc.hint) {
				t.Fatalf("错误信息缺少中文提示 %q: %v", tc.hint, err)
			}
			if lookupErr.NotFound() != tc.notFound || lookupErr.InvalidToken() != tc.invalid || lookupErr.NotInWiki() != tc.notInWiki {
				t.Fatalf("分类不符: NotFound=%v InvalidToken=%v NotInWiki=%v", lookupErr.NotFound(), lookupErr.InvalidToken(), lookupErr.NotInWiki())
			}
			if tc.wantNoRetr && lookupErr.Retryable() {
				t.Fatalf("code=%d 是终态错误，不应可重试", tc.code)
			}
		})
	}
}

func TestResolveWikiNode_IncompleteOrMissingNode(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"缺少 obj_token", `{"code":0,"data":{"node":{"node_token":"wikA","obj_type":"docx"}}}`, "不完整"},
		{"缺少 node", `{"code":0,"data":{}}`, "未返回节点信息"},
		{"obj_token 输入且缺 node_token", `{"code":0,"data":{"node":{"obj_token":"tok","obj_type":"docx"}}}`, "未返回 node_token"},
		{"非 JSON 的 5xx", "", "HTTP 502"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.body == "" {
					http.Error(w, "bad gateway", http.StatusBadGateway)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			setupTestConfig(t, server.URL)

			_, err := ResolveWikiNode("tok", "u-token")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望包含 %q 的错误，得到 %v", tc.want, err)
			}
		})
	}
}

func TestResolveWikiNode_EmptyToken(t *testing.T) {
	if _, err := ResolveWikiNode("  ", "u-token"); err == nil {
		t.Fatal("空 token 应在本地报错")
	}
}

// TestGetWikiNodeWrappers_DelegateToNodeByToken 保证保留的兼容包装也走 node_by_token，
// 且 GetWikiNodeWithOptions 不再发送 obj_type。
func TestGetWikiNodeWrappers_DelegateToNodeByToken(t *testing.T) {
	var paths []string
	var objTypes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		objTypes = append(objTypes, r.URL.Query().Get("obj_type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, nodeByTokenOKBody)
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	if _, err := GetWikiNode("wikNode123", "u-token"); err != nil {
		t.Fatalf("GetWikiNode 失败: %v", err)
	}
	if _, err := GetWikiNodeWithOptions("docObj456", "docx", "u-token"); err != nil {
		t.Fatalf("GetWikiNodeWithOptions 失败: %v", err)
	}
	for i, p := range paths {
		if p != WikiNodeByTokenPath || objTypes[i] != "" {
			t.Fatalf("第 %d 次请求 path=%q obj_type=%q, 期望 node_by_token 且无 obj_type", i, p, objTypes[i])
		}
	}
}
