package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// fakePNG 是 16x8 的真实 PNG（尺寸解析与按内容补扩展名都依赖合法文件头）。
var fakePNG = func() []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 16, 8))); err != nil {
		panic(err)
	}
	return buf.Bytes()
}()

// mediaRecorder 记录 httptest 服务端收到的请求（方法 + 路径 + 查询、JSON 体、上传表单）。
type mediaRecorder struct {
	mu      sync.Mutex
	reqs    []string
	bodies  map[string]string
	uploads []map[string]string
}

func (m *mediaRecorder) add(r *http.Request, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		key += "?" + r.URL.RawQuery
	}
	m.reqs = append(m.reqs, key)
	if m.bodies == nil {
		m.bodies = map[string]string{}
	}
	m.bodies[r.Method+" "+r.URL.Path] = body
}

func (m *mediaRecorder) list() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.reqs...)
}

// newDocMediaServer 模拟 docx / drive 素材接口。cover 为空表示文档没有封面；patchCode 非 0 时 PATCH 返回该业务码。
func newDocMediaServer(t *testing.T, cover string, patchCode int) (*httptest.Server, *mediaRecorder) {
	t.Helper()
	rec := &mediaRecorder{}
	blockSeq := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		var body string
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("解析上传表单失败: %v", err)
			}
			f, hdr, _ := r.FormFile("file")
			data, _ := io.ReadAll(f)
			up := map[string]string{"file": string(data), "filename": hdr.Filename}
			for _, k := range []string{"file_name", "parent_type", "parent_node", "size", "extra"} {
				up[k] = r.FormValue(k)
			}
			rec.mu.Lock()
			rec.uploads = append(rec.uploads, up)
			rec.mu.Unlock()
		} else {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
		}
		rec.add(r, body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"node":{"node_token":"WikTok","obj_token":"DocObj","obj_type":"docx","space_id":"1"}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/open-apis/docx/v1/documents/DocObj":
			if cover == "" {
				fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"DocObj","revision_id":3}}}`)
				return
			}
			fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"DocObj","cover":{"token":%q,"offset_ratio_x":0.1}}}}`, cover)
		case r.Method == http.MethodPatch && r.URL.Path == "/open-apis/docx/v1/documents/DocObj":
			if patchCode != 0 {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, `{"code":%d,"msg":"invalid cover","error":{"log_id":"logP"}}`, patchCode)
				return
			}
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{}}`)
		case r.URL.Path == "/open-apis/drive/v1/medias/upload_all":
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"file_token":"boxUploaded"}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/download") && strings.HasPrefix(r.URL.Path, "/open-apis/drive/v1/medias/"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(fakePNG)
		case r.Method == http.MethodGet && r.URL.Path == "/open-apis/drive/v1/medias/boxPrev/preview_download":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(fakePNG)
		case r.Method == http.MethodGet && r.URL.Path == "/open-apis/docx/v1/documents/DocObj/blocks/DocObj":
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"block":{"block_id":"DocObj","block_type":1,"children":["b1","b2"]}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/open-apis/docx/v1/documents/DocObj/blocks/DocObj/children":
			blockSeq++
			if strings.Contains(body, `"block_type":23`) {
				fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"children":[{"block_id":"view%d","block_type":33,"children":["file%d"]}],"document_revision_id":4}}`, blockSeq, blockSeq)
				return
			}
			fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"children":[{"block_id":"img%d","block_type":27}],"document_revision_id":4}}`, blockSeq)
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/open-apis/docx/v1/documents/DocObj/blocks/"):
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document_revision_id":5}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"code":404,"msg":"unexpected %s %s"}`, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	initDocUpdateTestConfig(t, server.URL)
	return server, rec
}

func runMediaCmd(t *testing.T, c *cobra.Command, flags map[string]string, args ...string) (string, error) {
	t.Helper()
	var names []string
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatalf("设置 --%s=%s 失败: %v", k, v, err)
		}
		names = append(names, k)
	}
	defer resetCmdFlag(c, names...)
	return captureCmdStdout(t, func() error { return c.RunE(c, args) })
}

func stubClipboard(t *testing.T, data []byte, err error) *int {
	t.Helper()
	old := readClipboardImage
	t.Cleanup(func() { readClipboardImage = old })
	calls := 0
	readClipboardImage = func() ([]byte, error) {
		calls++
		return data, err
	}
	return &calls
}

