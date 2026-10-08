package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/safefile"
)

var markdownExportAtomicWrite = atomicWriteFile

func exportPathEscapes(outputDir, fileName string) bool {
	absDir, err := filepath.Abs(outputDir)
	if err != nil {
		return true
	}
	candidate := filepath.Clean(filepath.Join(absDir, fileName))
	rel, err := filepath.Rel(absDir, candidate)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolveSafeMarkdownExportPath(outputDir, fileName string) (string, error) {
	if strings.TrimSpace(outputDir) == "" {
		outputDir = "."
	}
	if exportPathEscapes(outputDir, fileName) {
		return "", fmt.Errorf("导出文件名不安全，越出 --output-dir: %s", fileName)
	}
	absDir, err := filepath.Abs(outputDir)
	if err != nil {
		return "", fmt.Errorf("解析 --output-dir 失败: %w", err)
	}
	name := sanitizeExportName(filepath.Base(fileName), "export.md")
	target := filepath.Join(absDir, name)
	rel, err := filepath.Rel(absDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("导出路径越出 --output-dir: %s", fileName)
	}
	return target, nil
}

func writeMarkdownExportFile(outputDir, fileName string, data []byte, overwrite bool) (string, error) {
	target, err := resolveSafeMarkdownExportPath(outputDir, fileName)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(target); err == nil && !overwrite {
		return "", fmt.Errorf("文件已存在: %s（使用 --overwrite 覆盖）", target)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("创建 --output-dir 失败: %w", err)
	}
	if err := markdownExportAtomicWrite(target, data); err != nil {
		return "", fmt.Errorf("写文件失败: %w", err)
	}
	return target, nil
}

// atomicWriteFile 把导出内容原子落盘（同目录临时文件 → chmod → write → fsync → rename → fsync 目录，
// Windows 无法直接覆盖时 .bak 兜底），复用 internal/safefile 的公共实现。
// 显式权限位不可省：没有 chmod 时文件会停留在 CreateTemp 的默认权限。
func atomicWriteFile(path string, data []byte) error {
	return safefile.AtomicWriteFile(path, data, exportFilePermForTest)
}

// exportFilePerm 导出文件权限：0600，与 token/缓存一致，避免多用户机器上被旁人读取。
const exportFilePerm os.FileMode = 0600

// exportFilePermForTest 实际使用的权限位，供测试注入以验证 Chmod 步骤真的生效
// （os.CreateTemp 默认恰好也是 0600，用常量断言无法区分）。生产恒等于 exportFilePerm。
var exportFilePermForTest = exportFilePerm
