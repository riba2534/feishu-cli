package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUniqueBatchFilename(t *testing.T) {
	used := map[string]struct{}{}
	got := []string{}
	for _, name := range []string{"meeting.mp4", "meeting.mp4", "meeting.mp4", "audio", "audio", "meeting-2.mp4"} {
		n := uniqueBatchFilename(name, used)
		used[n] = struct{}{}
		got = append(got, n)
	}
	want := []string{"meeting.mp4", "meeting-2.mp4", "meeting-3.mp4", "audio", "audio-2", "meeting-2-2.mp4"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// startMinutesMediaStub 假妙记媒体服务：media 接口返回指向 media.example.test 的预签名 URL，
// 下载响应带相同的 Content-Disposition 文件名（模拟多条妙记同名）。
// 预签名下载拒绝 localhost/内网 IP，测试把 http.DefaultTransport 对该域名的拨号指到桩服务器。
func startMinutesMediaStub(t *testing.T, tokens []string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, tk := range tokens {
			switch r.URL.Path {
			case "/open-apis/minutes/v1/minutes/" + tk + "/media":
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"code":0,"data":{"download_url":"http://media.example.test/files/%s"}}`, tk)
				return
			case "/files/" + tk:
				w.Header().Set("Content-Type", "video/mp4")
				w.Header().Set("Content-Disposition", `attachment; filename="meeting.mp4"`)
				_, _ = fmt.Fprint(w, "content-of-"+tk)
				return
			}
		}
		http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().String()
	orig := http.DefaultTransport
	tr := orig.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.DialContext = func(ctx context.Context, network, target string) (net.Conn, error) {
		// 只把预签名下载域名指到桩服务器；其他请求（含可能被缓存的 API client）照常拨号
		if strings.HasPrefix(target, "media.example.test:") {
			target = addr
		}
		return (&net.Dialer{}).DialContext(ctx, network, target)
	}
	http.DefaultTransport = tr
	t.Cleanup(func() { http.DefaultTransport = orig })
	return writeStubConfig(t, srv.URL)
}

// TestMinutesDownloadBatchSameNameNoOverwrite 同一批次的同名文件自动改名，
// 不加 --overwrite 时后一个不再报"已存在"，加了也不覆盖本批次已写出的文件。
func TestMinutesDownloadBatchSameNameNoOverwrite(t *testing.T) {
	tokens := []string{"obcnaaaaaa", "obcnbbbbbb", "obcncccccc"}
	for _, overwrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("overwrite=%v", overwrite), func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
			cfg := startMinutesMediaStub(t, tokens)
			dir := t.TempDir()
			if overwrite {
				// 下载前就存在的同名文件：--overwrite 时允许被本批次第一个文件覆盖
				if err := os.WriteFile(filepath.Join(dir, "meeting.mp4"), []byte("stale"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"minutes", "download", "--minute-tokens", strings.Join(tokens, ","), "--output", dir, "--config", cfg}
			if overwrite {
				args = append(args, "--overwrite")
			}
			stdout, stderr, err := runCLI(t, args...)
			if err != nil {
				t.Fatalf("批量下载失败: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
			}
			if strings.Contains(stdout, "FAIL") {
				t.Fatalf("同名文件不应报已存在:\n%s", stdout)
			}
			want := map[string]string{
				"meeting.mp4":   "content-of-obcnaaaaaa",
				"meeting-2.mp4": "content-of-obcnbbbbbb",
				"meeting-3.mp4": "content-of-obcncccccc",
			}
			for name, content := range want {
				b, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || string(b) != content {
					t.Fatalf("%s = %q, %v; want %q\nstdout=%s", name, b, err, content, stdout)
				}
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != len(want) {
				t.Fatalf("目录中应恰好 %d 个文件，实际 %d", len(want), len(entries))
			}
		})
	}
}
