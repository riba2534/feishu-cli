package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

var markdownFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "读取 Drive 中的原生 Markdown (.md) 文件内容",
	Long: `下载一个 Drive 上的 .md 文件，按需要直接打印到 stdout 或保存到本地。

底层走 ` + "`GET /open-apis/drive/v1/medias/{file_token}/preview_download?preview_type=16`" + `。
可选 ` + "`--version`" + ` 下载历史版本。--as bot|user|auto（默认 auto：User 优先；未配置回退 Bot；已配置但解析/刷新失败 fail-closed）。

必填:
  --file-token   Markdown 文件 token

可选:
  --output-path  本地保存路径（缺省时打印到 stdout）
  --version      历史版本号
  --output, -o   输出格式（json；不传 --output-path 时返回 content）
  --overwrite    本地路径已存在时是否覆盖
  --dry-run      只打印将要发出的请求
  --user-access-token  覆盖登录态

权限:
  - --as bot|user|auto（默认 auto）
  - drive:file:download（或 drive:drive）

示例:
  feishu-cli markdown fetch --file-token boxcnxxx
  feishu-cli markdown fetch --file-token boxcnxxx --output-path ./local.md --overwrite
  feishu-cli markdown fetch --file-token boxcnxxx --version 7633658129540910621 --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken, _ := cmd.Flags().GetString("file-token")
		outputPath, _ := cmd.Flags().GetString("output-path")
		version, _ := cmd.Flags().GetString("version")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		outputFormat, _ := cmd.Flags().GetString("output")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		fileToken = strings.TrimSpace(fileToken)
		outputPath = strings.TrimSpace(outputPath)
		version = strings.TrimSpace(version)
		if fileToken == "" {
			return clierr.Usagef("--file-token 必填")
		}
		if err := validateMarkdownDiffVersionValue(version, "--version"); err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		// 本地保存路径在任何网络请求（含 token 刷新）之前校验，敏感目录直接拒绝
		if outputPath != "" {
			if err := validateOutputPath(outputPath, ""); err != nil {
				return err
			}
		}

		if dryRun {
			return printDryRunPlan(cmd, "download markdown source file preview artifact bytes", map[string]any{
				"file_token": fileToken,
				"output":     markdownFirstNonEmpty(outputPath, "<stdout>"),
			}, []dryRunStep{{
				Method: "GET",
				URL:    markdownPreviewDownloadPath(fileToken),
				Params: markdownPreviewParams(version),
			}})
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		data, fileName, err := client.FetchMarkdownSource(fileToken, version, token)
		if err != nil {
			return err
		}

		printToStdout := outputPath == ""
		if printToStdout {
			if outputFormat == "json" {
				return printJSON(map[string]any{
					"file_token": fileToken,
					"file_name":  fileName,
					"content":    string(data),
					"size_bytes": len(data),
				})
			}
			fmt.Print(string(data))
			return nil
		}

		finalPath := outputPath
		// 保存到目录时文件名取自服务端，按单段文件名收敛（去掉路径分隔符），防止越出目标目录
		localName := safeOutputPath(markdownFirstNonEmpty(strings.TrimSpace(fileName), fileToken+".md"), "")
		if strings.HasSuffix(finalPath, string(os.PathSeparator)) || strings.HasSuffix(finalPath, "/") || strings.HasSuffix(finalPath, "\\") {
			finalPath = filepath.Join(finalPath, localName)
		} else if stat, err := os.Stat(finalPath); err == nil && stat.IsDir() {
			finalPath = filepath.Join(finalPath, localName)
		}
		if _, err := os.Stat(finalPath); err == nil && !overwrite {
			return clierr.Usagef("本地文件已存在: %s（使用 --overwrite 覆盖）", finalPath)
		}
		if err := safefile.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
			return fmt.Errorf("创建输出目录失败: %w", err)
		}
		if err := safefile.AtomicWriteFile(finalPath, data, 0o644); err != nil {
			return fmt.Errorf("写文件失败: %w", err)
		}

		result := map[string]any{
			"file_token": fileToken,
			"file_name":  fileName,
			"saved_path": finalPath,
			"size_bytes": len(data),
		}
		if outputFormat == "json" {
			return printJSON(result)
		}
		fmt.Printf("Markdown 文件下载成功!\n")
		fmt.Printf("  保存路径: %s\n", finalPath)
		fmt.Printf("  大小:     %d bytes\n", len(data))
		return nil
	},
}

func markdownFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func init() {
	markdownCmd.AddCommand(markdownFetchCmd)
	markdownFetchCmd.Flags().String("file-token", "", "Markdown 文件 token（必填）")
	markdownFetchCmd.Flags().String("output-path", "", "本地保存路径（缺省时打印到 stdout）")
	markdownFetchCmd.Flags().String("version", "", "历史版本号（走 preview_download 的 version 查询参数）")
	markdownFetchCmd.Flags().Bool("overwrite", false, "本地路径已存在时是否覆盖")
	markdownFetchCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	markdownFetchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	markdownFetchCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(markdownFetchCmd, "file-token")
}
