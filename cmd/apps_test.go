package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// newAppsScopeTestCmd 构造一个带 access-scope-set 同款 flag 的临时命令，供纯逻辑测试。
func newAppsScopeTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "x"}
	c.Flags().String("app-id", "", "")
	c.Flags().String("scope", "", "")
	c.Flags().String("targets", "", "")
	c.Flags().Bool("apply-enabled", false, "")
	c.Flags().String("approver", "", "")
	c.Flags().Bool("require-login", false, "")
	return c
}

func mustSet(t *testing.T, c *cobra.Command, name, val string) {
	t.Helper()
	if err := c.Flags().Set(name, val); err != nil {
		t.Fatalf("set %s=%s: %v", name, val, err)
	}
}

func TestAppsScopeToServerEnum(t *testing.T) {
	want := map[string]string{"public": "All", "tenant": "Tenant", "specific": "Range"}
	for k, v := range want {
		if appsScopeToServerEnum[k] != v {
			t.Errorf("scope %q → %q, want %q", k, appsScopeToServerEnum[k], v)
		}
	}
}

func TestBuildAppsAccessScopeBody(t *testing.T) {
	t.Run("tenant", func(t *testing.T) {
		c := newAppsScopeTestCmd()
		mustSet(t, c, "scope", "tenant")
		body, err := buildAppsAccessScopeBody(c)
		if err != nil {
			t.Fatal(err)
		}
		if body["scope"] != "Tenant" || len(body) != 1 {
			t.Fatalf("tenant body = %#v", body)
		}
	})

	t.Run("public require_login", func(t *testing.T) {
		c := newAppsScopeTestCmd()
		mustSet(t, c, "scope", "public")
		mustSet(t, c, "require-login", "true")
		body, err := buildAppsAccessScopeBody(c)
		if err != nil {
			t.Fatal(err)
		}
		if body["scope"] != "All" || body["require_login"] != true {
			t.Fatalf("public body = %#v", body)
		}
	})

	t.Run("specific splits targets + apply_config", func(t *testing.T) {
		c := newAppsScopeTestCmd()
		mustSet(t, c, "scope", "specific")
		mustSet(t, c, "targets", `[{"type":"user","id":"ou_a"},{"type":"department","id":"od_b"},{"type":"chat","id":"oc_c"}]`)
		mustSet(t, c, "apply-enabled", "true")
		mustSet(t, c, "approver", "ou_appr")
		body, err := buildAppsAccessScopeBody(c)
		if err != nil {
			t.Fatal(err)
		}
		if body["scope"] != "Range" {
			t.Fatalf("scope = %v", body["scope"])
		}
		assertStrSlice(t, body["users"], []string{"ou_a"})
		assertStrSlice(t, body["departments"], []string{"od_b"})
		assertStrSlice(t, body["chats"], []string{"oc_c"})
		ac, ok := body["apply_config"].(map[string]any)
		if !ok || ac["enabled"] != true {
			t.Fatalf("apply_config = %#v", body["apply_config"])
		}
		assertStrSlice(t, ac["approvers"], []string{"ou_appr"})
	})
}

func assertStrSlice(t *testing.T, got any, want []string) {
	t.Helper()
	gs, ok := got.([]string)
	if !ok {
		t.Fatalf("not []string: %#v", got)
	}
	if len(gs) != len(want) {
		t.Fatalf("len %d != %d (%v)", len(gs), len(want), gs)
	}
	for i := range want {
		if gs[i] != want[i] {
			t.Fatalf("idx %d: %q != %q", i, gs[i], want[i])
		}
	}
}

