package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var markdownFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "读取 Drive 中的原生 Markdown (.md) 文件内容",
	Long: `下载一个 Drive 上的 .md 文件，按需要直接打印到 stdout 或保存到本地。

底层走 ` + "`/open-apis/drive/v1/files/{file_token}/download`" + `（client.DownloadFileWithToken），
与 ` + "`drive download`" + ` 同一 endpoint，但默认面向 .md 文本场景：未指定 --output 时直接打印为 UTF-8。

必填:
  --file-token   Markdown 文件 token

可选:
  --output       本地保存路径（缺省时打印到 stdout）
  --overwrite    本地路径已存在时是否覆盖
  --user-access-token  覆盖登录态

权限:
  - User Access Token
  - drive:file:download（或 drive:drive）

示例:
  feishu-cli markdown fetch --file-token boxcnxxx                 # 打印到 stdout
  feishu-cli markdown fetch --file-token boxcnxxx --output ./local.md
  feishu-cli markdown fetch --file-token boxcnxxx --output ./local.md --overwrite`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := requireUserToken(cmd, "markdown fetch")
		if err != nil {
			return err
		}

		fileToken, _ := cmd.Flags().GetString("file-token")
		outputPath, _ := cmd.Flags().GetString("output")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		outputFormat, _ := cmd.Flags().GetString("output-format")

		if fileToken == "" {
			return fmt.Errorf("--file-token 必填")
		}

		// 没传 --output → 落临时文件读完打印（保持与 lark-cli 行为一致：直接给字符串）。
		printToStdout := outputPath == ""
		finalPath := outputPath
		var cleanup func()
		if printToStdout {
			tmp, err := os.CreateTemp("", "feishu-md-fetch-*.md")
			if err != nil {
				return fmt.Errorf("创建临时文件失败: %w", err)
			}
			tmp.Close()
			finalPath = tmp.Name()
			cleanup = func() { os.Remove(finalPath) }
			defer cleanup()
		} else {
			// 路径是目录时，拼上文件名（用 fileToken.md 兜底）。
			if stat, err := os.Stat(finalPath); err == nil && stat.IsDir() {
				finalPath = filepath.Join(finalPath, fileToken+".md")
			}
			if _, err := os.Stat(finalPath); err == nil && !overwrite {
				return fmt.Errorf("本地文件已存在: %s（使用 --overwrite 覆盖）", finalPath)
			}
		}

		if err := client.DownloadFileWithToken(fileToken, finalPath, token); err != nil {
			return err
		}

		data, err := os.ReadFile(finalPath)
		if err != nil {
			return fmt.Errorf("读取下载文件失败: %w", err)
		}

		if printToStdout {
			if outputFormat == "json" {
				return printJSON(map[string]any{
					"file_token": fileToken,
					"content":    string(data),
					"size_bytes": len(data),
				})
			}
			fmt.Print(string(data))
			return nil
		}

		stat, _ := os.Stat(finalPath)
		result := map[string]any{
			"file_token": fileToken,
			"saved_path": finalPath,
			"size_bytes": int64(0),
		}
		if stat != nil {
			result["size_bytes"] = stat.Size()
		}

		if outputFormat == "json" {
			return printJSON(result)
		}

		fmt.Printf("Markdown 文件下载成功!\n")
		fmt.Printf("  保存路径: %s\n", finalPath)
		if stat != nil {
			fmt.Printf("  大小:     %d bytes\n", stat.Size())
		}
		return nil
	},
}

func init() {
	markdownCmd.AddCommand(markdownFetchCmd)
	markdownFetchCmd.Flags().String("file-token", "", "Markdown 文件 token（必填）")
	markdownFetchCmd.Flags().String("output", "", "本地保存路径（缺省打印到 stdout）")
	markdownFetchCmd.Flags().Bool("overwrite", false, "本地路径已存在时是否覆盖")
	markdownFetchCmd.Flags().String("output-format", "", "输出格式（json）")
	markdownFetchCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(markdownFetchCmd, "file-token")
}
