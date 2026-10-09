// Package clipboard 从系统剪贴板读取图片字节（全程内存，不落临时文件）。
//
// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.）。
//
// 平台支持：
//
//	macOS   — osascript（系统自带）：先取 PNG，取不到再扫描 HTML/RTF/文本里内嵌的 base64 data URI 图片
//	Windows — PowerShell + System.Windows.Forms（系统自带），以 base64 输出 PNG
//	Linux   — 依次尝试 xclip（X11）、wl-paste（Wayland）、xsel（X11 兜底）
//
// 所有失败（缺工具、剪贴板没有图片、工具执行失败）都归为用法错误（退出码 2）：
// 需要用户复制图片或安装工具后再试，重试同一命令没有意义。
package clipboard

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// 以下变量供测试注入，避免依赖宿主剪贴板与外部命令。
var (
	goos     = runtime.GOOS
	lookPath = exec.LookPath
	// runCommand 执行外部命令，分别返回 stdout 与去除首尾空白的 stderr。
	runCommand = func(name string, args ...string) ([]byte, string, error) {
		cmd := exec.Command(name, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		return out, strings.TrimSpace(stderr.String()), err
	}
)

// ReadImage 读取剪贴板中的图片并返回原始字节（Linux/Windows 为 PNG；macOS 也可能是 data URI 中的 JPEG/GIF 等）。
func ReadImage() ([]byte, error) {
	var (
		data []byte
		err  error
	)
	switch goos {
	case "darwin":
		data, err = readDarwin()
	case "windows":
		data, err = readWindows()
	case "linux":
		data, err = readLinux()
	default:
		return nil, clierr.Usagef("当前系统 %s 不支持从剪贴板读取图片，请改用 --file 指定本地图片", goos)
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errNoImage("")
	}
	return data, nil
}

func errNoImage(detail string) error {
	msg := "剪贴板中没有图片数据，请先复制一张图片，或改用 --file 指定本地图片"
	if detail != "" {
		msg += "（" + detail + "）"
	}
	return clierr.Usage(errors.New(msg))
}

// reBase64DataURI 匹配剪贴板文本中内嵌的 data URI 图片（如 data:image/jpeg;base64,/9j/4AAQ...）。
// 字符集同时覆盖标准与 URL-safe base64，并包含空白：HTML/RTF 常按 76 字符折行。
var reBase64DataURI = regexp.MustCompile(`data:(image/[^;]+);base64,([A-Za-z0-9+/\-_\s]+=*)`)

// readDarwin 先让 osascript 以 PNG 取剪贴板（输出 «data PNGf…» 十六进制字面量），
// 取不到时依次扫描 HTML / RTF / 文本格式里的 base64 data URI 图片（从飞书、浏览器复制的图片多为此形式）。
func readDarwin() ([]byte, error) {
	out, stderrText, runErr := runCommand("osascript", "-e", "get the clipboard as «class PNGf»")
	if runErr == nil && len(out) > 0 {
		if data, err := decodeOsascriptData(strings.TrimSpace(string(out))); err == nil && len(data) > 0 {
			return data, nil
		}
	}
	if data := extractBase64ImageFromClipboard(); data != nil {
		return data, nil
	}
	if stderrText != "" {
		return nil, errNoImage("osascript: " + stderrText)
	}
	return nil, errNoImage("")
}

// clipboardTextFormats 是扫描内嵌 data URI 图片时依次尝试的 osascript 类型转换。
var clipboardTextFormats = []string{
	"get the clipboard as «class HTML»",
	"get the clipboard as «class RTF »",
	"get the clipboard as «class utf8»",
	"get the clipboard as string",
}

// extractBase64ImageFromClipboard 逐个文本格式查找 data URI 图片；解码结果必须带已知图片魔数，
// 避免把教程、代码样例里字面出现的 data URI 当成图片上传。
func extractBase64ImageFromClipboard() []byte {
	for _, expr := range clipboardTextFormats {
		out, _, err := runCommand("osascript", "-e", expr)
		if err != nil || len(out) == 0 {
			continue
		}
		decoded, err := decodeOsascriptData(strings.TrimSpace(string(out)))
		if err != nil || len(decoded) == 0 {
			continue
		}
		if img := imageFromDataURI(decoded); img != nil {
			return img
		}
	}
	return nil
}

// imageFromDataURI 从文本中提取第一个 data URI 图片并解码；不是有效图片时返回 nil。
func imageFromDataURI(text []byte) []byte {
	m := reBase64DataURI.FindSubmatch(text)
	if m == nil {
		return nil
	}
	b64 := strings.Join(strings.Fields(string(m[2])), "")
	img, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		img, err = base64.URLEncoding.DecodeString(b64)
	}
	if err != nil || len(img) == 0 || !HasKnownImageMagic(img) {
		return nil
	}
	return img
}

