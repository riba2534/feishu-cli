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

var markdownOverwriteCmd = &cobra.Command{
	Use:   "overwrite",
	Short: "覆盖 Drive 中已存在的 Markdown (.md) 文件",
	Long: `把新的 Markdown 内容（字符串或本地文件）写到一个已存在的 .md 文件，file_token 保持不变。

底层调 ` + "`POST /open-apis/drive/v1/files/upload_all`" + `，与 create 同一 endpoint，区别仅是多带 ` + "`file_token`" + ` 字段。
飞书 Go SDK v3.5.3 的 ` + "`UploadAllFileReqBody`" + ` 没暴露 file_token，所以走自定义 multipart（见 internal/client/markdown.go）。

必填:
  --file-token     目标 .md 文件 token
  --content        新 Markdown 内容（与 --content-file 二选一）
  --content-file   本地 .md 文件路径（与 --content 二选一）

可选:
  --name           覆盖后修改文件名（必须 .md 结尾；缺省保留原名/用本地 basename）
  --user-access-token  覆盖登录态

权限:
  - User Access Token，且对目标文件有编辑权限
  - drive:file:upload + drive:drive.metadata:readonly

示例:
  feishu-cli markdown overwrite --file-token boxcnxxx --content "新内容"
  feishu-cli markdown overwrite --file-token boxcnxxx --content-file ./new.md
  feishu-cli markdown overwrite --file-token boxcnxxx --content-file ./new.md --name renamed.md`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := requireUserToken(cmd, "markdown overwrite")
		if err != nil {
			return err
		}

		fileToken, _ := cmd.Flags().GetString("file-token")
		name, _ := cmd.Flags().GetString("name")
		content, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		output, _ := cmd.Flags().GetString("output")

		if fileToken == "" {
			return fmt.Errorf("--file-token 必填")
		}
		if content != "" && contentFile != "" {
			return fmt.Errorf("--content 与 --content-file 不能同时使用")
		}
		if content == "" && contentFile == "" {
			return fmt.Errorf("请提供 --content 或 --content-file")
		}

		// 确定新文件名：优先 --name；其次 --content-file basename；不再兜底 fileToken.md
		// （codex review 反馈：fallback fileToken.md 会默默重命名远端文件违反 help 承诺）
		fileName := strings.TrimSpace(name)
		if fileName == "" && contentFile != "" {
			fileName = filepath.Base(contentFile)
		}
		if fileName == "" {
			return fmt.Errorf("使用 --content 时必须提供 --name 指定远端文件名（保留原名请加 --name <现有文件名>.md）")
		}
		if !strings.HasSuffix(strings.ToLower(fileName), ".md") {
			return fmt.Errorf("--name 必须以 .md 结尾，得到 %q", fileName)
		}

		// 准备字节内容：--content 走字符串；--content-file 读盘。
		var payload []byte
		if content != "" {
			const maxOverwriteSize = 20 * 1024 * 1024
			if len(content) > maxOverwriteSize {
				return fmt.Errorf("--content 大小 %d 字节超过 20MB API 上限", len(content))
			}
			payload = []byte(content)
		} else {
			stat, err := os.Stat(contentFile)
			if err != nil {
				return fmt.Errorf("读取本地文件失败: %w", err)
			}
			if stat.IsDir() {
				return fmt.Errorf("--content-file 必须指向文件，不是目录")
			}
			// drive/v1/files/upload_all API 单次上传 ≤ 20MB（参 internal/client/markdown.go OverwriteFileWithToken 注释）
			const maxOverwriteSize = 20 * 1024 * 1024
			if stat.Size() > maxOverwriteSize {
				return fmt.Errorf("--content-file 大小 %d 字节超过 20MB API 上限，请切分或用多次 fetch+overwrite", stat.Size())
			}
			data, err := os.ReadFile(contentFile)
			if err != nil {
				return fmt.Errorf("读取本地文件失败: %w", err)
			}
			payload = data
		}
		if len(payload) == 0 {
			return fmt.Errorf("Markdown 内容为空，不支持把 .md 覆盖为空文件")
		}

		returnedToken, err := client.OverwriteFileWithToken(fileToken, fileName, payload, token)
		if err != nil {
			return err
		}

		result := map[string]any{
			"file_token": returnedToken,
			"file_name":  fileName,
			"size_bytes": len(payload),
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("Markdown 文件覆盖成功!\n")
		fmt.Printf("  file_name:  %s\n", fileName)
		fmt.Printf("  file_token: %s\n", returnedToken)
		fmt.Printf("  size:       %d bytes\n", len(payload))
		return nil
	},
}

func init() {
	markdownCmd.AddCommand(markdownOverwriteCmd)
	markdownOverwriteCmd.Flags().String("file-token", "", "目标 .md 文件 token（必填）")
	markdownOverwriteCmd.Flags().String("name", "", "覆盖后修改文件名（必须 .md 结尾；缺省保留原名）")
	markdownOverwriteCmd.Flags().String("content", "", "新 Markdown 内容（与 --content-file 二选一）")
	markdownOverwriteCmd.Flags().String("content-file", "", "本地 .md 文件路径（与 --content 二选一）")
	markdownOverwriteCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	markdownOverwriteCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(markdownOverwriteCmd, "file-token")
}
