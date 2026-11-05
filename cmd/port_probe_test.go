package cmd

import (
	"net/http"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
)

// 宿主机端口扫描器的探测（GET /、无鉴权头）不得进入测试 handler，否则请求计数类断言会偶发失败。
func TestStubServerIgnoresPortProbe(t *testing.T) {
	called := false
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) { called = true })
	defer cleanup()
	resp, err := http.Get(config.Get().BaseURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if called || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("探测请求不应进入 handler: called=%v status=%d", called, resp.StatusCode)
	}
}
