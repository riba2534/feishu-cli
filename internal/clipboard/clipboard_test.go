package clipboard

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

var testPNG = append(append([]byte{}, pngMagic...), []byte("\x00\x00\x00\rIHDRfake")...)

type fakeCall struct {
	name string
	args []string
}

// stubEnv 注入 goos / lookPath / runCommand，返回记录到的调用。
func stubEnv(t *testing.T, osName string, installed map[string]bool, run func(name string, args ...string) ([]byte, string, error)) *[]fakeCall {
	t.Helper()
	oldOS, oldLook, oldRun := goos, lookPath, runCommand
	t.Cleanup(func() { goos, lookPath, runCommand = oldOS, oldLook, oldRun })
	goos = osName
	lookPath = func(name string) (string, error) {
		if installed[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	var calls []fakeCall
	runCommand = func(name string, args ...string) ([]byte, string, error) {
		calls = append(calls, fakeCall{name, args})
		return run(name, args...)
	}
	return &calls
}

func TestReadImageLinuxToolOrder(t *testing.T) {
	cases := []struct {
		name      string
		installed map[string]bool
		run       func(name string, args ...string) ([]byte, string, error)
		wantTool  string
		wantErr   string
	}{
		{
			name:      "xclip 优先且按 image/png 取数据",
			installed: map[string]bool{"xclip": true, "wl-paste": true, "xsel": true},
			run: func(name string, args ...string) ([]byte, string, error) {
				if name == "xclip" && strings.Join(args, " ") == "-selection clipboard -t image/png -o" {
					return testPNG, "", nil
				}
				return nil, "", errors.New("unexpected")
			},
			wantTool: "xclip",
		},
		{
			name:      "xclip 失败时回退 wl-paste",
			installed: map[string]bool{"xclip": true, "wl-paste": true},
			run: func(name string, args ...string) ([]byte, string, error) {
				if name == "xclip" {
					return nil, "Error: Can't open display: (null)", errors.New("exit status 1")
				}
				return testPNG, "", nil
			},
			wantTool: "wl-paste",
		},
		{
			name:      "xsel 输出不是 PNG 时报无图片",
			installed: map[string]bool{"xsel": true},
			run: func(name string, args ...string) ([]byte, string, error) {
				return []byte("plain text"), "", nil
			},
			wantErr: "不是 PNG",
		},
		{
			name:      "xsel 输出 PNG 时接受",
			installed: map[string]bool{"xsel": true},
			run: func(name string, args ...string) ([]byte, string, error) {
				return testPNG, "", nil
			},
			wantTool: "xsel",
		},
		{
			name:      "工具存在但失败时保留 stderr，不误报未安装",
			installed: map[string]bool{"xclip": true},
			run: func(name string, args ...string) ([]byte, string, error) {
				return nil, "Error: Can't open display: (null)", errors.New("exit status 1")
			},
			wantErr: "Can't open display",
		},
		{
			name:      "空输出视为没有图片",
			installed: map[string]bool{"wl-paste": true},
			run: func(name string, args ...string) ([]byte, string, error) {
				return nil, "", nil
			},
			wantErr: "没有图片",
		},
		{
			name:      "三个工具都没装时给出安装建议",
			installed: map[string]bool{},
			run: func(name string, args ...string) ([]byte, string, error) {
				t.Fatalf("不应执行任何命令: %s", name)
				return nil, "", nil
			},
			wantErr: "sudo apt install xclip",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := stubEnv(t, "linux", c.installed, c.run)
			data, err := ReadImage()
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("期望错误含 %q，得到 %v", c.wantErr, err)
				}
				if !clierr.HasKind(err, clierr.KindUsage) {
					t.Fatalf("剪贴板错误应为用法错误（退出码 2）: %v", err)
				}
				return
			}
			if err != nil || !bytes.Equal(data, testPNG) {
				t.Fatalf("ReadImage = (%q, %v)", data, err)
			}
			last := (*calls)[len(*calls)-1]
			if last.name != c.wantTool {
				t.Fatalf("最终使用的工具 = %s，期望 %s（调用序列 %v）", last.name, c.wantTool, *calls)
			}
		})
	}
}

