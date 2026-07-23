package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jpegMagic 是最小可被 http.DetectContentType 识别为 image/jpeg 的头部字节。
var jpegMagic = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}

func TestBoardImageExt(t *testing.T) {
	pngMagic := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

	cases := []struct {
		name        string
		contentType string
		body        []byte
		wantExt     string
		wantErr     bool
	}{
		{"jpeg header", "image/jpeg", jpegMagic, ".jpg", false},
		{"png header", "image/png", pngMagic, ".png", false},
		{"header 带参数", "image/jpeg; charset=binary", jpegMagic, ".jpg", false},
		{"header 缺失按魔数嗅探 jpeg", "", jpegMagic, ".jpg", false},
		{"header 未知按魔数嗅探 png", "application/octet-stream", pngMagic, ".png", false},
		{"非图片响应报错", "application/json", []byte(`{"code":1}`), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ext, err := boardImageExt(tc.contentType, tc.body)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望报错，实际 ext=%q", ext)
				}
				return
			}
			if err != nil {
				t.Fatalf("意外报错: %v", err)
			}
			if ext != tc.wantExt {
				t.Fatalf("ext=%q, 期望 %q", ext, tc.wantExt)
			}
		})
	}
}

func TestResolveBoardImagePath(t *testing.T) {
	dir := t.TempDir()

	// 目录场景：<whiteboard_id><ext>
	got, err := resolveBoardImagePath(dir, "wb123", ".jpg")
	if err != nil || got != filepath.Join(dir, "wb123.jpg") {
		t.Fatalf("目录场景 got=%q err=%v", got, err)
	}

	// 尾斜杠但目录不存在：仍视为目录意图
	got, err = resolveBoardImagePath(filepath.Join(dir, "notyet")+"/", "wb123", ".jpg")
	if err != nil || got != filepath.Join(dir, "notyet", "wb123.jpg") {
		t.Fatalf("尾斜杠场景 got=%q err=%v", got, err)
	}

	// 无扩展名：自动补
	got, err = resolveBoardImagePath(filepath.Join(dir, "out"), "wb123", ".jpg")
	if err != nil || !strings.HasSuffix(got, "out.jpg") {
		t.Fatalf("无扩展名场景 got=%q err=%v", got, err)
	}

	// 扩展名匹配（.jpeg 视同 .jpg）
	if _, err = resolveBoardImagePath(filepath.Join(dir, "out.jpeg"), "wb123", ".jpg"); err != nil {
		t.Fatalf(".jpeg 应匹配 .jpg: %v", err)
	}

	// 扩展名不符：报错
	if _, err = resolveBoardImagePath(filepath.Join(dir, "out.png"), "wb123", ".jpg"); err == nil {
		t.Fatal("扩展名不符应报错")
	}

	// 非图片扩展名：报错
	if _, err = resolveBoardImagePath(filepath.Join(dir, "out.txt"), "wb123", ".jpg"); err == nil {
		t.Fatal("非图片扩展名应报错")
	}

	// 已存在的普通文件（非目录）带匹配扩展名：原样使用
	existing := filepath.Join(dir, "exists.jpg")
	if err := os.WriteFile(existing, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err = resolveBoardImagePath(existing, "wb123", ".jpg")
	if err != nil || got != existing {
		t.Fatalf("已存在文件场景 got=%q err=%v", got, err)
	}
}
