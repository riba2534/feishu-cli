package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// mustMarkFlagRequired 标记 flag 为必填，如果失败则 panic
// 用于 init() 函数中，确保配置错误在启动时被发现
func mustMarkFlagRequired(cmd *cobra.Command, flags ...string) {
	for _, flag := range flags {
		if err := cmd.MarkFlagRequired(flag); err != nil {
			panic(fmt.Sprintf("标记必填 flag '%s' 失败: %v", flag, err))
		}
	}
}

// printJSON 安全地打印 JSON 格式的数据
// 如果序列化失败，会返回错误而不是静默忽略
func printJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// confirmAction 在执行危险操作前请求用户确认
// 返回 true 表示用户确认执行，false 表示取消
func confirmAction(prompt string) bool {
	fmt.Printf("%s (y/N): ", prompt)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// validateOutputPath 验证输出路径是否安全
// 防止路径遍历攻击
func validateOutputPath(outputPath string, allowedDir string) error {
	// 清理路径
	cleanPath := filepath.Clean(outputPath)

	// 检查是否包含路径遍历
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("输出路径不能包含 '..'")
	}

	// 如果指定了允许的目录，验证路径在该目录下
	if allowedDir != "" {
		absOutput, err := filepath.Abs(cleanPath)
		if err != nil {
			return fmt.Errorf("无法解析输出路径: %w", err)
		}
		absAllowed, err := filepath.Abs(allowedDir)
		if err != nil {
			return fmt.Errorf("无法解析允许目录: %w", err)
		}
		if !strings.HasPrefix(absOutput, absAllowed) {
			return fmt.Errorf("输出路径必须在 %s 目录下", allowedDir)
		}
	}

	return nil
}

// safeOutputPath 生成安全的输出路径
// 移除不安全的字符，防止路径遍历
func safeOutputPath(baseName string, ext string) string {
	// 移除路径分隔符和不安全字符
	safeName := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, baseName)

	// 限制文件名长度
	if len(safeName) > 200 {
		safeName = safeName[:200]
	}

	if ext != "" && !strings.HasSuffix(safeName, ext) {
		safeName += ext
	}

	return safeName
}
