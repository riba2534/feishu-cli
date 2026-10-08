package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func resetMailTriageFlags() {
	for _, name := range []string{"folder", "label", "query", "page-token", "output", "max", "page-size"} {
		f := mailTriageCmd.Flags().Lookup(name)
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
	_ = mailTriageCmd.Flags().Set("unread-only", "false")
	_ = mailTriageCmd.Flags().Set("as", "user")
	_ = mailTriageCmd.Flags().Set("mailbox", "me")
	_ = mailTriageCmd.Flags().Set("user-access-token", "u-test")
}

// TestMailTriage_ListResolvesFolderPaginatesAndSummarizes 回归：
//   - 列表模式 --folder inbox 此前原样透传给 folder_id，服务端报 4038；现在解析为 INBOX，自定义名称查表得 ID；
//   - 自动翻页到 --max，batch_get(format=metadata) 补全摘要；JSON 保留旧字段 items。
func TestMailTriage_ListResolvesFolderPaginatesAndSummarizes(t *testing.T) {
	var listQueries []string
	var batchFormats []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		reply := func(data any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "ok", "data": data})
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/folders"):
			reply(map[string]any{"items": []any{map[string]any{"id": "7001", "name": "项目A"}}})
		case strings.HasSuffix(r.URL.Path, "/messages/batch_get"):
			var body struct {
				Format     string   `json:"format"`
				MessageIDs []string `json:"message_ids"`
			}
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			batchFormats = append(batchFormats, body.Format)
			var msgs []any
			for _, id := range body.MessageIDs {
				msgs = append(msgs, map[string]any{"message_id": id, "subject": "S-" + id, "internal_date": "1790757108345",
					"head_from": map[string]any{"mail_address": "a@example.com", "name": "A"}, "label_ids": []any{"IMPORTANT"}})
			}
			reply(map[string]any{"messages": msgs})
		case strings.HasSuffix(r.URL.Path, "/messages"):
			listQueries = append(listQueries, r.URL.RawQuery)
			q := r.URL.Query()
			if q.Get("page_token") == "" {
				var items []any
				for i := 0; i < 20; i++ {
					items = append(items, fmt.Sprintf("m%02d", i))
				}
				reply(map[string]any{"items": items, "has_more": true, "page_token": "p2"})
				return
			}
			reply(map[string]any{"items": []any{"m20", "m21", "m22"}, "has_more": true, "page_token": "p3"})
		default:
			t.Errorf("意外请求 %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)
	resetMailTriageFlags()
	defer resetMailTriageFlags()

	_ = mailTriageCmd.Flags().Set("folder", "inbox")
	_ = mailTriageCmd.Flags().Set("max", "22")
	_ = mailTriageCmd.Flags().Set("output", "json")
	var err error
	out := captureMailStdout(t, func() { err = mailTriageCmd.RunE(mailTriageCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if len(listQueries) != 2 || !strings.Contains(listQueries[0], "folder_id=INBOX") || !strings.Contains(listQueries[0], "page_size=20") ||
		!strings.Contains(listQueries[1], "page_size=2") || !strings.Contains(listQueries[1], "page_token=p2") {
		t.Errorf("翻页请求不符: %v", listQueries)
	}
	if len(batchFormats) == 0 || batchFormats[0] != "metadata" {
		t.Errorf("摘要应走 batch_get format=metadata，得到 %v", batchFormats)
	}
	var got struct {
		Items     []string         `json:"items"`
		Messages  []map[string]any `json:"messages"`
		Count     int              `json:"count"`
		HasMore   bool             `json:"has_more"`
		PageToken string           `json:"page_token"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, out)
	}
	if got.Count != 22 || len(got.Items) != 22 || got.Items[21] != "m21" || !got.HasMore || got.PageToken != "p3" {
		t.Errorf("结果不符: count=%d items=%d has_more=%v token=%q", got.Count, len(got.Items), got.HasMore, got.PageToken)
	}
	if got.Messages[0]["subject"] != "S-m00" || got.Messages[0]["from"] != "A <a@example.com>" || got.Messages[0]["labels"] != "IMPORTANT" {
		t.Errorf("摘要不符: %v", got.Messages[0])
	}

	// 自定义文件夹名称 → ID
	listQueries = nil
	_ = mailTriageCmd.Flags().Set("folder", "项目A")
	_ = mailTriageCmd.Flags().Set("max", "3")
	captureMailStdout(t, func() { err = mailTriageCmd.RunE(mailTriageCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if len(listQueries) == 0 || !strings.Contains(listQueries[0], "folder_id=7001") {
		t.Errorf("自定义文件夹名称应解析为 ID: %v", listQueries)
	}

	// 未知文件夹：用法错误且不发列表请求
	listQueries = nil
	_ = mailTriageCmd.Flags().Set("folder", "nope")
	if err := mailTriageCmd.RunE(mailTriageCmd, nil); err == nil || !strings.Contains(err.Error(), "未找到文件夹") {
		t.Errorf("未知文件夹应报错，得到 %v", err)
	}
	if len(listQueries) != 0 {
		t.Errorf("文件夹解析失败时不应请求列表")
	}
}

// TestMailTriage_SearchPaginatesWithDedupGuard 搜索路径单页 ≤15 自动翻页；服务端重复返回同一 page_token 时停止。
func TestMailTriage_SearchPaginatesWithDedupGuard(t *testing.T) {
	var pageSizes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pageSizes = append(pageSizes, r.URL.Query().Get("page_size"))
		var items []any
		for i := 0; i < 15; i++ {
			id := fmt.Sprintf("s%d-%d", len(pageSizes), i)
			items = append(items, map[string]any{"id": id, "meta_data": map[string]any{
				"message_biz_id": id, "title": "T" + id, "create_time": "2026-09-28T08:01:43Z",
				"from": map[string]any{"mail_address": "b@example.com"}}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "ok", "data": map[string]any{
			"items": items, "has_more": true, "page_token": "same"}})
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)
	resetMailTriageFlags()
	defer resetMailTriageFlags()

	_ = mailTriageCmd.Flags().Set("query", "x")
	_ = mailTriageCmd.Flags().Set("max", "100")
	_ = mailTriageCmd.Flags().Set("output", "json")
	var err error
	out := captureMailStdout(t, func() { err = mailTriageCmd.RunE(mailTriageCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if len(pageSizes) != 2 || pageSizes[0] != "15" {
		t.Errorf("重复 page_token 应在第 2 页后停止，请求 %v", pageSizes)
	}
	var got struct {
		Messages []map[string]any `json:"messages"`
		Count    int              `json:"count"`
	}
	_ = json.Unmarshal([]byte(out), &got)
	if got.Count != 30 || got.Messages[0]["subject"] != "Ts1-0" || got.Messages[0]["from"] != "b@example.com" {
		t.Errorf("搜索摘要不符: count=%d first=%v", got.Count, got.Messages)
	}
}

func TestResolveMailTriageMax(t *testing.T) {
	resetMailTriageFlags()
	defer resetMailTriageFlags()
	if n, err := resolveMailTriageMax(mailTriageCmd); err != nil || n != 20 {
		t.Errorf("默认应为 20，得到 %d %v", n, err)
	}
	_ = mailTriageCmd.Flags().Set("page-size", "35")
	if n, err := resolveMailTriageMax(mailTriageCmd); err != nil || n != 35 {
		t.Errorf("--page-size 应作为 --max 的别名，得到 %d %v", n, err)
	}
	_ = mailTriageCmd.Flags().Set("max", "401")
	if _, err := resolveMailTriageMax(mailTriageCmd); err == nil {
		t.Error("--max 超过 400 应报错")
	}
}
