package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var docMediaDownloadCmd = &cobra.Command{
	Use:   "media-download <token>",
	Short: "下载文档中的素材（图片/文件/画板缩略图）",
	Long: `下载文档中嵌入的图片、文件或画板缩略图。

参数:
  token       素材文件 token 或画板 ID
  --type      素材类型（media/whiteboard，默认 media）
  --output    输出文件路径（默认使用 token 作为文件名）
  --doc-token 素材所属文档 token 或 URL（文档内嵌图片需要；wiki URL 自动解析为底层文档）
  --doc-type  素材所属文档类型（默认 docx；--doc-token 为 URL 时按路径推断，冲突报错）
  --extra     原始 extra JSON（优先于 --doc-token/--doc-type）
  --timeout   下载超时时间（默认 5m，大文件可设置更长如 30m、1h）
  --overwrite 目标文件已存在时覆盖（默认拒绝覆盖并报错）

单个素材上限 100MB：超过时报错，且不会留下半截文件或改动已存在的同名文件。

输出文件名没有扩展名时（含默认的 token 文件名），按下载内容识别类型自动补扩展名
（png/jpg/gif/webp/bmp/pdf/docx/xlsx/pptx/zip/mp4/txt 等）。

示例:
  # 下载图片素材
  feishu-cli doc media-download boxcnXXX -o image.png

  # 下载文档内嵌图片
  feishu-cli doc media-download boxcnXXX --doc-token DOC_TOKEN --doc-type docx -o image.png

  # 手动指定素材下载 extra
  feishu-cli doc media-download boxcnXXX --extra '{"doc_token":"DOC_TOKEN","doc_type":"docx"}' -o image.png

  # 下载画板缩略图
  feishu-cli doc media-download XXX --type whiteboard -o board.png

  # 大文件下载，设置 30 分钟超时
  feishu-cli doc media-download boxcnXXX -o large.bin --timeout 30m`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := args[0]
		mediaType, _ := cmd.Flags().GetString("type")
		output, _ := cmd.Flags().GetString("output")
		docToken, _ := cmd.Flags().GetString("doc-token")
		docType, _ := cmd.Flags().GetString("doc-type")
		extra, _ := cmd.Flags().GetString("extra")
		timeoutStr, _ := cmd.Flags().GetString("timeout")
		overwrite, _ := cmd.Flags().GetBool("overwrite")

		if output == "" {
			output = safeOutputPath(token, "")
		}

		// 输出路径在任何网络请求（含 token 刷新）之前校验，敏感目录直接拒绝
		if err := validateOutputPath(output, ""); err != nil {
			return fmt.Errorf("输出路径不安全: %w", err)
		}
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)
		// 显式扩展名时可在下载前就检查覆盖；无扩展名时在识别出最终文件名后再检查
		if mediaHasExplicitExtension(output) {
			if err := ensureNotOverwriting(output, overwrite); err != nil {
				return err
			}
		}

		var timeout time.Duration
		if timeoutStr != "" {
			var err error
			timeout, err = time.ParseDuration(timeoutStr)
			if err != nil {
				return fmt.Errorf("无效的超时时间格式: %s（示例: 10m, 1h）", timeoutStr)
			}
		}

		// --doc-token 支持 URL 与 wiki：换出底层文档 token/类型（--extra 优先，此时不解析）
		if strings.TrimSpace(docToken) != "" && strings.TrimSpace(extra) == "" {
			explicitType := ""
			if cmd.Flags().Changed("doc-type") {
				explicitType = docType
			}
			res, err := resolveResourceArg(docToken, resourceArgOptions{
				ArgName:         "--doc-token",
				ExplicitType:    explicitType,
				DefaultType:     docType,
				ResolveWiki:     true,
				UserAccessToken: userAccessToken,
			})
			if err != nil {
				return err
			}
			noteWikiResolved(res)
			docToken, docType = res.Token, res.Type
		}

		switch mediaType {
		case "whiteboard":
			// 下载画板缩略图（扩展名按服务端实际格式决定）
			if !mediaHasExplicitExtension(output) {
				for _, ext := range []string{".png", ".jpg", ".jpeg", ".svg"} {
					if err := ensureNotOverwriting(output+ext, overwrite); err != nil {
						return err
					}
				}
			}
			savedPath, err := client.GetBoardImage(token, output, userAccessToken)
			if err != nil {
				return withMediaDownloadHint(fmt.Errorf("下载画板缩略图失败: %w", err), mediaType)
			}
			fmt.Printf("已下载到 %s\n", savedPath)
			return nil

		default:
			// 下载媒体文件（图片/附件）
			opts := client.DownloadMediaOptions{
				UserAccessToken: userAccessToken,
				DocToken:        docToken,
				DocType:         docType,
				Extra:           extra,
				Timeout:         timeout,
			}

			// 先下载到同目录临时文件，识别类型、检查覆盖后再原子改名，
			// 避免"已有文件被静默覆盖"与"下载失败留下半截文件"
			tmp, err := os.CreateTemp(filepath.Dir(output), ".feishu-media-*.part")
			if err != nil {
				return fmt.Errorf("创建临时文件失败: %w", err)
			}
			tmpPath := tmp.Name()
			_ = tmp.Close()
			defer os.Remove(tmpPath)

			downloaded := false
			// 优先尝试临时 URL 下载
			if url, err := client.GetMediaTempURL(token, opts); err == nil {
				if dlErr := client.DownloadFromURL(url, tmpPath, timeout); dlErr == nil {
					downloaded = true
				}
			}
			// 降级为直接下载
			if !downloaded {
				if err := client.DownloadMedia(token, tmpPath, opts); err != nil {
					return withMediaDownloadHint(fmt.Errorf("下载素材失败: %w", err), mediaType)
				}
			}

			finalPath := output
			if !mediaHasExplicitExtension(output) {
				if ext := sniffMediaExtension(tmpPath); ext != "" {
					finalPath = strings.TrimSuffix(output, ".") + ext
				}
				if err := validateOutputPath(finalPath, ""); err != nil {
					return fmt.Errorf("输出路径不安全: %w", err)
				}
				if err := ensureNotOverwriting(finalPath, overwrite); err != nil {
					return err
				}
			}
			if err := os.Rename(tmpPath, finalPath); err != nil {
				return fmt.Errorf("保存文件失败: %w", err)
			}
			_ = os.Chmod(finalPath, 0o644)
			output = finalPath
		}

		fmt.Printf("已下载到 %s\n", output)
		return nil
	},
}

