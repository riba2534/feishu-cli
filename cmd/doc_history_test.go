package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func resetHistoryFlags() {
	for _, c := range []*pflag.FlagSet{docHistoryListCmd.Flags(), docHistoryRevertCmd.Flags(), docHistoryRevertStatusCmd.Flags()} {
		c.VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
	}
}

func TestDocHistoryListPageAll(t *testing.T) {
	defer resetHistoryFlags()
	var tokens []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		if r.URL.Path != "/open-apis/docs_ai/v1/documents/docH/histories" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		tok := r.URL.Query().Get("page_token")
		tokens = append(tokens, tok+"|"+r.URL.Query().Get("page_size"))
		if tok == "" {
			fmt.Fprint(w, `{"code":0,"msg":"","data":{"entries":[{"history_version_id":"2","revision_id":5}],"has_more":true,"page_token":"p2"}}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"msg":"","data":{"entries":[{"history_version_id":"1","revision_id":3}],"has_more":false}}`)
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	_ = docHistoryListCmd.Flags().Set("page-all", "true")
	_ = docHistoryListCmd.Flags().Set("page-size", "1")
	_ = docHistoryListCmd.Flags().Set("output", "json")
	var err error
	out := captureStdout(t, func() { err = docHistoryListCmd.RunE(docHistoryListCmd, []string{"docH"}) })
	if err != nil {
		t.Fatalf("history list 失败: %v", err)
	}
	if strings.Join(tokens, ",") != "|1,p2|1" || !strings.Contains(out, `"history_version_id": "1"`) || !strings.Contains(out, `"has_more": false`) {
		t.Fatalf("翻页异常: tokens=%v out=%s", tokens, out)
	}
}

func TestDocHistoryValidation(t *testing.T) {
	defer resetHistoryFlags()
	initDocUpdateTestConfig(t, "http://127.0.0.1:59997")
	_ = docHistoryListCmd.Flags().Set("page-size", "21")
	if err := docHistoryListCmd.RunE(docHistoryListCmd, []string{"docH"}); err == nil || !strings.Contains(err.Error(), "1-20") {
		t.Fatalf("page-size 越界应报错: %v", err)
	}
	for _, v := range []string{"0", "abc", "-1"} {
		_ = docHistoryRevertCmd.Flags().Set("history-version-id", v)
		if err := docHistoryRevertCmd.RunE(docHistoryRevertCmd, []string{"docH"}); err == nil || !strings.Contains(err.Error(), "正整数") {
			t.Fatalf("history-version-id=%s 应报错: %v", v, err)
		}
	}
	_ = docHistoryRevertCmd.Flags().Set("history-version-id", "7")
	_ = docHistoryRevertCmd.Flags().Set("wait-timeout-ms", "30001")
	if err := docHistoryRevertCmd.RunE(docHistoryRevertCmd, []string{"docH"}); err == nil || !strings.Contains(err.Error(), "0-30000") {
		t.Fatalf("wait-timeout-ms 越界应报错: %v", err)
	}
}

func TestDocHistoryRevertStatusFailureNonZero(t *testing.T) {
	defer resetHistoryFlags()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		if r.URL.Query().Get("task_id") != "task1" {
			http.Error(w, "missing task_id", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"code":0,"msg":"","data":{"status":"partial_failed","history_version_id":"7","failed_block_tokens":["blkX"]}}`)
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)
	_ = docHistoryRevertStatusCmd.Flags().Set("task-id", "task1")
	var err error
	captureStdout(t, func() { err = docHistoryRevertStatusCmd.RunE(docHistoryRevertStatusCmd, []string{"docH"}) })
	if err == nil || !strings.Contains(err.Error(), "partial_failed") || !strings.Contains(err.Error(), "blkX") {
		t.Fatalf("partial_failed 应非零退出并给出 failed_block_tokens: %v", err)
	}
}
