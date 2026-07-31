package cmd

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

// testImage 生成一张 7x5 的纯色测试图。
func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 7, 5))
	for y := 0; y < 5; y++ {
		for x := 0; x < 7; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	return img
}

func writeTempImage(t *testing.T, name string, encode func(w io.Writer, img image.Image) error) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	var buf bytes.Buffer
	if err := encode(&buf, testImage()); err != nil {
		t.Fatalf("编码 %s 失败: %v", name, err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写入 %s 失败: %v", name, err)
	}
	return path
}

// TestDecodeImagePixelSizeFormats 覆盖 png/jpeg/gif/bmp/tiff 五种可编码格式
// （webp 标准库无编码器，注册机制与 bmp/tiff 相同，由导入实测覆盖）。
func TestDecodeImagePixelSizeFormats(t *testing.T) {
	cases := []struct {
		name   string
		encode func(w io.Writer, img image.Image) error
	}{
		{"a.png", func(w io.Writer, img image.Image) error { return png.Encode(w, img) }},
		{"a.jpg", func(w io.Writer, img image.Image) error { return jpeg.Encode(w, img, nil) }},
		{"a.gif", func(w io.Writer, img image.Image) error { return gif.Encode(w, img, nil) }},
		{"a.bmp", func(w io.Writer, img image.Image) error { return bmp.Encode(w, img) }},
		{"a.tiff", func(w io.Writer, img image.Image) error { return tiff.Encode(w, img, nil) }},
	}
	for _, tc := range cases {
		path := writeTempImage(t, tc.name, tc.encode)
		w, h := decodeImagePixelSize(path)
		if w != 7 || h != 5 {
			t.Errorf("%s: decodeImagePixelSize = (%d, %d)，期望 (7, 5)", tc.name, w, h)
		}
	}
}

func TestDecodeImagePixelSizeFailures(t *testing.T) {
	// 文件不存在
	if w, h := decodeImagePixelSize(filepath.Join(t.TempDir(), "missing.png")); w != 0 || h != 0 {
		t.Errorf("不存在的文件应返回 (0,0)，实际 (%d, %d)", w, h)
	}
	// 非图片内容
	path := filepath.Join(t.TempDir(), "not-image.png")
	if err := os.WriteFile(path, []byte("this is not an image"), 0o644); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if w, h := decodeImagePixelSize(path); w != 0 || h != 0 {
		t.Errorf("非图片内容应返回 (0,0)，实际 (%d, %d)", w, h)
	}
}

// exifApp1Segment 构造带指定 Orientation 的完整 APP1 段（小端 TIFF，含 marker 与长度字节）。
func exifApp1Segment(orientation byte) []byte {
	tiffBody := []byte{
		'I', 'I', 0x2A, 0x00, // 小端 TIFF 头
		0x08, 0x00, 0x00, 0x00, // IFD0 偏移 = 8
		0x01, 0x00, // 1 个目录项
		0x12, 0x01, // tag 0x0112 Orientation
		0x03, 0x00, // 类型 SHORT
		0x01, 0x00, 0x00, 0x00, // 数量 1
		orientation, 0x00, 0x00, 0x00, // 值（内联）
		0x00, 0x00, 0x00, 0x00, // 下一 IFD 偏移
	}
	payload := append([]byte("Exif\x00\x00"), tiffBody...)
	segLen := len(payload) + 2
	seg := []byte{0xFF, 0xE1, byte(segLen >> 8), byte(segLen & 0xFF)}
	return append(seg, payload...)
}

// jpegWithOrientation 在标准 jpeg.Encode 输出的 SOI 之后插入 EXIF APP1 段。
func jpegWithOrientation(t *testing.T, orientation byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(), nil); err != nil {
		t.Fatalf("编码 JPEG 失败: %v", err)
	}
	raw := buf.Bytes()
	spliced := append(append(append([]byte{}, raw[:2]...), exifApp1Segment(orientation)...), raw[2:]...)
	path := filepath.Join(t.TempDir(), "exif.jpg")
	if err := os.WriteFile(path, spliced, 0o644); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	return path
}

// TestDecodeImagePixelSizeEXIFOrientation 验证宽高转置的旋转（5-8）退回 (0,0)，
// 非转置方向（1-4）正常返回存储尺寸。
func TestDecodeImagePixelSizeEXIFOrientation(t *testing.T) {
	for _, o := range []byte{1, 2, 3, 4} {
		path := jpegWithOrientation(t, o)
		if w, h := decodeImagePixelSize(path); w != 7 || h != 5 {
			t.Errorf("Orientation=%d 不转置宽高，应返回 (7,5)，实际 (%d, %d)", o, w, h)
		}
	}
	for _, o := range []byte{5, 6, 7, 8} {
		path := jpegWithOrientation(t, o)
		if w, h := decodeImagePixelSize(path); w != 0 || h != 0 {
			t.Errorf("Orientation=%d 宽高转置，应退回 (0,0) 交服务端推断，实际 (%d, %d)", o, w, h)
		}
	}
}

func TestJpegEXIFOrientation(t *testing.T) {
	// 无 EXIF 的普通 JPEG → 0
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(), nil); err != nil {
		t.Fatalf("编码 JPEG 失败: %v", err)
	}
	if o := jpegEXIFOrientation(bytes.NewReader(buf.Bytes())); o != 0 {
		t.Errorf("无 EXIF 应返回 0，实际 %d", o)
	}
	// 非 JPEG 内容 → 0
	if o := jpegEXIFOrientation(bytes.NewReader([]byte("plain text"))); o != 0 {
		t.Errorf("非 JPEG 应返回 0，实际 %d", o)
	}
}

func TestExifOrientationFromAPP1(t *testing.T) {
	// 小端（去掉 marker 和长度字节后的负载）
	le := exifApp1Segment(6)[4:]
	if o := exifOrientationFromAPP1(le); o != 6 {
		t.Errorf("小端 Orientation=6 解析失败，实际 %d", o)
	}
	// 大端
	be := append([]byte("Exif\x00\x00"), []byte{
		'M', 'M', 0x00, 0x2A, // 大端 TIFF 头
		0x00, 0x00, 0x00, 0x08, // IFD0 偏移 = 8
		0x00, 0x01, // 1 个目录项
		0x01, 0x12, // tag 0x0112
		0x00, 0x03, // 类型 SHORT
		0x00, 0x00, 0x00, 0x01, // 数量 1
		0x00, 0x08, 0x00, 0x00, // 值 8（内联，前 2 字节）
		0x00, 0x00, 0x00, 0x00, // 下一 IFD
	}...)
	if o := exifOrientationFromAPP1(be); o != 8 {
		t.Errorf("大端 Orientation=8 解析失败，实际 %d", o)
	}
	// 非 EXIF 头 → 0
	if o := exifOrientationFromAPP1([]byte("XMP data here, not exif")); o != 0 {
		t.Errorf("非 EXIF 负载应返回 0，实际 %d", o)
	}
	// 截断负载 → 0（不 panic）
	if o := exifOrientationFromAPP1(le[:10]); o != 0 {
		t.Errorf("截断负载应返回 0，实际 %d", o)
	}
}