func TestDocResourceValidationBeforeNetwork(t *testing.T) {
	_, rec := newDocMediaServer(t, "boxCover", 0)
	dir := t.TempDir()
	img := filepath.Join(dir, "c.png")
	if err := os.WriteFile(img, fakePNG, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		cmd   *cobra.Command
		flags map[string]string
		want  string
	}{
		{"type 非 cover", docResourceDeleteCmd, map[string]string{"type": "icon"}, "只支持 cover"},
		{"update 无来源", docResourceUpdateCmd, nil, "三选一"},
		{"update 多来源", docResourceUpdateCmd, map[string]string{"file": img, "from-clipboard": "true"}, "只能指定一个"},
		{"update http URL", docResourceUpdateCmd, map[string]string{"url": "http://example.com/a.png"}, "https"},
		{"update URL 带 userinfo", docResourceUpdateCmd, map[string]string{"url": "https://u:p@example.com/a.png"}, "userinfo"},
		{"update NaN 偏移", docResourceUpdateCmd, map[string]string{"file": img, "offset-ratio-x": "NaN"}, "有限数值"},
		{"update 文件不存在", docResourceUpdateCmd, map[string]string{"file": filepath.Join(dir, "nope.png")}, "不存在"},
		{"update 敏感目录", docResourceUpdateCmd, map[string]string{"file": filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")}, "敏感目录"},
		{"download 缺 -o", docResourceDownloadCmd, nil, "--output 必填"},
		{"download 输出是目录", docResourceDownloadCmd, map[string]string{"output": dir}, "是目录"},
		{"download 输出已存在", docResourceDownloadCmd, map[string]string{"output": img}, "已存在"},
		{"download 非法格式", docResourceDownloadCmd, map[string]string{"output": filepath.Join(dir, "x"), "output-format": "yaml"}, "只支持 json"},
		{"download 敏感目录", docResourceDownloadCmd, map[string]string{"output": filepath.Join(os.Getenv("HOME"), ".feishu-cli", "cover")}, "敏感目录"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := runMediaCmd(t, c.cmd, c.flags, "DocObj")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("期望错误含 %q，得到 %v", c.want, err)
			}
			if !clierr.HasKind(err, clierr.KindUsage) {
				t.Fatalf("应为用法错误（退出码 2）: %v", err)
			}
		})
	}
	if got := rec.list(); len(got) != 0 {
		t.Fatalf("参数校验失败时不应发出任何请求: %v", got)
	}
	// 非 docx 链接直接拒绝
	if _, err := runMediaCmd(t, docResourceDeleteCmd, nil, "https://example.feishu.cn/sheets/shtXXX"); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("sheet 链接应以用法错误拒绝，得到 %v", err)
	}
}

func TestDocResourceDryRunOffline(t *testing.T) {
	_, rec := newDocMediaServer(t, "boxCover", 0)
	calls := stubClipboard(t, fakePNG, nil)

	out, err := runMediaCmd(t, docResourceUpdateCmd, map[string]string{
		"from-clipboard": "true", "offset-ratio-y": "-0.25", "dry-run": "true",
	}, "https://example.feishu.cn/wiki/WikTok")
	if err != nil {
		t.Fatalf("dry-run 失败: %v", err)
	}
	var plan struct {
		DryRun bool `json:"dry_run"`
		API    []dryRunStep
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("dry-run 输出不是 JSON: %v\n%s", err, out)
	}
	if !plan.DryRun || plan.Source != "clipboard" || len(plan.API) != 3 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.API[0].URL != "/open-apis/wiki/v2/spaces/node_by_token" || plan.API[1].Body["parent_type"] != "docx_image" {
		t.Fatalf("plan 步骤异常: %+v", plan.API)
	}
	cover, _ := plan.API[2].Body["update_cover"].(map[string]any)["cover"].(map[string]any)
	if len(cover) != 2 || cover["token"] != "<file_token>" || cover["offset_ratio_y"] != -0.25 {
		t.Fatalf("PATCH 预览 = %v（未设置的 offset_ratio_x 不应出现）", plan.API[2].Body)
	}

	out, err = runMediaCmd(t, docResourceDeleteCmd, map[string]string{"dry-run": "true"}, "DocObj")
	if err != nil || !strings.Contains(out, `"cover": null`) {
		t.Fatalf("delete dry-run 应展示 cover:null，得到 %v\n%s", err, out)
	}
	out, err = runMediaCmd(t, docResourceDownloadCmd, map[string]string{"dry-run": "true", "output": filepath.Join(t.TempDir(), "cover")}, "DocObj")
	if err != nil || !strings.Contains(out, "/open-apis/drive/v1/medias/<cover.token>/download") {
		t.Fatalf("download dry-run 输出异常: %v\n%s", err, out)
	}
	if got := rec.list(); len(got) != 0 {
		t.Fatalf("dry-run 不应联网: %v", got)
	}
	if *calls != 0 {
		t.Fatalf("dry-run 不应读取剪贴板，读取了 %d 次", *calls)
	}
}

