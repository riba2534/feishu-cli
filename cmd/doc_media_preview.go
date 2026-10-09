package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var docMediaPreviewCmd = &cobra.Command{
	Use:   "media-preview <token>",
	Short: "预览文档素材（图片 / 附件 / 评论图片，保存源文件并自动补扩展名）",
	Long: `通过素材预览接口（preview_download，preview_type=16 源文件）把文档素材保存到本地，便于打开查看。
支持文档内的图片、附件素材 token，也支持评论中的图片 token。

与 media-download 的区别：
  media-preview   预览接口，适合"看一下这张图 / 这个附件"，评论图片也能取到
  media-download  下载接口，支持画板缩略图（--type whiteboard）与 --doc-token 文档鉴权

参数:
  token          素材 token（file_token），如 doc export 输出的 <image token="..."/>
  --output, -o   保存路径（默认以 token 为文件名）；不带扩展名时按内容自动补扩展名
  --overwrite    目标文件已存在时覆盖（默认拒绝并以退出码 2 报错）
  --timeout      总时长上限（如 10m；默认不限总时长，空闲 60 秒无数据视为超时）
  --output-format json  以 JSON 输出 saved_path / size_bytes / content_type

身份：读类，User 优先、Bot 兜底（与 media-download 一致）。

示例:
  # 预览图片素材（自动补扩展名，如 ./asset.png）
  feishu-cli doc media-preview boxcnXXX -o ./asset

  # 指定带扩展名的文件名（不再补扩展名）
  feishu-cli doc media-preview boxcnXXX -o ./asset.png --overwrite

  # JSON 输出
  feishu-cli doc media-preview boxcnXXX -o ./asset --output-format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token := args[0]
		output, _ := cmd.Flags().GetString("output")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		timeoutStr, _ := cmd.Flags().GetString("timeout")
		format, _ := cmd.Flags().GetString("output-format")

		if !client.IsSafeResourceToken(token) {
			return clierr.Usagef("素材 token 无效（只允许字母、数字、_ 和 -，长度 1-128）: %q", token)
		}
		if format != "" && format != "json" {
			return clierr.Usagef("--output-format 只支持 json，得到 %q", format)
		}
		timeout, err := parseOptionalTimeout(timeoutStr)
		if err != nil {
			return err
		}
		if output == "" {
			output = safeOutputPath(token, "")
		}
		// 输出路径在任何网络请求（含 token 刷新）之前校验
		if err := checkMediaOutputPath(output, overwrite); err != nil {
			return err
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)
		d, err := client.OpenMediaPreviewDownload(token, userAccessToken, timeout)
		if err != nil {
			return withMediaPreviewHint(err)
		}
		defer d.Close()
		saved, err := saveMediaDownload(d, output, overwrite)
		if err != nil {
			return err
		}
		if format == "json" {
			return printJSON(map[string]any{
				"token":        token,
				"saved_path":   saved.Path,
				"size_bytes":   saved.Size,
				"content_type": saved.ContentType,
			})
		}
		fmt.Printf("已下载到 %s\n", saved.Path)
		return nil
	},
}

// checkMediaOutputPath 联网前校验输出路径：敏感目录拒绝、不能是目录；带扩展名时提前做覆盖检查。
func checkMediaOutputPath(output string, overwrite bool) error {
	if err := validateOutputPath(output, ""); err != nil {
		return fmt.Errorf("输出路径不安全: %w", err)
	}
	if st, err := os.Stat(output); err == nil && st.IsDir() {
		return clierr.Usagef("输出路径 %s 是目录，请指定文件路径（如 %s/asset）", output, output)
	}
	if mediaHasExplicitExtension(output) {
		return ensureNotOverwriting(output, overwrite)
	}
	return nil
}

// parseOptionalTimeout 解析可选的 --timeout；空串表示不限总时长。
func parseOptionalTimeout(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, clierr.Usagef("无效的 --timeout %q（示例: 10m, 1h）", s)
	}
	return d, nil
}

// withMediaPreviewHint 为 403 / 限流补充排查建议。
func withMediaPreviewHint(err error) error {
	switch {
	case client.HasHTTPStatus(err, 403) || client.HasAPICode(err, 1061004):
		return fmt.Errorf("%w\n提示：当前身份无权预览该素材；确认对素材所属文档有阅读权限，必要时用 --user-access-token 或 auth login 以文档协作者身份访问", err)
	case client.IsRateLimitError(err):
		return fmt.Errorf("%w\n提示：请求被限流，请稍后按指数退避重试", err)
	}
	return err
}

func init() {
	docCmd.AddCommand(docMediaPreviewCmd)
	docMediaPreviewCmd.Flags().StringP("output", "o", "", "保存路径（默认以 token 为文件名；不带扩展名时自动补）")
	docMediaPreviewCmd.Flags().Bool("overwrite", false, "目标文件已存在时覆盖（默认拒绝）")
	docMediaPreviewCmd.Flags().String("timeout", "", "总时长上限（如 10m；默认不限，仅空闲超时）")
	docMediaPreviewCmd.Flags().String("output-format", "", "输出格式（json）")
	docMediaPreviewCmd.Flags().String("user-access-token", "", "User Access Token（可选；默认优先使用 auth login 登录态，失败时回退 App Token）")
}