// decodeOsascriptData 把 osascript 输出的 «data XXXX<hex>» 字面量转为原始字节；
// 不是该格式时原样返回（普通字符串输出）。
func decodeOsascriptData(s string) ([]byte, error) {
	const prefix = "«data "
	if !strings.HasPrefix(s, prefix) {
		return []byte(s), nil
	}
	s = s[len(prefix):]
	if len(s) >= 4 {
		s = s[4:] // 跳过 4 字符类型码，如 PNGf、HTML
	}
	s = strings.TrimSpace(strings.TrimSuffix(s, "»"))
	return hex.DecodeString(s)
}

// readWindows 用 PowerShell 把剪贴板图片编码为 PNG 并以 base64 写到 stdout，在 Go 侧解码（不落临时文件）。
func readWindows() ([]byte, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$img = [System.Windows.Forms.Clipboard]::GetImage()
if ($img -eq $null) { Write-Error 'clipboard contains no image data'; exit 1 }
$ms = New-Object System.IO.MemoryStream
$img.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
[Convert]::ToBase64String($ms.ToArray())
`
	out, stderrText, err := runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		detail := stderrText
		if detail == "" {
			detail = err.Error()
		}
		return nil, clierr.Usagef("读取剪贴板图片失败（PowerShell: %s）；请先复制一张图片，或改用 --file", detail)
	}
	data, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if decErr != nil {
		return nil, clierr.Usagef("解码剪贴板图片失败: %v", decErr)
	}
	return data, nil
}

// pngMagic 是 PNG 文件头，用于校验无法按 MIME 类型取数据的工具（xsel）的输出。
var pngMagic = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

func hasPNGMagic(b []byte) bool { return bytes.HasPrefix(b, pngMagic) }

// HasKnownImageMagic 判断字节流是否以常见图片格式（PNG/JPEG/GIF/WebP/BMP）的文件头开头。
func HasKnownImageMagic(b []byte) bool {
	switch {
	case hasPNGMagic(b),
		bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}),
		bytes.HasPrefix(b, []byte("GIF87a")),
		bytes.HasPrefix(b, []byte("GIF89a")),
		bytes.HasPrefix(b, []byte("BM")):
		return true
	case bytes.HasPrefix(b, []byte("RIFF")):
		// RIFF 还可能是 WAV/AVI，只有偏移 8 处为 WEBP 才算图片
		return len(b) >= 12 && string(b[8:12]) == "WEBP"
	}
	return false
}

// linuxTool 描述一个 Linux 剪贴板读取工具。
type linuxTool struct {
	name        string
	args        []string
	validatePNG bool // 无法按 MIME 类型取数据时（xsel）需要校验 PNG 文件头
}

var linuxTools = []linuxTool{
	{"xclip", []string{"-selection", "clipboard", "-t", "image/png", "-o"}, false},
	{"wl-paste", []string{"--type", "image/png"}, false},
	{"xsel", []string{"--clipboard", "--output"}, true},
}

// readLinux 依次尝试 xclip / wl-paste / xsel，返回第一个成功工具读到的 PNG。
// 工具存在但失败时保留最后一个错误，避免误报"未安装工具"。
func readLinux() ([]byte, error) {
	var lastErr error
	found := false
	for _, t := range linuxTools {
		if _, err := lookPath(t.name); err != nil {
			continue
		}
		found = true
		out, stderrText, err := runCommand(t.name, t.args...)
		if err != nil {
			detail := stderrText
			if detail == "" {
				detail = err.Error()
			}
			lastErr = clierr.Usagef("通过 %s 读取剪贴板图片失败: %s（剪贴板里可能没有 PNG 图片，或当前会话没有可用的图形显示 DISPLAY/WAYLAND_DISPLAY）", t.name, detail)
			continue
		}
		if len(out) == 0 {
			lastErr = errNoImage(t.name + " 返回空内容")
			continue
		}
		if t.validatePNG && !hasPNGMagic(out) {
			lastErr = errNoImage(t.name + " 输出的不是 PNG 图片")
			continue
		}
		return out, nil
	}
	if found && lastErr != nil {
		return nil, lastErr
	}
	return nil, clierr.Usagef("读取剪贴板图片失败：未找到可用的剪贴板工具。请安装 xclip（X11）、wl-clipboard（Wayland，提供 wl-paste）或 xsel 之一，" +
		"例如 `sudo apt install xclip`、`sudo dnf install wl-clipboard`、`sudo pacman -S xclip`；或改用 --file 指定本地图片")
}