func TestDocResourceUpdateFromFile(t *testing.T) {
	_, rec := newDocMediaServer(t, "", 0)
	img := filepath.Join(t.TempDir(), "banner.png")
	if err := os.WriteFile(img, fakePNG, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runMediaCmd(t, docResourceUpdateCmd, map[string]string{
		"file": img, "offset-ratio-x": "0", "output": "json",
	}, "https://example.feishu.cn/wiki/WikTok")
	if err != nil {
		t.Fatalf("update 失败: %v", err)
	}
	if len(rec.uploads) != 1 {
		t.Fatalf("应上传 1 次: %v", rec.list())
	}
	up := rec.uploads[0]
	if up["parent_type"] != "docx_image" || up["parent_node"] != "DocObj" || up["file_name"] != "banner.png" ||
		up["extra"] != `{"drive_route_token":"DocObj"}` || up["file"] != string(fakePNG) {
		t.Fatalf("上传表单异常（wiki 必须解包为 docx token）: %v", up)
	}
	if got := rec.bodies["PATCH /open-apis/docx/v1/documents/DocObj"]; got != `{"update_cover":{"cover":{"token":"boxUploaded","offset_ratio_x":0}}}` {
		t.Fatalf("PATCH 请求体 = %s", got)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil || res["file_token"] != "boxUploaded" || res["source"] != "file" || res["updated"] != true {
		t.Fatalf("JSON 输出异常: %v %s", err, out)
	}
}

func TestDocResourceUpdateFromClipboard(t *testing.T) {
	_, rec := newDocMediaServer(t, "", 0)
	calls := stubClipboard(t, fakePNG, nil)
	if _, err := runMediaCmd(t, docResourceUpdateCmd, map[string]string{"from-clipboard": "true"}, "DocObj"); err != nil {
		t.Fatalf("update --from-clipboard 失败: %v", err)
	}
	if *calls != 1 || len(rec.uploads) != 1 || rec.uploads[0]["file"] != string(fakePNG) || rec.uploads[0]["file_name"] != "clipboard.png" {
		t.Fatalf("剪贴板内容应原样上传为 clipboard.png: calls=%d uploads=%v", *calls, rec.uploads)
	}

	// 剪贴板读取失败：用法错误，且不发出任何请求
	_, rec2 := newDocMediaServer(t, "", 0)
	stubClipboard(t, nil, clierr.Usage(errors.New("剪贴板中没有图片数据")))
	_, err := runMediaCmd(t, docResourceUpdateCmd, map[string]string{"from-clipboard": "true"}, "DocObj")
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("剪贴板无图应为用法错误，得到 %v", err)
	}
	if got := rec2.list(); len(got) != 0 {
		t.Fatalf("剪贴板失败时不应联网: %v", got)
	}
}

func TestDocResourceUpdatePatchFailureKeepsFileToken(t *testing.T) {
	newDocMediaServer(t, "", 1770001)
	img := filepath.Join(t.TempDir(), "c.png")
	if err := os.WriteFile(img, fakePNG, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runMediaCmd(t, docResourceUpdateCmd, map[string]string{"file": img}, "DocObj")
	if err == nil || !strings.Contains(err.Error(), "file_token=boxUploaded") || !strings.Contains(err.Error(), "code=1770001") {
		t.Fatalf("PATCH 失败应透出业务码并给出可复用的 file_token，得到 %v", err)
	}
}

func TestDocResourceDelete(t *testing.T) {
	_, rec := newDocMediaServer(t, "boxCover", 0)
	out, err := runMediaCmd(t, docResourceDeleteCmd, map[string]string{"output": "json"}, "DocObj")
	if err != nil {
		t.Fatalf("delete 失败: %v", err)
	}
	if got := rec.bodies["PATCH /open-apis/docx/v1/documents/DocObj"]; got != `{"update_cover":{"cover":null}}` {
		t.Fatalf("删除封面应发送 cover:null，得到 %q", got)
	}
	var res map[string]any
	_ = json.Unmarshal([]byte(out), &res)
	if res["deleted"] != true || res["already_empty"] != false {
		t.Fatalf("输出 = %s", out)
	}

	// 没有封面：幂等成功，不发 PATCH
	_, rec2 := newDocMediaServer(t, "", 0)
	out, err = runMediaCmd(t, docResourceDeleteCmd, map[string]string{"output": "json"}, "DocObj")
	if err != nil {
		t.Fatalf("空封面 delete 应成功: %v", err)
	}
	for _, r := range rec2.list() {
		if strings.HasPrefix(r, "PATCH") {
			t.Fatalf("空封面不应发 PATCH: %v", rec2.list())
		}
	}
	res = nil
	_ = json.Unmarshal([]byte(out), &res)
	if res["deleted"] != false || res["already_empty"] != true {
		t.Fatalf("输出 = %s", out)
	}
}

func TestDocResourceDownload(t *testing.T) {
	_, rec := newDocMediaServer(t, "boxCover", 0)
	dir := t.TempDir()
	out := filepath.Join(dir, "cover")
	stdout, err := runMediaCmd(t, docResourceDownloadCmd, map[string]string{"output": out, "output-format": "json"}, "DocObj")
	if err != nil {
		t.Fatalf("download 失败: %v", err)
	}
	data, err := os.ReadFile(out + ".png")
	if err != nil || !bytes.Equal(data, fakePNG) {
		t.Fatalf("应按内容补 .png 保存: %v", err)
	}
	if !strings.Contains(strings.Join(rec.list(), "\n"), "GET /open-apis/drive/v1/medias/boxCover/download") {
		t.Fatalf("应按 cover.token 下载: %v", rec.list())
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || res["saved_path"] != out+".png" || res["type"] != "cover" {
		t.Fatalf("JSON 输出异常: %v %s", err, stdout)
	}
	// 再次下载：最终文件已存在 → 拒绝
	if _, err := runMediaCmd(t, docResourceDownloadCmd, map[string]string{"output": out}, "DocObj"); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("目标已存在应拒绝，得到 %v", err)
	}
	if _, err := runMediaCmd(t, docResourceDownloadCmd, map[string]string{"output": out, "overwrite": "true"}, "DocObj"); err != nil {
		t.Fatalf("--overwrite 应放行: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") || strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("不应残留临时文件: %s", e.Name())
		}
	}

	// 没有封面：用法错误且不创建文件
	newDocMediaServer(t, "", 0)
	empty := filepath.Join(dir, "none")
	if _, err := runMediaCmd(t, docResourceDownloadCmd, map[string]string{"output": empty}, "DocObj"); err == nil || !strings.Contains(err.Error(), "没有封面") || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("无封面应以用法错误失败，得到 %v", err)
	}
	if matches, _ := filepath.Glob(empty + "*"); len(matches) != 0 {
		t.Fatalf("无封面时不应创建文件: %v", matches)
	}
}

func TestDocMediaPreview(t *testing.T) {
	_, rec := newDocMediaServer(t, "", 0)
	dir := t.TempDir()
	out := filepath.Join(dir, "asset")
	if _, err := runMediaCmd(t, docMediaPreviewCmd, map[string]string{"output": out}, "boxPrev"); err != nil {
		t.Fatalf("media-preview 失败: %v", err)
	}
	if data, err := os.ReadFile(out + ".png"); err != nil || !bytes.Equal(data, fakePNG) {
		t.Fatalf("应按内容补 .png（响应头是 octet-stream）: %v", err)
	}
	if got := rec.list(); len(got) != 1 || got[0] != "GET /open-apis/drive/v1/medias/boxPrev/preview_download?preview_type=16" {
		t.Fatalf("请求 = %v", got)
	}
	if _, err := runMediaCmd(t, docMediaPreviewCmd, map[string]string{"output": out}, "../etc"); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("非法 token 应为用法错误，得到 %v", err)
	}
}
