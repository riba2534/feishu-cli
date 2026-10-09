package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/spf13/cobra"
)

// -o 结果文件指向敏感目录：ParseOptions（命令发请求前调用）即返回用法错误；Render 写入另有兜底。
func TestOutputFileRejectsSensitiveDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(home, ".ssh", "result.json")

	cmd := &cobra.Command{Use: "x"}
	AddOutputFlags(cmd)
	if err := cmd.Flags().Set("output", bad); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseOptions(cmd); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("ParseOptions 应拒绝敏感 -o 路径，得到 %v", err)
	}

	// 绕过 ParseOptions 直接构造选项时，写入兜底同样拒绝
	if err := Render(&Options{Format: FormatJSON, OutputFile: bad}, map[string]any{"a": 1}); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("Render 写入敏感路径应被兜底拒绝，得到 %v", err)
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("被拒绝的路径不应生成文件")
	}

	ok := filepath.Join(t.TempDir(), "result.json")
	if err := Render(&Options{Format: FormatJSON, OutputFile: ok}, map[string]any{"a": 1}); err != nil {
		t.Fatalf("普通路径应可写: %v", err)
	}
	if b, _ := os.ReadFile(ok); len(b) == 0 {
		t.Fatal("普通路径应写出内容")
	}
}
