package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPrintJSON_Success(t *testing.T) {
	// 保存原始 stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	testData := struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}{
		Name: "测试用户",
		Age:  25,
	}

	err := printJSON(testData)

	// 恢复 stdout
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("printJSON() 返回错误: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, `"name": "测试用户"`) {
		t.Errorf("输出不包含预期内容: %s", output)
	}
	if !strings.Contains(output, `"age": 25`) {
		t.Errorf("输出不包含预期内容: %s", output)
	}
}

func TestPrintJSON_Error(t *testing.T) {
	// channel 无法被 JSON 序列化
	badData := make(chan int)

	err := printJSON(badData)
	if err == nil {
		t.Error("printJSON() 应返回错误，因为 channel 无法序列化")
	}
}

func TestPrintJSON_Nil(t *testing.T) {
	// 保存原始 stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printJSON(nil)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("printJSON(nil) 返回错误: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := strings.TrimSpace(buf.String())

	if output != "null" {
		t.Errorf("输出 = %q, 期望 %q", output, "null")
	}
}

func TestPrintJSON_EmptyStruct(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printJSON(struct{}{})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("printJSON() 返回错误: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := strings.TrimSpace(buf.String())

	if output != "{}" {
		t.Errorf("输出 = %q, 期望 %q", output, "{}")
	}
}

func TestPrintJSON_Slice(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	testData := []string{"a", "b", "c"}
	err := printJSON(testData)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("printJSON() 返回错误: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, `"a"`) {
		t.Errorf("输出不包含预期内容: %s", output)
	}
}

func TestValidateOutputPath_Valid(t *testing.T) {
	tests := []struct {
		name       string
		outputPath string
		allowedDir string
	}{
		{"简单文件名", "output.md", ""},
		{"相对路径", "./output/file.md", ""},
		{"当前目录", ".", ""},
		{"允许目录内", "./subdir/file.md", "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOutputPath(tt.outputPath, tt.allowedDir)
			if err != nil {
				t.Errorf("validateOutputPath(%q, %q) 返回错误: %v", tt.outputPath, tt.allowedDir, err)
			}
		})
	}
}

func TestValidateOutputPath_Invalid(t *testing.T) {
	tests := []struct {
		name       string
		outputPath string
		allowedDir string
		errContain string
	}{
		{"路径遍历 1", "../etc/passwd", "", ".."},
		{"路径遍历 2", "../../secret", "", ".."},
		{"路径遍历 3", "./dir/../../../root", "", ".."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOutputPath(tt.outputPath, tt.allowedDir)
			if err == nil {
				t.Errorf("validateOutputPath(%q, %q) 应返回错误", tt.outputPath, tt.allowedDir)
			}
		})
	}
}

func TestSafeOutputPath_Basic(t *testing.T) {
	tests := []struct {
		name     string
		baseName string
		ext      string
		expected string
	}{
		{"简单名称", "document", ".md", "document.md"},
		{"已有扩展名", "document.md", ".md", "document.md"},
		{"无扩展名", "document", "", "document"},
		{"中文名称", "测试文档", ".md", "测试文档.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeOutputPath(tt.baseName, tt.ext)
			if result != tt.expected {
				t.Errorf("safeOutputPath(%q, %q) = %q, 期望 %q", tt.baseName, tt.ext, result, tt.expected)
			}
		})
	}
}

func TestSafeOutputPath_UnsafeCharacters(t *testing.T) {
	tests := []struct {
		name     string
		baseName string
		ext      string
		contains string
	}{
		{"斜杠", "path/to/file", ".md", "path_to_file"},
		{"反斜杠", "path\\to\\file", ".md", "path_to_file"},
		{"冒号", "C:file", ".md", "C_file"},
		{"星号", "file*name", ".md", "file_name"},
		{"问号", "file?name", ".md", "file_name"},
		{"引号", `file"name`, ".md", "file_name"},
		{"小于号", "file<name", ".md", "file_name"},
		{"大于号", "file>name", ".md", "file_name"},
		{"管道符", "file|name", ".md", "file_name"},
		{"多个特殊字符", "a/b\\c:d*e?f", ".txt", "a_b_c_d_e_f"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeOutputPath(tt.baseName, tt.ext)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("safeOutputPath(%q, %q) = %q, 应包含 %q", tt.baseName, tt.ext, result, tt.contains)
			}
			// 验证不包含不安全字符
			unsafeChars := []rune{'/', '\\', ':', '*', '?', '"', '<', '>', '|'}
			for _, c := range unsafeChars {
				if strings.ContainsRune(result, c) {
					t.Errorf("safeOutputPath(%q, %q) = %q, 不应包含字符 %q", tt.baseName, tt.ext, result, string(c))
				}
			}
		})
	}
}

func TestSafeOutputPath_LongName(t *testing.T) {
	// 创建超过 200 字符的名称
	longName := strings.Repeat("a", 250)
	result := safeOutputPath(longName, ".md")

	// 应该截断到 200 字符 + 扩展名
	if len(result) != 200+len(".md") {
		t.Errorf("safeOutputPath() 长度 = %d, 期望 %d", len(result), 200+len(".md"))
	}

	if !strings.HasSuffix(result, ".md") {
		t.Errorf("safeOutputPath() 应以 .md 结尾")
	}
}

func TestSafeOutputPath_ExactlyMaxLength(t *testing.T) {
	// 正好 200 字符
	name := strings.Repeat("b", 200)
	result := safeOutputPath(name, ".txt")

	if len(result) != 200+len(".txt") {
		t.Errorf("safeOutputPath() 长度 = %d, 期望 %d", len(result), 200+len(".txt"))
	}
}

func TestMustMarkFlagRequired_Success(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}
	cmd.Flags().String("file", "", "文件路径")
	cmd.Flags().String("output", "", "输出路径")

	// 不应 panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("mustMarkFlagRequired() 不应 panic: %v", r)
		}
	}()

	mustMarkFlagRequired(cmd, "file", "output")
}

func TestMustMarkFlagRequired_Panic(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}
	// 不添加 flag，直接标记为必填应该 panic

	defer func() {
		if r := recover(); r == nil {
			t.Error("mustMarkFlagRequired() 应该 panic")
		}
	}()

	mustMarkFlagRequired(cmd, "nonexistent")
}

func TestMustMarkFlagRequired_MultipleFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}
	cmd.Flags().String("a", "", "")
	cmd.Flags().String("b", "", "")
	cmd.Flags().String("c", "", "")

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("mustMarkFlagRequired() 不应 panic: %v", r)
		}
	}()

	mustMarkFlagRequired(cmd, "a", "b", "c")
}

func TestMustMarkFlagRequired_EmptyFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
	}

	// 空的 flags 列表不应 panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("mustMarkFlagRequired() 不应 panic: %v", r)
		}
	}()

	mustMarkFlagRequired(cmd)
}

// 测试 validateOutputPath 与允许目录的交互
func TestValidateOutputPath_WithAllowedDir(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		outputPath string
		allowedDir string
		shouldPass bool
	}{
		{"目录内文件", tmpDir + "/output.md", tmpDir, true},
		{"目录内子目录", tmpDir + "/subdir/output.md", tmpDir, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOutputPath(tt.outputPath, tt.allowedDir)
			if tt.shouldPass && err != nil {
				t.Errorf("validateOutputPath(%q, %q) 返回错误: %v", tt.outputPath, tt.allowedDir, err)
			}
			if !tt.shouldPass && err == nil {
				t.Errorf("validateOutputPath(%q, %q) 应返回错误", tt.outputPath, tt.allowedDir)
			}
		})
	}
}
