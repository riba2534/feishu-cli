package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/client"
)

func TestResolveResourceDownloadName(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		explicit bool
		meta     *client.ResourceMeta
		want     string
	}{
		{"默认用服务端文件名", "file_xxx", false, &client.ResourceMeta{FileName: "周报.pdf"}, "周报.pdf"},
		{"默认无文件名按 MIME 补扩展名", "img_xxx", false, &client.ResourceMeta{ContentType: "image/png; charset=binary"}, "img_xxx.png"},
		{"显式无扩展名只补扩展名", "out/report", true, &client.ResourceMeta{FileName: "周报.pdf"}, "out/report.pdf"},
		{"显式带扩展名不改", "a.bin", true, &client.ResourceMeta{FileName: "x.pdf"}, "a.bin"},
		{"服务端文件名路径穿越被拒", "file_xxx", false, &client.ResourceMeta{FileName: "../../etc/passwd", ContentType: "text/plain"}, "passwd"},
		{"隐藏文件名被拒退回 MIME", "file_xxx", false, &client.ResourceMeta{FileName: ".bashrc", ContentType: "text/plain"}, "file_xxx.txt"},
		{"未知 MIME 保持原样", "file_xxx", false, &client.ResourceMeta{ContentType: "application/x-unknown"}, "file_xxx"},
		{"无元信息保持原样", "file_xxx", false, nil, "file_xxx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveResourceDownloadName(tt.path, tt.explicit, tt.meta); got != filepath.FromSlash(tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNonClobberingPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.pdf")
	if got := nonClobberingPath(p); got != p {
		t.Fatalf("不存在时应原样返回: %s", got)
	}
	_ = os.WriteFile(p, []byte("x"), 0o600)
	if got := nonClobberingPath(p); got != filepath.Join(dir, "a_1.pdf") {
		t.Fatalf("已存在时应追加 _1: %s", got)
	}
}

func TestParseContentDispositionFilename(t *testing.T) {
	if got := client.ParseContentDispositionFilename(`attachment; filename*=UTF-8''%E5%91%A8%E6%8A%A5.pdf`); got != "周报.pdf" {
		t.Fatalf("filename* 解析失败: %q", got)
	}
	if got := client.ParseContentDispositionFilename(`attachment; filename="a b.txt"`); got != "a b.txt" {
		t.Fatalf("filename 解析失败: %q", got)
	}
	if got := client.ParseContentDispositionFilename(""); got != "" {
		t.Fatalf("空头应返回空: %q", got)
	}
}
