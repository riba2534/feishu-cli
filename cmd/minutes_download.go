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

var minutesDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "下载妙记媒体文件（批量）",
	Long: `下载妙记对应的音视频媒体文件。

先调用 GET /open-apis/minutes/v1/minutes/{minute_token}/media 获取预签名 URL，
然后通过 HTTP 流式下载到磁盘。

必填:
  --minute-tokens  妙记 token 列表，逗号分隔（最多 50 条）

可选:
  -o, --output  输出路径：
                  • 单 token 模式下：文件路径或目录
                  • 批量模式下：目录（文件名自动从响应头解析）
                默认当前目录
  --overwrite   覆盖已存在文件
  --url-only    只打印下载 URL，不实际下载

权限:
  - 默认 User 身份（--as user），可用 --as bot|auto 切换
  - minutes:minutes.media:export

示例:
  # 单条下载到当前目录
  feishu-cli minutes download --minute-tokens obcnxxxx

  # 批量下载到指定目录
  feishu-cli minutes download --minute-tokens t1,t2,t3 --output ./media --overwrite

  # 只取下载链接
  feishu-cli minutes download --minute-tokens obcnxxxx --url-only`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := resolveVCReadIdentity(cmd)
		if err != nil {
			return err
		}

		raw, _ := cmd.Flags().GetString("minute-tokens")
		outputPath, _ := cmd.Flags().GetString("output")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		urlOnly, _ := cmd.Flags().GetBool("url-only")

		tokens, err := parseCSVIDs(raw, "minute-tokens")
		if err != nil {
			return err
		}
		if len(tokens) == 0 {
			return fmt.Errorf("请通过 --minute-tokens 指定至少一个妙记 token")
		}
		for _, t := range tokens {
			if err := ensureMinuteToken(t); err != nil {
				return err
			}
		}

		// 决定目录与单文件模式
		outputDir := "."
		forcedFilename := ""
		singleMode := len(tokens) == 1
		if outputPath != "" {
			if singleMode {
				// 单条模式：outputPath 可以是文件路径或目录
				if stat, statErr := os.Stat(outputPath); statErr == nil && stat.IsDir() {
					outputDir = outputPath
				} else {
					outputDir = filepath.Dir(outputPath)
					if outputDir == "" || outputDir == "." {
						outputDir = "."
					}
					forcedFilename = filepath.Base(outputPath)
				}
			} else {
				// 批量模式：outputPath 必须是目录
				outputDir = outputPath
				if stat, statErr := os.Stat(outputPath); statErr == nil && !stat.IsDir() {
					return fmt.Errorf("批量模式下 --output 必须是目录，当前 %q 是文件", outputPath)
				}
			}
		}

		if !urlOnly {
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("创建输出目录失败: %w", err)
			}
		}

		items := make([]vcBatchItem, 0, len(tokens))
		usedNames := make(map[string]struct{}) // 批量模式文件名去重
		rateTicker := time.NewTicker(time.Second / 5)
		defer rateTicker.Stop()

		for i, mt := range tokens {
			if i > 0 {
				<-rateTicker.C
			}

			presigned, err := client.GetMinuteMediaURL(mt, token)
			if err != nil {
				items = append(items, vcBatchItem{ID: mt, OK: false, Error: err.Error()})
				continue
			}

			if urlOnly {
				items = append(items, vcBatchItem{
					ID: mt,
					OK: true,
					Data: map[string]any{
						"download_url": presigned,
					},
				})
				continue
			}

			opts := client.DownloadOptions{
				OutputDir: outputDir,
				Overwrite: overwrite,
			}
			if forcedFilename != "" {
				opts.Filename = forcedFilename
			}

			result, err := client.DownloadFromPresignedURL(presigned, mt, opts)
			if err != nil {
				items = append(items, vcBatchItem{ID: mt, OK: false, Error: err.Error()})
				continue
			}

			// 批量模式下处理文件名冲突
			if !singleMode && forcedFilename == "" {
				finalName := result.Filename
				if _, dup := usedNames[finalName]; dup {
					newName := mt + "-" + finalName
					newPath := filepath.Join(outputDir, newName)
					if err := os.Rename(result.SavedPath, newPath); err == nil {
						result.SavedPath = newPath
						result.Filename = newName
					}
				}
				usedNames[result.Filename] = struct{}{}
			}

			items = append(items, vcBatchItem{
				ID: mt,
				OK: true,
				Data: map[string]any{
					"saved_path":   result.SavedPath,
					"size_bytes":   result.Size,
					"download_url": presigned,
				},
			})
		}

		summary := summarizeBatch(items)

		// 文本输出
		for i, it := range items {
			fmt.Printf("[%d] %s\n", i+1, it.ID)
			if !it.OK {
				fmt.Printf("    FAIL: %s\n", it.Error)
				continue
			}
			if m, ok := it.Data.(map[string]any); ok {
				if urlOnly {
					fmt.Printf("    download_url: %s\n", m["download_url"])
				} else {
					fmt.Printf("    saved_path:   %s\n", m["saved_path"])
					fmt.Printf("    size_bytes:   %v\n", m["size_bytes"])
				}
			}
		}
		fmt.Printf("\n合计: %d / 成功 %d / 失败 %d\n", summary.Total, summary.Succeeded, summary.Failed)

		if summary.Failed > 0 && summary.Succeeded == 0 {
			return fmt.Errorf("全部妙记下载失败")
		}
		return nil
	},
}

func init() {
	minutesCmd.AddCommand(minutesDownloadCmd)
	minutesDownloadCmd.Flags().String("minute-tokens", "", "妙记 token 列表，逗号分隔（最多 50 条）")
	minutesDownloadCmd.Flags().StringP("output", "o", "", "输出路径（文件或目录）")
	minutesDownloadCmd.Flags().Bool("overwrite", false, "覆盖已存在文件")
	minutesDownloadCmd.Flags().Bool("url-only", false, "只打印下载 URL，不下载")
	minutesDownloadCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addVCReadAsFlag(minutesDownloadCmd)
	mustMarkFlagRequired(minutesDownloadCmd, "minute-tokens")
}
