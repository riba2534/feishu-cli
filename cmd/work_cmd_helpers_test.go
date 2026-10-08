package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// workReq 记录 mock 服务端收到的请求
type workReq struct {
	Method string
	Path   string
	Query  string
	Body   string
	Auth   string
}

type workRecorder struct {
	mu   sync.Mutex
	reqs []workReq
}

func (r *workRecorder) all() []workReq {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]workReq, len(r.reqs))
	copy(out, r.reqs)
	return out
}

// apiReqs 返回业务请求（排除 tenant_access_token 获取）
func (r *workRecorder) apiReqs() []workReq {
	var out []workReq
	for _, q := range r.all() {
		if !strings.Contains(q.Path, "tenant_access_token") {
			out = append(out, q)
		}
	}
	return out
}

// setupWorkCmdTest 隔离 HOME 与配置，启动 mock 服务端；userToken 非空时通过环境变量提供 User Token。
func setupWorkCmdTest(t *testing.T, userToken string, handler func(w http.ResponseWriter, r *http.Request, body string)) *workRecorder {
	t.Helper()
	rec := &workRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, workReq{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Body: string(b), Auth: r.Header.Get("Authorization")})
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","tenant_access_token":"t-bot","expire":7200}`)
			return
		}
		if handler == nil {
			http.Error(w, `{"code":404,"msg":"no handler"}`, http.StatusNotFound)
			return
		}
		handler(w, r, string(b))
	}))
	t.Cleanup(srv.Close)

	home := t.TempDir()
	restore := profile.SetHomeFunc(func() (string, error) { return home, nil })
	t.Cleanup(restore)
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("FEISHU_APP_ID", "cli_test")
	t.Setenv("FEISHU_APP_SECRET", "test_secret")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", userToken)
	t.Setenv("FEISHU_PROFILE", "")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf("app_id: cli_test\napp_secret: test_secret\nbase_url: %q\n", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(cfgPath); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	var errBuf bytes.Buffer
	old := workStderr
	workStderr = &errBuf
	t.Cleanup(func() { workStderr = old })
	return rec
}

// runWorkCmd 设置 flags 后直接调用 RunE，捕获 stdout，结束后把 flags 复位为默认值。
func runWorkCmd(t *testing.T, c *cobra.Command, args []string, flags map[string]string) (string, error) {
	t.Helper()
	t.Cleanup(func() {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		})
	})
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s: %v", k, err)
		}
	}
	var runErr error
	out := captureStdout(t, func() { runErr = c.RunE(c, args) })
	return out, runErr
}
