package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

func TestDocMediaInsertFileView(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "a.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4 fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		view string
		want string
	}{
		{"", `{"block_type":23,"file":{"token":""}}`}, // 不传 --file-view：与旧版本完全一致，不下发 view_type
		{"card", `{"block_type":23,"file":{"token":"","view_type":1}}`},
		{"preview", `{"block_type":23,"file":{"token":"","view_type":2}}`},
		{"inline", `{"block_type":23,"file":{"token":"","view_type":3}}`},
	} {
		t.Run("view="+c.view, func(t *testing.T) {
			_, rec := newDocMediaServer(t, "", 0)
			flags := map[string]string{"file": pdf, "type": "file", "output": "json"}
			if c.view != "" {
				flags["file-view"] = c.view
			}
			if _, err := runMediaCmd(t, docMediaInsertCmd, flags, "DocObj"); err != nil {
				t.Fatalf("media-insert 失败: %v", err)
			}
			body := rec.bodies["POST /open-apis/docx/v1/documents/DocObj/blocks/DocObj/children"]
			if !strings.Contains(body, c.want) {
				t.Fatalf("创建块请求体 = %s，期望包含 %s", body, c.want)
			}
		})
	}
}

func TestDocMediaInsertSourceValidation(t *testing.T) {
	_, rec := newDocMediaServer(t, "", 0)
	img := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(img, fakePNG, 0o600); err != nil {
		t.Fatal(err)
	}
	stubClipboard(t, fakePNG, nil)
	cases := []struct {
		flags map[string]string
		want  string
	}{
		{map[string]string{}, "--file 或 --from-clipboard"},
		{map[string]string{"file": img, "from-clipboard": "true"}, "只能指定一个"},
		{map[string]string{"file": img, "file-view": "preview"}, "只用于 --type file"},
		{map[string]string{"file": img, "type": "file", "file-view": "grid"}, "不支持的 --file-view"},
	}
	for _, c := range cases {
		_, err := runMediaCmd(t, docMediaInsertCmd, c.flags, "DocObj")
		if err == nil || !strings.Contains(err.Error(), c.want) || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%v: 期望用法错误含 %q，得到 %v", c.flags, c.want, err)
		}
	}
	if got := rec.list(); len(got) != 0 {
		t.Fatalf("参数校验失败时不应联网: %v", got)
	}
}

func TestDocMediaInsertFromClipboard(t *testing.T) {
	_, rec := newDocMediaServer(t, "", 0)
	stubClipboard(t, fakePNG, nil)
	out, err := runMediaCmd(t, docMediaInsertCmd, map[string]string{"from-clipboard": "true", "width": "8", "output": "json"}, "DocObj")
	if err != nil {
		t.Fatalf("media-insert --from-clipboard 失败: %v", err)
	}
	if len(rec.uploads) != 1 || rec.uploads[0]["file"] != string(fakePNG) || rec.uploads[0]["file_name"] != "clipboard.png" ||
		rec.uploads[0]["parent_node"] != "img1" || rec.uploads[0]["extra"] != `{"drive_route_token":"DocObj"}` {
		t.Fatalf("剪贴板图片上传异常: %v", rec.uploads)
	}
	// 剪贴板图片 16x8：只给 --width 8 时按比例算出高度 4
	if got := rec.bodies["PATCH /open-apis/docx/v1/documents/DocObj/blocks/img1"]; !strings.Contains(got, `"width":8`) || !strings.Contains(got, `"height":4`) {
		t.Fatalf("应按剪贴板图片比例计算尺寸，PATCH = %s", got)
	}
	if !strings.Contains(out, `"file": "clipboard.png"`) {
		t.Fatalf("输出应标明来源 clipboard.png: %s", out)
	}
}

// TestDocMediaInsertInlineRejectedHint 服务端拒绝 view_type=3 时补充可执行的提示，且不会进入上传步骤。
func TestDocMediaInsertInlineRejectedHint(t *testing.T) {
	var uploads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/docx/v1/documents/DocObj/blocks/DocObj" && r.Method == http.MethodGet:
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"block":{"block_id":"DocObj","block_type":1,"children":[]}}}`)
		case strings.HasSuffix(r.URL.Path, "/children"):
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"code":99992402,"msg":"field validation failed","error":{"field_violations":[{"field":"children[*].file.view_type","description":"options: [1,2]","value":"3"}]}}`)
		case strings.Contains(r.URL.Path, "/medias/"):
			uploads++
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"file_token":"x"}}`)
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)
	pdf := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runMediaCmd(t, docMediaInsertCmd, map[string]string{"file": pdf, "type": "file", "file-view": "inline"}, "DocObj")
	if err == nil || !strings.Contains(err.Error(), "改用 --file-view card 或 preview") {
		t.Fatalf("inline 被拒时应提示改用 card/preview，得到 %v", err)
	}
	if uploads != 0 {
		t.Fatalf("创建块失败时不应上传素材")
	}
}
