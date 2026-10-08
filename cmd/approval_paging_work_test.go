package cmd

import (
	"net/http"
	"strings"
	"testing"
)

func sparseApprovalHandler(w http.ResponseWriter, r *http.Request, body string) {
	switch r.URL.Query().Get("page_token") {
	case "":
		_, _ = w.Write([]byte(`{"code":0,"data":{"tasks":[],"has_more":true,"page_token":"p2","count":99}}`))
	case "p2":
		_, _ = w.Write([]byte(`{"code":0,"data":{"tasks":[{"task_id":"t1","title":"A"},{"task_id":"t2","title":"B"}],"has_more":true,"page_token":"p3"}}`))
	default:
		_, _ = w.Write([]byte(`{"code":0,"data":{"tasks":[{"task_id":"t3","title":"C"}],"has_more":false}}`))
	}
}

// TestApprovalTaskQueryEmptyPageWithHasMore 空页 + has_more=true 不能报"没有找到"（回归：稀疏分页误报）
func TestApprovalTaskQueryEmptyPageWithHasMore(t *testing.T) {
	setupWorkCmdTest(t, "u-test", sparseApprovalHandler)
	out, err := runWorkCmd(t, approvalTaskQueryCmd, nil, map[string]string{"topic": "done"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if strings.Contains(out, "没有找到") || !strings.Contains(out, "--page-token p2") {
		t.Fatalf("空页 has_more 应提示继续翻页: %s", out)
	}
	if strings.Contains(out, "总数约") {
		t.Fatalf("不应输出误导性的总数: %s", out)
	}
}

// TestApprovalTaskQueryPageAll --page-all 以 has_more 为准翻完所有页
func TestApprovalTaskQueryPageAll(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", sparseApprovalHandler)
	out, err := runWorkCmd(t, approvalTaskQueryCmd, nil, map[string]string{"topic": "done", "page-all": "true", "output": "json"})
	if err != nil {
		t.Fatalf("query --page-all: %v", err)
	}
	for _, id := range []string{`"t1"`, `"t2"`, `"t3"`, `"pages": 3`, `"has_more": false`} {
		if !strings.Contains(out, id) {
			t.Fatalf("输出缺少 %s: %s", id, out)
		}
	}
	if n := len(rec.apiReqs()); n != 3 {
		t.Fatalf("应请求 3 页，实际 %d", n)
	}
}

// TestApprovalPagingRepeatedTokenStops 服务端重复返回同一游标时停止，避免死循环
func TestApprovalPagingRepeatedTokenStops(t *testing.T) {
	setupWorkCmdTest(t, "", nil)
	calls := 0
	res, err := approvalCollectPages("", 50, func(tok string) ([]int, string, bool, error) {
		calls++
		return []int{calls}, "same", true, nil
	})
	if err != nil || calls != 2 || !res.HasMore || res.PageToken != "same" {
		t.Fatalf("重复游标应在第 2 页后停止: calls=%d res=%+v err=%v", calls, res, err)
	}
}

// TestApprovalWriteDryRunNoNetwork 审批写命令 --dry-run 不联网、不要求 User Token（真实回归只允许 dry-run）
func TestApprovalWriteDryRunNoNetwork(t *testing.T) {
	rec := setupWorkCmdTest(t, "", nil)
	cases := []struct {
		name  string
		run   func() (string, error)
		wants []string
	}{
		{"approve", func() (string, error) {
			return runWorkCmd(t, approvalTaskApproveCmd, nil, map[string]string{"instance-code": "IC", "task-id": "T", "dry-run": "true"})
		}, []string{"/open-apis/approval/v4/tasks/pass", `"task_id": "T"`}},
		{"remind", func() (string, error) {
			return runWorkCmd(t, approvalTaskRemindCmd, nil, map[string]string{"instance-code": "IC", "task-ids": "T1,T2", "dry-run": "true"})
		}, []string{"/open-apis/approval/v4/instances/remind", `"T2"`}},
		{"cancel", func() (string, error) {
			return runWorkCmd(t, approvalInstanceCancelCmd, nil, map[string]string{"instance-code": "IC", "dry-run": "true"})
		}, []string{"/open-apis/approval/v4/instances/recall"}},
	}
	for _, c := range cases {
		out, err := c.run()
		if err != nil {
			t.Fatalf("%s --dry-run: %v", c.name, err)
		}
		for _, w := range c.wants {
			if !strings.Contains(out, w) {
				t.Fatalf("%s 预览缺少 %s: %s", c.name, w, out)
			}
		}
	}
	if len(rec.all()) != 0 {
		t.Fatalf("dry-run 不应联网: %+v", rec.all())
	}
}
