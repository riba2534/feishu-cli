package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/apidiag"
)

// TestRenderErrorDiagnosticsFromRegistry 回归：`doc get <非法 id>` 过去只输出
// "获取文档失败: code=99992402, msg=field validation failed"，丢失字段校验与 log_id。
func TestRenderErrorDiagnosticsFromRegistry(t *testing.T) {
	apidiag.Reset()
	defer apidiag.Reset()
	info, ok := apidiag.Parse(400, nil, []byte(`{"code":99992402,"msg":"field validation failed","error":{"log_id":"2026LOGID","field_violations":[{"field":"document_id","description":"the min len is 27"}]}}`))
	if !ok {
		t.Fatal("测试数据应能解析")
	}
	apidiag.Record(info)

	got := strings.Join(renderErrorDiagnostics(errors.New("获取文档失败: code=99992402, msg=field validation failed")), "\n")
	for _, want := range []string{"字段校验: document_id: the min len is 27", "log_id: 2026LOGID"} {
		if !strings.Contains(got, want) {
			t.Errorf("诊断应包含 %q，实际:\n%s", want, got)
		}
	}
	if lines := renderErrorDiagnostics(errors.New("获取文档失败: code=1770002, msg=not found")); len(lines) != 0 {
		t.Errorf("业务码不匹配时不应附加其他请求的诊断: %v", lines)
	}
	if lines := renderErrorDiagnostics(errors.New("读取文件失败: no such file")); len(lines) != 0 {
		t.Errorf("非 API 错误不应附加诊断: %v", lines)
	}
}

// TestRunAPI_BizErrorKeepsStdoutClean 回归：api 业务错误过去把错误体写 stdout 且 --jq 仍处理，
// 管道下游会把错误 JSON 当成功结果。现在 stdout 为空（--raw 除外），错误与诊断走 stderr。
func TestRunAPI_BizErrorKeepsStdoutClean(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()
	const errBody = `{"code":99991672,"msg":"Access denied.","error":{"log_id":"LOG1","permission_violations":[{"subject":"okr:okr:readonly"}]}}`
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-fake","expire":7200}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, errBody)
	})
	defer cleanup()

	cases := []struct {
		name       string
		setup      func()
		wantStdout string
	}{
		{name: "默认 pretty", setup: func() {}},
		{name: "--jq", setup: func() { apiJQ = ".code" }},
		{name: "--format table", setup: func() { apiFormat = "table" }},
		{name: "--raw 保留原样输出", setup: func() { apiRaw = true }, wantStdout: errBody},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetAPIFlags()
			cmd := newTestAPICmd()
			apiAs = "bot"
			tc.setup()
			var runErr error
			stdout := captureStdout(t, func() {
				runErr = cmd.RunE(cmd, []string{"GET", "/open-apis/okr/v1/periods"})
			})
			if runErr == nil {
				t.Fatal("业务错误应返回非 nil")
			}
			if got := exitCodeFor(runErr); got != 3 {
				t.Errorf("99991672 退出码 = %d, want 3", got)
			}
			if strings.TrimSpace(stdout) != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
			rendered := strings.Join(renderErrorDiagnostics(runErr), "\n")
			if !strings.Contains(rendered, "okr:okr:readonly") || !strings.Contains(rendered, "log_id: LOG1") {
				t.Errorf("stderr 诊断应含缺失 scope 与 log_id，实际:\n%s", rendered)
			}
		})
	}
}