func TestValidateAppsAccessScopeFlags(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(c *cobra.Command)
		wantErr bool
	}{
		{"tenant ok", func(c *cobra.Command) { mustSetT(c, "scope", "tenant") }, false},
		{"tenant+targets rejected", func(c *cobra.Command) {
			mustSetT(c, "scope", "tenant")
			mustSetT(c, "targets", `[{"type":"user","id":"ou_a"}]`)
		}, true},
		{"public needs require-login", func(c *cobra.Command) { mustSetT(c, "scope", "public") }, true},
		{"public with require-login ok", func(c *cobra.Command) {
			mustSetT(c, "scope", "public")
			mustSetT(c, "require-login", "false")
		}, false},
		{"specific needs targets", func(c *cobra.Command) { mustSetT(c, "scope", "specific") }, true},
		{"specific ok", func(c *cobra.Command) {
			mustSetT(c, "scope", "specific")
			mustSetT(c, "targets", `[{"type":"user","id":"ou_a"}]`)
		}, false},
		{"specific approver needs apply-enabled", func(c *cobra.Command) {
			mustSetT(c, "scope", "specific")
			mustSetT(c, "targets", `[{"type":"user","id":"ou_a"}]`)
			mustSetT(c, "approver", "ou_x")
		}, true},
		{"bad scope", func(c *cobra.Command) { mustSetT(c, "scope", "nope") }, true},
		{"bad targets json", func(c *cobra.Command) {
			mustSetT(c, "scope", "specific")
			mustSetT(c, "targets", `not json`)
		}, true},
		{"targets bad type", func(c *cobra.Command) {
			mustSetT(c, "scope", "specific")
			mustSetT(c, "targets", `[{"type":"robot","id":"x"}]`)
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newAppsScopeTestCmd()
			tc.setup(c)
			err := validateAppsAccessScopeFlags(c)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// mustSetT 是 setup 闭包用的 Set（出错直接 panic，测试里可接受）。
func mustSetT(c *cobra.Command, name, val string) {
	if err := c.Flags().Set(name, val); err != nil {
		panic(err)
	}
}

func TestAppsIsUnsafeRelPath(t *testing.T) {
	unsafe := []string{"/etc/passwd", "..", "../x", "a/../b", "a/..", "a\x00b"}
	for _, p := range unsafe {
		if !appsIsUnsafeRelPath(p) {
			t.Errorf("%q should be unsafe", p)
		}
	}
	safe := []string{"index.html", "assets/app.css", "archive.tar..bak", "a/b/c.js"}
	for _, p := range safe {
		if appsIsUnsafeRelPath(p) {
			t.Errorf("%q should be safe", p)
		}
	}
}

func TestAppsIsSensitiveRelPath(t *testing.T) {
	hit := []string{".env", ".env.production", "sub/.npmrc", ".netrc", "a/.git-credentials",
		".aws/credentials", "x/.docker/config.json", ".kube/config"}
	for _, p := range hit {
		if !appsIsSensitiveRelPath(p) {
			t.Errorf("%q should be sensitive", p)
		}
	}
	miss := []string{"index.html", "credentials", "config.json", "config",
		"env", "app.env.js", ".envrc"}
	for _, p := range miss {
		if appsIsSensitiveRelPath(p) {
			t.Errorf("%q should NOT be sensitive", p)
		}
	}
}

func TestAppsIsSensitiveCandidate_RootIsCredDir(t *testing.T) {
	// --path 本身就是 .aws，candidate RelPath 退化成裸 "credentials"，需靠根上下文命中。
	c := appsCandidate{RelPath: "credentials"}
	if !appsIsSensitiveCandidate("/home/u/.aws", c) {
		t.Error("root=.aws + credentials should be sensitive")
	}
	// 普通根目录不应误判 credentials。
	if appsIsSensitiveCandidate("/home/u/site", c) {
		t.Error("root=site + credentials should NOT be sensitive")
	}
}

func TestAppsEnsureIndexHTML(t *testing.T) {
	if err := appsEnsureIndexHTML([]appsCandidate{{RelPath: "index.html"}}); err != nil {
		t.Errorf("should pass: %v", err)
	}
	if err := appsEnsureIndexHTML([]appsCandidate{{RelPath: "main.html"}}); err == nil {
		t.Error("should fail without index.html")
	}
}

func TestAppsWalkAndBuildTarball(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":     "<h1>hi</h1>",
		"assets/app.css": "body{}",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	candidates, err := appsWalkCandidates(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(candidates))
	}

	tarball, err := appsBuildTarball(candidates)
	if err != nil {
		t.Fatal(err)
	}

	// 解开 tar.gz 校验内容一致。
	got := untarGz(t, tarball)
	gotNames := make([]string, 0, len(got))
	for k := range got {
		gotNames = append(gotNames, k)
	}
	sort.Strings(gotNames)
	if len(gotNames) != 2 {
		t.Fatalf("tarball entries = %v", gotNames)
	}
	for rel, content := range files {
		if got[rel] != content {
			t.Errorf("entry %q = %q, want %q", rel, got[rel], content)
		}
	}
}

func untarGz(t *testing.T, data []byte) map[string]string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, tr); err != nil { //nolint:gosec // test fixture, bounded size
			t.Fatal(err)
		}
		out[hdr.Name] = buf.String()
	}
	return out
}

