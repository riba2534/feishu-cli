package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var msgResourceDownloadCmd = &cobra.Command{
	Use:   "resource-download <message_id> <file_key>",
	Short: "下载消息中的资源文件（图片/文件）",
	Long: `下载消息中的图片或文件资源。
使用用户身份直连下载时，如遇到飞书大文件限制，会自动使用 HTTP Range 分片下载并合并。

参数:
  message_id  消息 ID（om_xxx 格式）
  file_key    文件 key（img_xxx 或 file_xxx 格式）

选项:
  --type       资源类型（image 或 file，必填）
  -o, --output 输出文件路径（默认用服务端文件名；拿不到时用 file_key + 按 MIME 推断的扩展名）
  --timeout    下载超时时间（默认 5m，大文件可设置更长如 30m、1h）
  --user-access-token  使用用户身份下载用户可见、但 Bot 不可见的历史消息资源

示例:
  # 下载图片
  feishu-cli msg resource-download om_xxx img_xxx --type image -o photo.png

  # 下载文件
  feishu-cli msg resource-download om_xxx file_xxx --type file -o document.pdf

  # 使用用户身份下载
  feishu-cli msg resource-download om_xxx file_xxx --type file --user-access-token u-xxx -o document.pdf

  # 大文件下载，设置 30 分钟超时
  feishu-cli msg resource-download om_xxx file_xxx --type file -o large.zip --timeout 30m`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		messageID := args[0]
		fileKey := args[1]
		resourceType, _ := cmd.Flags().GetString("type")
		outputPath, _ := cmd.Flags().GetString("output")
		timeoutStr, _ := cmd.Flags().GetString("timeout")
		userToken := resolveOptionalUserTokenWithFallback(cmd)

		if strings.ContainsAny(fileKey, `/\`) || strings.TrimSpace(fileKey) == "" {
			return clierr.Usagef("file_key 无效（不能为空或包含路径分隔符）: %q", fileKey)
		}
		explicitOutput := outputPath != ""
		if outputPath == "" {
			outputPath = fileKey
		}
		if err := validateOutputPath(outputPath, ""); err != nil {
			return err
		}

		var timeout time.Duration
		if timeoutStr != "" {
			var err error
			timeout, err = time.ParseDuration(timeoutStr)
			if err != nil {
				return fmt.Errorf("无效的超时时间格式: %s（示例: 10m, 1h）", timeoutStr)
			}
		}

		meta, err := client.DownloadMessageResourceWithMeta(messageID, fileKey, resourceType, outputPath, userToken, timeout)
		if err != nil {
			return err
		}
		// 输出路径没有扩展名时按服务端文件名 / MIME 补全（对齐官方）：
		// 未指定 -o → 用 Content-Disposition 文件名（否则 file_key + MIME 扩展名）；
		// 指定了无扩展名的 -o → 保留文件名，只补扩展名。不覆盖已存在的文件。
		if final := resolveResourceDownloadName(outputPath, explicitOutput, meta); final != outputPath {
			final = nonClobberingPath(final)
			if renameErr := os.Rename(outputPath, final); renameErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "[提示] 已下载到 %s，重命名为 %s 失败: %v\n", outputPath, final, renameErr)
			} else {
				outputPath = final
			}
		}

		fmt.Printf("资源下载成功！\n")
		fmt.Printf("  消息 ID:   %s\n", messageID)
		fmt.Printf("  文件 Key:  %s\n", fileKey)
		fmt.Printf("  资源类型:  %s\n", resourceType)
		fmt.Printf("  保存路径:  %s\n", outputPath)

		return nil
	},
}

func init() {
	msgCmd.AddCommand(msgResourceDownloadCmd)
	msgResourceDownloadCmd.Flags().String("type", "", "资源类型（image 或 file）")
	msgResourceDownloadCmd.Flags().StringP("output", "o", "", "输出文件路径（默认用服务端文件名，拿不到时 file_key+扩展名；无扩展名时自动补全）")
	msgResourceDownloadCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	msgResourceDownloadCmd.Flags().String("timeout", "", "下载超时时间（默认 5m，示例: 10m, 30m, 1h）")
	mustMarkFlagRequired(msgResourceDownloadCmd, "type")
}

var resourceMimeToExt = map[string]string{
	"image/png":          ".png",
	"image/jpeg":         ".jpg",
	"image/gif":          ".gif",
	"image/webp":         ".webp",
	"image/svg+xml":      ".svg",
	"image/bmp":          ".bmp",
	"image/tiff":         ".tiff",
	"application/pdf":    ".pdf",
	"video/mp4":          ".mp4",
	"video/quicktime":    ".mov",
	"audio/mpeg":         ".mp3",
	"audio/ogg":          ".ogg",
	"audio/opus":         ".opus",
	"audio/wav":          ".wav",
	"text/plain":         ".txt",
	"text/csv":           ".csv",
	"text/html":          ".html",
	"application/json":   ".json",
	"application/xml":    ".xml",
	"application/zip":    ".zip",
	"application/msword": ".doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
	"application/vnd.ms-excel": ".xls",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.ms-powerpoint":                                             ".ppt",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
}

// resolveResourceDownloadName 计算下载后的最终文件名；返回值等于 path 表示无需改名。
func resolveResourceDownloadName(path string, explicit bool, meta *client.ResourceMeta) string {
	if filepath.Ext(path) != "" || meta == nil {
		return path
	}
	serverName := sanitizeServerFileName(meta.FileName)
	if serverName != "" {
		if !explicit {
			return filepath.Join(filepath.Dir(path), serverName)
		}
		if ext := filepath.Ext(serverName); ext != "" {
			return path + ext
		}
	}
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(meta.ContentType, ";")[0]))
	if ext, ok := resourceMimeToExt[mimeType]; ok {
		return path + ext
	}
	return path
}

// sanitizeServerFileName 只取服务端文件名的 basename，拒绝空名、. / .. 与控制字符，防止路径穿越。
func sanitizeServerFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == ".." || name == "/" || strings.HasPrefix(name, ".") {
		return ""
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return name
}

// nonClobberingPath 目标已存在时追加 _1、_2…，避免覆盖用户已有文件。
func nonClobberingPath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return path
}
