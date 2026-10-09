package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var driveDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "下载云盘文件到本地",
	Long: `下载云盘文件到本地（流式写盘，不受 100MB 限制）。
如遇到飞书大文件限制，会自动使用 HTTP Range 分片下载并合并；单个分片失败有界重试并断点续传，
空闲 60 秒无数据视为超时；写盘为临时文件 + rename，失败不会破坏已存在的同名文件。

下载前先用 query_by_token 识别 token 类型：wiki 节点自动解包为底层文件；在线文档（docx/sheet/...）
会提示改用 drive export。识别失败（权限等）只告警，按原 token 继续。

必填:
  --file-token    云盘文件 token，或文件/wiki 的飞书 URL

可选:
  --output        输出路径（文件或已存在的目录）；省略或为目录时文件名按
                  响应头 Content-Disposition → 云盘标题 → token 依次决定
  --overwrite     已存在时覆盖
  --timeout       整个下载的总时长上限（如 30m；默认不限，只受空闲超时约束）
  --as            bot | user | auto（不传时保持旧行为：必须 User Token）
  --user-access-token  覆盖登录态

权限:
  - User Access Token（默认）或 Tenant Token（--as bot）
  - drive:file:download

示例:
  feishu-cli drive download --file-token boxcnxxxx --output ./report.pdf
  feishu-cli drive download --file-token boxcnxxxx --output ./downloads/ --overwrite
  feishu-cli drive download --file-token https://xxx.feishu.cn/file/boxcnxxxx
  feishu-cli drive download --file-token boxcnxxxx --as bot`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		rawToken, _ := cmd.Flags().GetString("file-token")
		outputPath, _ := cmd.Flags().GetString("output")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		timeoutStr, _ := cmd.Flags().GetString("timeout")
		output, _ := cmd.Flags().GetString("output-format")

		if rawToken == "" {
			return clierr.Usagef("--file-token 必填")
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		// 超时解析：未指定时不设总时长（大文件慢网也能完成），只受空闲超时约束
		var timeout time.Duration
		if timeoutStr != "" {
			d, err := time.ParseDuration(timeoutStr)
			if err != nil {
				return clierr.Usagef("解析 --timeout 失败: %v", err)
			}
			timeout = d
		}

		// 输出目标：显式文件路径可在联网前做覆盖检查；敏感目录在任何网络请求（含 token 刷新）之前拒绝
		outputIsDir := false
		if outputPath != "" {
			if err := validateOutputPath(outputPath, ""); err != nil {
				return err
			}
			if stat, err := os.Stat(outputPath); err == nil && stat.IsDir() {
				outputIsDir = true
			} else if err == nil && !overwrite {
				return fmt.Errorf("文件已存在: %s（使用 --overwrite 覆盖）", outputPath)
			}
		}

		token, err := resolveIdentityWithLegacyDefault(cmd, func(c *cobra.Command) (string, error) {
			return requireUserToken(c, "drive download")
		})
		if err != nil {
			return err
		}

		src, err := resolveDriveDownloadSource(rawToken, token, cmd.ErrOrStderr())
		if err != nil {
			return err
		}

		d, err := client.OpenDriveFileDownload(src.FileToken, "", token, timeout)
		if err != nil {
			return err
		}
		defer d.Close()

		finalPath := outputPath
		fileName := ""
		if outputPath == "" || outputIsDir {
			fileName = defaultDownloadName(d, src.FileToken, token)
			if outputIsDir {
				finalPath = filepath.Join(outputPath, fileName)
			} else {
				finalPath = fileName
			}
			if _, err := os.Stat(finalPath); err == nil && !overwrite {
				return fmt.Errorf("文件已存在: %s（使用 --overwrite 覆盖）", finalPath)
			}
		} else {
			fileName = filepath.Base(finalPath)
		}

		size, err := d.SaveTo(finalPath)
		if err != nil {
			return err
		}

		result := map[string]any{
			"file_token": src.FileToken,
			"file_name":  fileName,
			"saved_path": finalPath,
			"size_bytes": size,
		}
		if src.WikiToken != "" {
			result["wiki_token"] = src.WikiToken
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("文件下载成功!\n")
		fmt.Printf("  保存路径: %s\n", finalPath)
		fmt.Printf("  大小:     %d bytes\n", size)
		return nil
	},
}

func init() {
	driveCmd.AddCommand(driveDownloadCmd)
	driveDownloadCmd.Flags().String("file-token", "", "云盘文件 token 或飞书 URL（必填）")
	driveDownloadCmd.Flags().String("output", "", "输出路径（文件或已存在的目录）")
	driveDownloadCmd.Flags().Bool("overwrite", false, "已存在时覆盖")
	driveDownloadCmd.Flags().String("timeout", "", "整个下载的总时长上限（如 30m；默认不限，只受 60s 空闲超时约束）")
	driveDownloadCmd.Flags().String("output-format", "", "输出格式（json）")
	driveDownloadCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addLegacyAsFlag(driveDownloadCmd, "保持旧行为（必须 User Token）")
	mustMarkFlagRequired(driveDownloadCmd, "file-token")
}