func TestSplitAppsAccessScopeTargets(t *testing.T) {
	targets := []map[string]any{
		{"type": "user", "id": "ou_a"},
		{"type": "user", "id": "ou_b"},
		{"type": "department", "id": "od_c"},
		{"type": "chat", "id": "oc_d"},
		{"type": "chat", "id": "  "}, // 空 id 跳过
		{"type": "robot", "id": "x"}, // 未知 type 跳过
	}
	users, depts, chats := splitAppsAccessScopeTargets(targets)
	assertStrSlice(t, users, []string{"ou_a", "ou_b"})
	assertStrSlice(t, depts, []string{"od_c"})
	assertStrSlice(t, chats, []string{"oc_d"})
}

// ---- html-publish size cap 拦截测试（补齐官方 lark-cli v1.0.47 的 parity） ----
// 两道上限是 html-publish 仅有的防御闸门，且都在 requireUserToken/网络调用之前 return，
// 因此可不接网络直接驱动 RunE 验证拦截路径（调小包级 var 触发）。

// newAppsHTMLPublishTestCmd 复刻 appsHTMLPublishCmd 的 flag 集，用于直接驱动其 RunE。
func newAppsHTMLPublishTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "html-publish", Run: func(*cobra.Command, []string) {}}
	c.Flags().String("app-id", "", "")
	c.Flags().String("path", "", "")
	c.Flags().Bool("allow-sensitive", false, "")
	addAppsWriteFlags(c)
	return c
}

// writeAppsIndexFixture 造一个含 index.html 的目录，返回目录路径。
func writeAppsIndexFixture(t *testing.T, indexContent string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexContent), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// initAppsTestConfig 装配最小 config（app_id/secret），让 RunE 的 config.Validate 通过。
func initAppsTestConfig(t *testing.T) {
	t.Helper()
	viper.Reset()
	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("app_id: a\napp_secret: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(cfgFile); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
}

func TestAppsHTMLPublish_RejectsOversizeRaw(t *testing.T) {
	initAppsTestConfig(t)
	orig := maxAppsRawBytes
	maxAppsRawBytes = 10
	t.Cleanup(func() { maxAppsRawBytes = orig })

	dir := writeAppsIndexFixture(t, strings.Repeat("x", 200)) // 未压缩 200B > 10B 上限
	c := newAppsHTMLPublishTestCmd()
	mustSet(t, c, "app-id", "app_x")
	mustSet(t, c, "path", dir)

	err := appsHTMLPublishCmd.RunE(c, nil)
	if err == nil {
		t.Fatal("超 raw 上限应报错，client 不应被调用")
	}
	if !strings.Contains(err.Error(), "未压缩总大小") {
		t.Fatalf("err 应是 raw cap 拦截，得到: %v", err)
	}
}

func TestAppsHTMLPublish_RejectsOversizeTarball(t *testing.T) {
	initAppsTestConfig(t)
	orig := maxAppsTarballBytes
	maxAppsTarballBytes = 10
	t.Cleanup(func() { maxAppsTarballBytes = orig })

	// 内容很小（不触 raw 上限），但 tar.gz 最小也远 > 10B → 命中 tarball 上限。
	dir := writeAppsIndexFixture(t, "<h1>hi</h1>")
	c := newAppsHTMLPublishTestCmd()
	mustSet(t, c, "app-id", "app_x")
	mustSet(t, c, "path", dir)

	err := appsHTMLPublishCmd.RunE(c, nil)
	if err == nil {
		t.Fatal("超 tarball 上限应报错，client 不应被调用")
	}
	if !strings.Contains(err.Error(), "打包后 tar.gz") {
		t.Fatalf("err 应是 tarball cap 拦截，得到: %v", err)
	}
}

func TestAppsSizeCapDefaults(t *testing.T) {
	if maxAppsRawBytes != 200*1024*1024 {
		t.Errorf("maxAppsRawBytes 默认 = %d, want %d (200MiB)", maxAppsRawBytes, 200*1024*1024)
	}
	if maxAppsTarballBytes != 20*1024*1024 {
		t.Errorf("maxAppsTarballBytes 默认 = %d, want %d (20MiB)", maxAppsTarballBytes, 20*1024*1024)
	}
}

// captureAppsStdout 捕获 fn 执行期间写到 os.Stdout 的内容（dry-run 走 output.Render 打到 stdout）。
func captureAppsStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), err
}

