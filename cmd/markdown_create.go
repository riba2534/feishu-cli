package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var markdownCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "在 Drive 创建一个原生 Markdown (.md) 文件",
	Long: `把一段 Markdown 内容（或本地 .md 文件）作为普通 Drive 文件上传，保留原始 Markdown 格式。

底层调用 ` + "`/open-apis/drive/v1/files/upload_all`" + `（parent_type=explorer），与 ` + "`feishu-cli drive upload`" + ` 同一 endpoint，
但本命令强制 .md 后缀、面向 AI agent 文档写盘场景。

必填:
  --name           远端文件名（必须以 .md 结尾），与 --content 搭配使用
  --content        Markdown 字符串内容（或用 --content-file 指向本地文件）
  --content-file   本地 .md 文件路径（与 --content 二选一）

可选:
  --folder-token        目标文件夹 token（默认 Drive 根目录）
  --user-access-token   覆盖登录态

权限:
  - User Access Token
  - drive:file:upload（或 drive:drive）

示例:
  feishu-cli markdown create --name plan.md --content "# Plan\n\n- todo 1"
  feishu-cli markdown create --content-file ./local.md --folder-token fldxxx
  feishu-cli markdown create --name draft.md --content-file ./tmp.md --folder-token fldxxx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := requireUserToken(cmd, "markdown create")
		if err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		content, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		folderToken, _ := cmd.Flags().GetString("folder-token")
		output, _ := cmd.Flags().GetString("output")

		if content != "" && contentFile != "" {
			return fmt.Errorf("--content 与 --content-file 不能同时使用")
		}
		if content == "" && contentFile == "" {
			return fmt.Errorf("请提供 --content 或 --content-file")
		}

		// 解析最终文件名：优先 --name；否则若是 --content-file 用本地文件 basename。
		fileName := strings.TrimSpace(name)
		if fileName == "" && contentFile != "" {
			fileName = filepath.Base(contentFile)
		}
		if fileName == "" {
			return fmt.Errorf("--name 必填（使用 --content 时）")
		}
		if !strings.HasSuffix(strings.ToLower(fileName), ".md") {
			return fmt.Errorf("--name 必须以 .md 结尾，得到 %q", fileName)
		}

		// 准备本地路径：--content 写临时 .md；--content-file 直接复用本地文件路径。
		uploadPath := contentFile
		var cleanup func()
		if content != "" {
			tmpFile, err := os.CreateTemp("", "feishu-md-*.md")
			if err != nil {
				return fmt.Errorf("创建临时文件失败: %w", err)
			}
			if _, err := tmpFile.WriteString(content); err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return fmt.Errorf("写入临时文件失败: %w", err)
			}
			if err := tmpFile.Close(); err != nil {
				os.Remove(tmpFile.Name())
				return fmt.Errorf("关闭临时文件失败: %w", err)
			}
			uploadPath = tmpFile.Name()
			cleanup = func() { os.Remove(uploadPath) }
			defer cleanup()
		}

		stat, err := os.Stat(uploadPath)
		if err != nil {
			return fmt.Errorf("读取本地文件失败: %w", err)
		}
		if stat.IsDir() {
			return fmt.Errorf("--content-file 必须指向文件，不是目录")
		}
		if stat.Size() == 0 {
			return fmt.Errorf("Markdown 内容为空，不支持创建空 .md 文件")
		}

		fileToken, err := client.UploadFileWithToken(uploadPath, folderToken, fileName, token)
		if err != nil {
			return err
		}

		result := map[string]any{
			"file_token": fileToken,
			"file_name":  fileName,
			"size_bytes": stat.Size(),
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("Markdown 文件创建成功!\n")
		fmt.Printf("  file_name:  %s\n", fileName)
		fmt.Printf("  file_token: %s\n", fileToken)
		fmt.Printf("  size:       %d bytes\n", stat.Size())
		return nil
	},
}

func init() {
	markdownCmd.AddCommand(markdownCreateCmd)
	markdownCreateCmd.Flags().String("name", "", "远端文件名（必须 .md 结尾；--content 搭配时必填）")
	markdownCreateCmd.Flags().String("content", "", "Markdown 字符串内容（与 --content-file 二选一）")
	markdownCreateCmd.Flags().String("content-file", "", "本地 .md 文件路径（与 --content 二选一）")
	markdownCreateCmd.Flags().String("folder-token", "", "目标文件夹 token（默认 Drive 根目录）")
	markdownCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	markdownCreateCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}
