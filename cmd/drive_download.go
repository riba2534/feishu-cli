package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var driveDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "下载云盘文件到本地",
	Long: `下载云盘文件到本地。
如遇到飞书大文件限制，会自动使用 HTTP Range 分片下载并合并。

必填:
  --file-token    云盘文件 token

可选:
  --output        输出路径（文件或目录），默认当前目录
  --overwrite     已存在时覆盖
  --timeout       下载超时（如 10m，默认 5m）
  --user-access-token  覆盖登录态

权限:
  - User Access Token
  - drive:file:download

示例:
  feishu-cli drive download --file-token boxcnxxxx --output ./report.pdf
  feishu-cli drive download --file-token boxcnxxxx --output ./downloads/ --overwrite`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := requireUserToken(cmd, "drive download")
		if err != nil {
			return err
		}

		fileToken, _ := cmd.Flags().GetString("file-token")
		outputPath, _ := cmd.Flags().GetString("output")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		timeoutStr, _ := cmd.Flags().GetString("timeout")
		output, _ := cmd.Flags().GetString("output-format")

		if fileToken == "" {
			return fmt.Errorf("--file-token 必填")
		}

		// 解析输出路径
		finalPath := outputPath
		if finalPath == "" {
			finalPath = fileToken // 兜底用 token 作为文件名
		} else if stat, err := os.Stat(finalPath); err == nil && stat.IsDir() {
			finalPath = filepath.Join(finalPath, fileToken)
		}

		// 覆盖检查
		if _, err := os.Stat(finalPath); err == nil && !overwrite {
			return fmt.Errorf("文件已存在: %s（使用 --overwrite 覆盖）", finalPath)
		}

		// 超时解析
		timeout := 5 * time.Minute
		if timeoutStr != "" {
			if d, err := time.ParseDuration(timeoutStr); err == nil {
				timeout = d
			} else {
				return fmt.Errorf("解析 --timeout 失败: %w", err)
			}
		}

		if err := client.DownloadFileWithToken(fileToken, finalPath, token, timeout); err != nil {
			return err
		}

		stat, _ := os.Stat(finalPath)
		result := map[string]any{
			"file_token": fileToken,
			"saved_path": finalPath,
			"size_bytes": 0,
		}
		if stat != nil {
			result["size_bytes"] = stat.Size()
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("文件下载成功!\n")
		fmt.Printf("  保存路径: %s\n", finalPath)
		if stat != nil {
			fmt.Printf("  大小:     %d bytes\n", stat.Size())
		}
		return nil
	},
}

func init() {
	driveCmd.AddCommand(driveDownloadCmd)
	driveDownloadCmd.Flags().String("file-token", "", "云盘文件 token（必填）")
	driveDownloadCmd.Flags().String("output", "", "输出路径（文件或目录）")
	driveDownloadCmd.Flags().Bool("overwrite", false, "已存在时覆盖")
	driveDownloadCmd.Flags().String("timeout", "", "下载超时（如 10m，默认 5m）")
	driveDownloadCmd.Flags().String("output-format", "", "输出格式（json）")
	driveDownloadCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(driveDownloadCmd, "file-token")
}