func TestAppsHTMLPublish_RequiresAppID(t *testing.T) {
	initAppsTestConfig(t)
	c := newAppsHTMLPublishTestCmd()
	mustSet(t, c, "path", t.TempDir())
	if err := appsHTMLPublishCmd.RunE(c, nil); err == nil {
		t.Fatal("缺 --app-id 应报错")
	}
}

func TestAppsHTMLPublish_RequiresPath(t *testing.T) {
	initAppsTestConfig(t)
	c := newAppsHTMLPublishTestCmd()
	mustSet(t, c, "app-id", "app_x")
	if err := appsHTMLPublishCmd.RunE(c, nil); err == nil {
		t.Fatal("缺 --path 应报错")
	}
}

// TestAppsHTMLPublish_DryRunPrintsManifest 覆盖 dry-run 渲染分支（appsHTMLPublishDryRun）：
// 不发请求、打印打包清单（endpoint/文件列表/dry_run 标记），且不要求 User Token。
func TestAppsHTMLPublish_DryRunPrintsManifest(t *testing.T) {
	initAppsTestConfig(t)
	dir := writeAppsIndexFixture(t, "<h1>hi</h1>")
	c := newAppsHTMLPublishTestCmd()
	mustSet(t, c, "app-id", "app_x")
	mustSet(t, c, "path", dir)
	mustSet(t, c, "dry-run", "true")

	out, err := captureAppsStdout(t, func() error { return appsHTMLPublishCmd.RunE(c, nil) })
	if err != nil {
		t.Fatalf("dry-run 不应报错（即使无 User Token）: %v", err)
	}
	for _, want := range []string{"upload_and_release_html_code", "index.html", "\"dry_run\": true"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run 预览缺 %q，实际输出:\n%s", want, out)
		}
	}
}

// TestAppsHTMLPublish_SensitiveBlocksInRunE 验证凭证文件门在 RunE 实际流程里生效（非仅纯匹配器）：
// 含 .env 且未加 --allow-sensitive → 非零退出，client 不被调用。
func TestAppsHTMLPublish_SensitiveBlocksInRunE(t *testing.T) {
	initAppsTestConfig(t)
	dir := writeAppsIndexFixture(t, "<h1>hi</h1>")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newAppsHTMLPublishTestCmd()
	mustSet(t, c, "app-id", "app_x")
	mustSet(t, c, "path", dir)

	err := appsHTMLPublishCmd.RunE(c, nil)
	if err == nil {
		t.Fatal("含 .env 未放行应报错")
	}
	if !strings.Contains(err.Error(), "凭证文件") {
		t.Fatalf("err 应是凭证文件拦截，得到: %v", err)
	}

	// 加 --allow-sensitive + --dry-run → 放行并在清单里标注 waived。
	c2 := newAppsHTMLPublishTestCmd()
	mustSet(t, c2, "app-id", "app_x")
	mustSet(t, c2, "path", dir)
	mustSet(t, c2, "allow-sensitive", "true")
	mustSet(t, c2, "dry-run", "true")
	out, err := captureAppsStdout(t, func() error { return appsHTMLPublishCmd.RunE(c2, nil) })
	if err != nil {
		t.Fatalf("--allow-sensitive dry-run 不应报错: %v", err)
	}
	if !strings.Contains(out, "sensitive_waived") {
		t.Errorf("放行后清单应含 sensitive_waived，实际:\n%s", out)
	}
}