func TestReadImageDarwin(t *testing.T) {
	hexPNG := "«data PNGf" + strings.ToUpper(hex.EncodeToString(testPNG)) + "»"
	t.Run("PNGf 十六进制字面量", func(t *testing.T) {
		stubEnv(t, "darwin", nil, func(name string, args ...string) ([]byte, string, error) {
			if strings.Contains(args[1], "PNGf") {
				return []byte(hexPNG + "\n"), "", nil
			}
			return nil, "", errors.New("unexpected")
		})
		data, err := ReadImage()
		if err != nil || !bytes.Equal(data, testPNG) {
			t.Fatalf("ReadImage = (%q, %v)", data, err)
		}
	})
	t.Run("HTML 中折行的 data URI", func(t *testing.T) {
		jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
		b64 := base64.StdEncoding.EncodeToString(jpeg)
		html := `<img src="data:image/jpeg;base64,` + b64[:6] + "\n" + b64[6:] + `">`
		stubEnv(t, "darwin", nil, func(name string, args ...string) ([]byte, string, error) {
			switch {
			case strings.Contains(args[1], "PNGf"):
				return nil, "execution error: Can’t make some data into the expected type. (-1700)", errors.New("exit status 1")
			case strings.Contains(args[1], "HTML"):
				return []byte("«data HTML" + hex.EncodeToString([]byte(html)) + "»"), "", nil
			}
			return nil, "", errors.New("no such format")
		})
		data, err := ReadImage()
		if err != nil || !bytes.Equal(data, jpeg) {
			t.Fatalf("ReadImage = (%x, %v)", data, err)
		}
	})
	t.Run("文本里字面出现的 data URI 不是图片时拒绝", func(t *testing.T) {
		fake := base64.StdEncoding.EncodeToString([]byte("not an image at all"))
		stubEnv(t, "darwin", nil, func(name string, args ...string) ([]byte, string, error) {
			if strings.Contains(args[1], "as string") {
				return []byte("示例 data:image/png;base64," + fake), "", nil
			}
			return nil, "osascript failed", errors.New("exit status 1")
		})
		if _, err := ReadImage(); err == nil || !strings.Contains(err.Error(), "没有图片") {
			t.Fatalf("应报没有图片，得到 %v", err)
		}
	})
}

func TestReadImageWindows(t *testing.T) {
	stubEnv(t, "windows", nil, func(name string, args ...string) ([]byte, string, error) {
		if name != "powershell" {
			t.Fatalf("应调用 powershell，得到 %s", name)
		}
		return []byte(base64.StdEncoding.EncodeToString(testPNG) + "\r\n"), "", nil
	})
	data, err := ReadImage()
	if err != nil || !bytes.Equal(data, testPNG) {
		t.Fatalf("ReadImage = (%q, %v)", data, err)
	}

	stubEnv(t, "windows", nil, func(name string, args ...string) ([]byte, string, error) {
		return nil, "clipboard contains no image data", errors.New("exit status 1")
	})
	if _, err := ReadImage(); err == nil || !strings.Contains(err.Error(), "clipboard contains no image data") || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("PowerShell 失败应透出 stderr 并为用法错误，得到 %v", err)
	}
}

func TestReadImageUnsupportedOS(t *testing.T) {
	stubEnv(t, "plan9", nil, func(name string, args ...string) ([]byte, string, error) { return nil, "", nil })
	if _, err := ReadImage(); err == nil || !strings.Contains(err.Error(), "plan9") {
		t.Fatalf("不支持的系统应报错，得到 %v", err)
	}
}

func TestHasKnownImageMagic(t *testing.T) {
	cases := map[string]struct {
		data []byte
		want bool
	}{
		"png":  {testPNG, true},
		"jpeg": {[]byte{0xff, 0xd8, 0xff, 0xdb}, true},
		"gif":  {[]byte("GIF89a...."), true},
		"bmp":  {[]byte("BM\x00\x00"), true},
		"webp": {[]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), true},
		"wav":  {[]byte("RIFF\x00\x00\x00\x00WAVEfmt "), false},
		"text": {[]byte("hello"), false},
	}
	for name, c := range cases {
		if got := HasKnownImageMagic(c.data); got != c.want {
			t.Errorf("%s: HasKnownImageMagic = %v，期望 %v", name, got, c.want)
		}
	}
}
