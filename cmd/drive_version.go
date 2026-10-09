package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var driveVersionNumberRe = regexp.MustCompile(`^\d{1,19}$`)

var driveVersionHistoryCmd = &cobra.Command{
	Use:   "version-history",
	Short: "列出云盘上传文件的版本历史",
	Long: `列出上传文件（type=file）的版本历史：GET /open-apis/drive/v1/files/{file_token}/history?only_tag=true。

每个版本输出 version（用于 version-get / file version revert，不是 tag）、名称、编辑时间、编辑人、
大小、动作类型（upload/rename/delete_version/revert）。还有更多时输出 next_cursor，用 --cursor 续翻。

必填:
  --file-token   文件 token

可选:
  --limit        每页数量（1-200，默认 20）
  --cursor       上一页输出的 next_cursor
  --as           bot | user | auto（默认 auto）

示例:
  feishu-cli drive version-history --file-token boxcnXXX
  feishu-cli drive version-history --file-token boxcnXXX --limit 50 --cursor 1700000000000 -o json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		fileToken, _ := cmd.Flags().GetString("file-token")
		limit, _ := cmd.Flags().GetInt("limit")
		cursor, _ := cmd.Flags().GetString("cursor")
		output, _ := cmd.Flags().GetString("output")
		fileToken, cursor = strings.TrimSpace(fileToken), strings.TrimSpace(cursor)
		if err := validateResourceIdentifier(fileToken, "--file-token"); err != nil {
			return err
		}
		if limit < 1 || limit > 200 {
			return clierr.Usagef("--limit 必须在 1-200 之间，得到 %d", limit)
		}
		if cursor != "" && !driveVersionNumberRe.MatchString(cursor) {
			return clierr.Usagef("--cursor 必须是上一页输出的数字游标（next_cursor）")
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		page, err := client.ListDriveFileHistory(fileToken, limit, cursor, token)
		if err != nil {
			return err
		}
		if output == "json" {
			return printJSON(page)
		}
		if len(page.Versions) == 0 {
			fmt.Println("没有版本记录")
			return nil
		}
		fmt.Printf("共 %d 个版本:\n", len(page.Versions))
		for _, v := range page.Versions {
			deleted := ""
			if v.IsDeleted {
				deleted = " [已删除]"
			}
			fmt.Printf("  %s  %-14s %10d bytes  %s%s\n", v.Version, v.ActionType, v.SizeBytes, v.Name, deleted)
		}
		if page.HasMore {
			fmt.Fprintf(cmd.ErrOrStderr(), "提示: 还有更多版本，续翻: --cursor %s\n", page.NextCursor)
		}
		return nil
	},
}

var driveVersionGetCmd = &cobra.Command{
	Use:   "version-get",
	Short: "下载云盘上传文件的指定历史版本",
	Long: `下载上传文件的指定历史版本：GET /open-apis/drive/v1/files/{file_token}/download?version=N。
与 drive download 相同的流式链路（分片重试、空闲超时、临时文件 + rename）。

必填:
  --file-token   文件 token
  --version      drive version-history 输出的 version（不是 tag）

可选:
  --output       本地保存路径或已存在的目录；省略或为目录时使用服务端文件名（Content-Disposition）
  --overwrite    已存在时覆盖
  --as           bot | user | auto（默认 auto）

示例:
  feishu-cli drive version-get --file-token boxcnXXX --version 7694404161344900828 --output ./old.pdf`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		fileToken, _ := cmd.Flags().GetString("file-token")
		version, _ := cmd.Flags().GetString("version")
		outputPath, _ := cmd.Flags().GetString("output")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		format, _ := cmd.Flags().GetString("output-format")
		fileToken, version = strings.TrimSpace(fileToken), strings.TrimSpace(version)
		if err := validateResourceIdentifier(fileToken, "--file-token"); err != nil {
			return err
		}
		if !driveVersionNumberRe.MatchString(version) {
			return clierr.Usagef("--version 必须是 drive version-history 输出的数字版本号（不是 tag）")
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		outputIsDir := false
		if outputPath != "" {
			// 敏感目录在任何网络请求（含 token 刷新）之前拒绝
			if err := validateOutputPath(outputPath, ""); err != nil {
				return err
			}
			if st, err := os.Stat(outputPath); err == nil && st.IsDir() {
				outputIsDir = true
			} else if err == nil && !overwrite {
				return fmt.Errorf("文件已存在: %s（使用 --overwrite 覆盖）", outputPath)
			}
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		d, err := client.OpenDriveFileDownload(fileToken, version, token, 0)
		if err != nil {
			return err
		}
		defer d.Close()
		finalPath := outputPath
		if outputPath == "" || outputIsDir {
			name := d.FileName()
			if name == "" {
				name = fileToken + "_v" + version
			}
			finalPath = name
			if outputIsDir {
				finalPath = filepath.Join(outputPath, name)
			}
			if _, err := os.Stat(finalPath); err == nil && !overwrite {
				return fmt.Errorf("文件已存在: %s（使用 --overwrite 覆盖）", finalPath)
			}
		}
		size, err := d.SaveTo(finalPath)
		if err != nil {
			return err
		}
		result := map[string]any{
			"file_token": fileToken,
			"version":    version,
			"file_name":  filepath.Base(finalPath),
			"saved_path": finalPath,
			"size_bytes": size,
		}
		if format == "json" {
			return printJSON(result)
		}
		fmt.Printf("版本下载成功!\n  保存路径: %s\n  大小:     %d bytes\n", finalPath, size)
		return nil
	},
}

func init() {
	driveCmd.AddCommand(driveVersionHistoryCmd)
	driveVersionHistoryCmd.Flags().String("file-token", "", "文件 token（必填）")
	driveVersionHistoryCmd.Flags().Int("limit", 20, "每页数量（1-200）")
	driveVersionHistoryCmd.Flags().String("cursor", "", "上一页输出的 next_cursor")
	addAsFlag(driveVersionHistoryCmd)
	driveVersionHistoryCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	driveVersionHistoryCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(driveVersionHistoryCmd, "file-token")

	driveCmd.AddCommand(driveVersionGetCmd)
	driveVersionGetCmd.Flags().String("file-token", "", "文件 token（必填）")
	driveVersionGetCmd.Flags().String("version", "", "版本号（必填，来自 drive version-history）")
	driveVersionGetCmd.Flags().String("output", "", "本地保存路径或已存在的目录")
	driveVersionGetCmd.Flags().Bool("overwrite", false, "已存在时覆盖")
	driveVersionGetCmd.Flags().String("output-format", "", "输出格式（json）")
	addAsFlag(driveVersionGetCmd)
	driveVersionGetCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(driveVersionGetCmd, "file-token", "version")
}
