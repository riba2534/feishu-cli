package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/v2/internal/client"
)

// ===== 缺陷 5：导出资源路径相对 Markdown 文件 =====

// TestExportAssetLinksRelativeToMarkdownFile --download-images 写文件时，资源引用路径相对输出 Markdown 所在目录，
// 使导出后原地 doc import（按 Markdown 目录解析相对路径）能找到资源。
func TestExportAssetLinksRelativeToMarkdownFile(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	outDir := filepath.Join(root, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	blocks := []*larkdocx.Block{
		{BlockId: strPtr("img"), BlockType: intPtr(int(BlockTypeImage)), Image: &larkdocx.Image{Token: strPtr("imgTok")}},
		{BlockId: strPtr("board"), BlockType: intPtr(int(BlockTypeBoard)), Board: &larkdocx.Board{Token: strPtr("boardTok")}},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{DownloadImages: true, AssetsDir: assets, AssetsLinkBase: outDir})
	conv.services = &blockToMarkdownServices{
		getMediaTempURL: func(string, client.DownloadMediaOptions) (string, error) { return "https://example.com/x", nil },
		downloadFromURL: func(string, string) error { return nil },
		downloadMedia:   func(string, string, client.DownloadMediaOptions) error { return nil },
		getBoardImage: func(token, outputPath, userAccessToken string) (string, error) {
			return outputPath + ".png", nil
		},
	}
	md, err := conv.Convert()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"(../assets/image_1.png)", "(../assets/board_2.png)"} {
		if !strings.Contains(md, want) {
			t.Fatalf("资源路径应相对输出文件所在目录 %q:\n%s", want, md)
		}
	}

	// 未设置 AssetsLinkBase（输出到 stdout）时保持原路径
	conv2 := NewBlockToMarkdown(blocks[:1], ConvertOptions{DownloadImages: true, AssetsDir: "./assets"})
	conv2.services = conv.services
	md2, _ := conv2.Convert()
	if !strings.Contains(md2, "(assets/image_1.png)") {
		t.Fatalf("stdout 模式应保持相对工作目录的路径:\n%s", md2)
	}

	// 含空格的路径用 <...> 包裹，仍是合法的链接目标
	spaced := NewBlockToMarkdown(nil, ConvertOptions{AssetsLinkBase: root})
	if got := spaced.assetLink(filepath.Join(root, "my assets", "a.png")); got != "<my assets/a.png>" {
		t.Fatalf("assetLink with space = %q", got)
	}
	// 导入端能识别 <...> 包裹的路径
	back := convertForTest(t, "![a](<my assets/a.png>)\n", ConvertOptions{UploadImages: true})
	if ref := back.MediaRefs[back.BlockNodes[0].Block]; ref == nil || ref.Source != "my assets/a.png" {
		t.Fatalf("导入端应解析 <...> 包裹的路径: %#v", ref)
	}
}

// TestVideoAssetNameCannotEscapeAssetsDir 视频文件名来自文档内容（协作者可改），含 "../" 或分隔符时
// 必须收敛为资源目录内的单段文件名，不能写出 --assets-dir。
func TestVideoAssetNameCannotEscapeAssetsDir(t *testing.T) {
	assetsDir := t.TempDir()
	conv := NewBlockToMarkdown(nil, ConvertOptions{AssetsDir: assetsDir})
	for _, name := range []string{"../../.bashrc.mp4", "a/b/c.mp4", `..\..\evil.mp4`, "..", "."} {
		got := conv.nextVideoAssetPath(name)
		if filepath.Dir(got) != assetsDir {
			t.Fatalf("nextVideoAssetPath(%q) = %q，越出资源目录 %q", name, got, assetsDir)
		}
	}
}

// TestDownloadImagesRejectsSensitiveAssetsDir --assets-dir 落在敏感目录时不创建目录、不下载（降级为可 roundtrip 的标签）。
func TestDownloadImagesRejectsSensitiveAssetsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(home, ".ssh", "assets")
	blocks := []*larkdocx.Block{
		{BlockId: strPtr("img"), BlockType: intPtr(int(BlockTypeImage)), Image: &larkdocx.Image{Token: strPtr("imgTok")}},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{DownloadImages: true, AssetsDir: assets})
	downloads := 0
	conv.services = &blockToMarkdownServices{
		getMediaTempURL: func(string, client.DownloadMediaOptions) (string, error) { downloads++; return "", nil },
		downloadFromURL: func(string, string) error { downloads++; return nil },
		downloadMedia:   func(string, string, client.DownloadMediaOptions) error { downloads++; return nil },
		getBoardImage:   func(string, string, string) (string, error) { downloads++; return "", nil },
	}
	if _, err := conv.Convert(); err == nil || !strings.Contains(err.Error(), "~/.ssh") {
		t.Fatalf("敏感 --assets-dir 应报错，得到 %v", err)
	}
	if downloads != 0 {
		t.Fatalf("敏感 --assets-dir 不应发起下载，实际 %d 次", downloads)
	}
	if _, err := os.Stat(assets); !os.IsNotExist(err) {
		t.Fatal("不应在 ~/.ssh 下创建资源目录")
	}
}
