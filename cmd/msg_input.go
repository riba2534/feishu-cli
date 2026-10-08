package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/safefile"
)

// messageInputStdin 是 --text/--markdown/--content 取值为 "-" 时读取的输入流（测试可替换）。
var messageInputStdin io.Reader = os.Stdin

// maxMessageInputBytes 内容输入上限：飞书消息体最大约 150KB（卡片 30KB），留足余量防误读大文件。
const maxMessageInputBytes = 4 << 20

// expandMessageInputValue 解析消息内容 flag 的统一输入语法（对齐官方 Input: File/Stdin）：
//
//	取值 "-"      从 stdin 读取全部内容
//	取值 "@@xxx"  转义：发送字面量 "@xxx"
//	取值 "@path"  读取文件 path 的内容（去掉 UTF-8 BOM）
//
// literalFallback=true（--text / --markdown）时，若 @path 指向的文件不存在则按字面文本发送，
// 保持旧版"@张三 你好"这类文本的兼容；看起来像路径时在 stderr 提示。
// --content 必须是 JSON，以 @ 开头不可能是合法 JSON，因此文件不存在直接报错。
func expandMessageInputValue(flagName, raw string, literalFallback bool, stdinUsed *bool, errOut io.Writer) (string, error) {
	switch {
	case raw == "":
		return raw, nil
	case raw == "-":
		if stdinUsed != nil && *stdinUsed {
			return "", clierr.Usagef("%s: stdin（-）只能被一个参数使用", flagName)
		}
		if stdinUsed != nil {
			*stdinUsed = true
		}
		data, err := io.ReadAll(io.LimitReader(messageInputStdin, maxMessageInputBytes+1))
		if err != nil {
			return "", fmt.Errorf("%s: 读取 stdin 失败: %w", flagName, err)
		}
		if len(data) > maxMessageInputBytes {
			return "", clierr.Usagef("%s: stdin 内容超过 %d 字节上限", flagName, maxMessageInputBytes)
		}
		return stripUTF8BOM(string(data)), nil
	case strings.HasPrefix(raw, "@@"):
		return raw[1:], nil
	case strings.HasPrefix(raw, "@"):
		path := strings.TrimSpace(raw[1:])
		if path == "" {
			if literalFallback {
				return raw, nil
			}
			return "", clierr.Usagef("%s: @ 后的文件路径不能为空", flagName)
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			if literalFallback && os.IsNotExist(statErr) {
				if looksLikeFilePath(path) && errOut != nil {
					fmt.Fprintf(errOut, "[提示] %s 以 @ 开头但文件 %q 不存在，按字面文本发送；读取文件请检查路径，字面 @ 开头可用 @@ 转义\n", flagName, path)
				}
				return raw, nil
			}
			return "", clierr.Usagef("%s: 读取文件 %s 失败: %v（字面 @ 开头请用 @@ 转义）", flagName, path, statErr)
		}
		if !info.Mode().IsRegular() {
			return "", clierr.Usagef("%s: %s 不是普通文件", flagName, path)
		}
		if info.Size() > maxMessageInputBytes {
			return "", clierr.Usagef("%s: 文件 %s 超过 %d 字节上限", flagName, path, maxMessageInputBytes)
		}
		if err := safefile.ValidateInputPath(path); err != nil {
			return "", clierr.Usage(fmt.Errorf("%s: %w", flagName, err))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("%s: 读取文件 %s 失败: %w", flagName, path, err)
		}
		return stripUTF8BOM(string(data)), nil
	default:
		return raw, nil
	}
}

func stripUTF8BOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}

// looksLikeFilePath 粗略判断 @ 后的内容是否像文件路径（无空白且含路径分隔符或扩展名）。
func looksLikeFilePath(s string) bool {
	if strings.ContainsAny(s, " \t\n") {
		return false
	}
	return strings.ContainsAny(s, `/\`) || strings.Contains(s, ".")
}

// expandInputSources 就地展开 --text / --markdown / --content 的 @file 与 - 输入。
// 必须在 validate 之前调用。
func (input *messageContentInput) expandInputSources(errOut io.Writer) error {
	stdinUsed := false
	var err error
	if input.text, err = expandMessageInputValue("--text", input.text, true, &stdinUsed, errOut); err != nil {
		return err
	}
	if input.markdown, err = expandMessageInputValue("--markdown", input.markdown, true, &stdinUsed, errOut); err != nil {
		return err
	}
	if input.content, err = expandMessageInputValue("--content", input.content, false, &stdinUsed, errOut); err != nil {
		return err
	}
	return nil
}
