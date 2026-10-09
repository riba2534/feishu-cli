package safefile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// MkdirAll 创建用户指定的输出目录：先按 ValidateOutputPath 拒绝敏感目录，再 os.MkdirAll。
// 违规时返回用法错误（退出码 2），不创建任何目录。CLI 自管目录直接用 os.MkdirAll。
func MkdirAll(dir string, perm os.FileMode) error {
	if err := ValidateOutputPath(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, perm)
}

// StatInputFile 校验并检查用户指定的本地输入文件（上传、--xxx-file、Markdown 源文件等）：
//   - 敏感目录（~/.ssh、~/.feishu-cli、/etc 等）拒绝读取；
//   - 不存在、是目录、无权限访问时返回带路径的中文用法错误（退出码 2），提示修正参数而不是重试。
func StatInputFile(path string) (os.FileInfo, error) {
	if err := ValidateInputPath(path); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, InputFileError(path, err)
	}
	if info.IsDir() {
		return nil, clierr.Usagef("输入文件 %q 是目录，请指定具体文件路径", path)
	}
	return info, nil
}

// OpenInputFile 按 StatInputFile 的规则校验后打开输入文件（流式上传等场景），调用方负责 Close。
func OpenInputFile(path string) (*os.File, os.FileInfo, error) {
	info, err := StatInputFile(path)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, InputFileError(path, err)
	}
	return f, info, nil
}

// ReadInputFile 按 StatInputFile 的规则校验后读取整个输入文件。
func ReadInputFile(path string) ([]byte, error) {
	if _, err := StatInputFile(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, InputFileError(path, err)
	}
	return data, nil
}

// InputFileError 把读取本地输入文件时的系统错误归类：不存在 / 无权限属于参数问题，
// 返回用法错误（退出码 2）；其他 I/O 错误原样包装（退出码 1）。
func InputFileError(path string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrNotExist):
		return clierr.Usagef("输入文件 %q 不存在，请检查路径是否正确", path)
	case errors.Is(err, fs.ErrPermission):
		return clierr.Usagef("无权限读取输入文件 %q，请检查文件权限或换用其他文件", path)
	}
	return fmt.Errorf("读取输入文件 %q 失败: %w", path, err)
}