// mediaHasExplicitExtension 判断输出路径是否已带扩展名。
func mediaHasExplicitExtension(path string) bool {
	ext := filepath.Ext(path)
	return ext != "" && ext != "."
}

// ensureNotOverwriting 目标已存在且未传 --overwrite 时报错（此前静默覆盖）。
func ensureNotOverwriting(path string, overwrite bool) error {
	if overwrite {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return clierr.Usagef("输出文件已存在: %s（如需覆盖请加 --overwrite，或用 -o 指定其它路径）", path)
	}
	return nil
}

// sniffMediaExtension 按文件内容识别扩展名；无法识别返回空串。
// Office 文档本质是 zip，通过 [Content_Types].xml 区分 docx/xlsx/pptx。
func sniffMediaExtension(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if n == 0 {
		return ""
	}
	if bytes.HasPrefix(head, []byte("PK\x03\x04")) {
		if zr, err := zip.OpenReader(path); err == nil {
			defer zr.Close()
			for _, zf := range zr.File {
				switch {
				case strings.HasPrefix(zf.Name, "word/"):
					return ".docx"
				case strings.HasPrefix(zf.Name, "xl/"):
					return ".xlsx"
				case strings.HasPrefix(zf.Name, "ppt/"):
					return ".pptx"
				}
			}
		}
		return ".zip"
	}
	ct := http.DetectContentType(head)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	switch ct {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "application/pdf":
		return ".pdf"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "audio/mpeg":
		return ".mp3"
	case "application/x-gzip":
		return ".gz"
	case "text/plain":
		return ".txt"
	case "text/html":
		return ".html"
	case "text/xml":
		return ".xml"
	}
	if bytes.HasPrefix(head, []byte("<svg")) || bytes.Contains(head, []byte("<svg ")) {
		return ".svg"
	}
	return ""
}

// withMediaDownloadHint 为 403 / 限流补充可执行的排查建议。
func withMediaDownloadHint(err error, mediaType string) error {
	lower := strings.ToLower(err.Error())
	switch {
	case client.HasHTTPStatus(err, 403) || strings.Contains(lower, "forbidden") || strings.Contains(lower, "permission denied") || strings.Contains(lower, "no permission"):
		if mediaType == "whiteboard" {
			return fmt.Errorf("%w\n提示：当前身份无权读取该画板；确认画板所属文档已对当前身份开放，或改用有权限的身份（--user-access-token / auth login）", err)
		}
		return fmt.Errorf("%w\n提示：HTTP 403 表示当前身份无权下载该素材。文档内嵌的图片/附件需带 --doc-token <文档 token 或 URL> 按文档鉴权，"+
			"并确认当前身份对该文档有阅读与下载权限（文档可能禁止下载/导出）；必要时用 --user-access-token 或 auth login 以文档协作者身份下载", err)
	case client.IsRateLimitError(err):
		return fmt.Errorf("%w\n提示：下载被限流，请稍后按指数退避重试，不要立即重复请求", err)
	}
	return err
}

func init() {
	docCmd.AddCommand(docMediaDownloadCmd)
	docMediaDownloadCmd.Flags().String("type", "media", "素材类型（media/whiteboard）")
	docMediaDownloadCmd.Flags().StringP("output", "o", "", "输出文件路径")
	docMediaDownloadCmd.Flags().String("doc-token", "", "素材所属文档 token 或 URL（用于下载文档内嵌图片；wiki URL 自动解析）")
	docMediaDownloadCmd.Flags().String("doc-type", "docx", "素材所属文档类型（默认 docx）")
	docMediaDownloadCmd.Flags().String("extra", "", "素材下载 extra JSON（优先于 --doc-token/--doc-type）")
	docMediaDownloadCmd.Flags().String("user-access-token", "", "User Access Token（可选；默认优先使用 auth login 登录态，失败时回退 App Token）")
	docMediaDownloadCmd.Flags().String("timeout", "", "下载超时时间（默认 5m，示例: 10m, 30m, 1h）")
	docMediaDownloadCmd.Flags().Bool("overwrite", false, "目标文件已存在时覆盖（默认拒绝）")
}
