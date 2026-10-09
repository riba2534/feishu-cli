package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

func TestMediaUploadDocID(t *testing.T) {
	img := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(img, fakePNG, 0o600); err != nil {
		t.Fatal(err)
	}
	_, rec := newDocMediaServer(t, "", 0)
	if _, err := runMediaCmd(t, uploadMediaCmd, map[string]string{"parent-node": "blk1", "doc-id": "https://example.feishu.cn/wiki/WikTok"}, img); err != nil {
		t.Fatalf("media upload --doc-id 失败: %v", err)
	}
	if len(rec.uploads) != 1 || rec.uploads[0]["extra"] != `{"drive_route_token":"DocObj"}` || rec.uploads[0]["parent_node"] != "blk1" {
		t.Fatalf("--doc-id 应写入 extra.drive_route_token（wiki 解包后）: %v", rec.uploads)
	}

	// 不传 --doc-id：与旧版本一致，不携带 extra
	_, rec2 := newDocMediaServer(t, "", 0)
	if _, err := runMediaCmd(t, uploadMediaCmd, map[string]string{"parent-node": "blk1"}, img); err != nil {
		t.Fatalf("media upload 失败: %v", err)
	}
	if len(rec2.uploads) != 1 || rec2.uploads[0]["extra"] != "" {
		t.Fatalf("未传 --doc-id 时不应带 extra: %v", rec2.uploads)
	}

	if _, err := runMediaCmd(t, uploadMediaCmd, map[string]string{"parent-node": "blk1", "doc-id": "a/b"}, img); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("非法 --doc-id 应为用法错误，得到 %v", err)
	}
}
