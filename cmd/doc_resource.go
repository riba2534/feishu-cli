package cmd

import (
	"fmt"
	"math"
	"path/filepath"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/clipboard"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

const (
	docResourceTypeCover     = "cover"
	docCoverUploadParentType = "docx_image"
)

// readClipboardImage 读取剪贴板图片；测试中替换为注入的字节。
var readClipboardImage = clipboard.ReadImage

var docResourceCmd = &cobra.Command{
	Use:   "resource",
	Short: "文档资源：下载 / 更新 / 删除封面图（--type cover）",
	Long: `管理 docx 文档的资源，当前支持文档封面图（--type cover，默认值）。

封面图不是正文里的图片块：下载、设置、删除封面用本组命令，不要用 media-insert / media-download 拼步骤。

子命令:
  download  下载封面图到本地（读类：User 优先、Bot 兜底）
  update    用本地文件 / HTTPS 图片 / 剪贴板图片设置封面（写类：默认 Bot 身份）
  delete    删除封面（写类：默认 Bot 身份；文档本来没有封面时也成功返回）

文档参数接受 document_id、/docx/ URL，以及底层为 docx 的 /wiki/ URL（自动解析）。
写类命令默认 Bot 身份；Bot 不是文档协作者时用 --user-access-token 或 FEISHU_USER_ACCESS_TOKEN 以本人身份执行。

示例:
  feishu-cli doc resource download DOC_ID -o ./cover
  feishu-cli doc resource update DOC_ID --file ./cover.png
  feishu-cli doc resource update DOC_ID --url https://example.com/cover.png --offset-ratio-y 0.2
  feishu-cli doc resource update DOC_ID --from-clipboard
  feishu-cli doc resource delete DOC_ID`,
}

var docResourceDownloadCmd = &cobra.Command{
	Use:   "download <document_id|url>",
	Short: "下载文档封面图",
	Long: `读取文档的 cover.token 后下载封面图片到本地。

参数:
  document_id      文档 ID 或 URL（/docx/、/wiki/）
  --type           资源类型，当前只支持 cover（默认）
  --output, -o     保存路径（必填）；不带扩展名时按内容自动补扩展名（规则同 media-download）
  --overwrite      目标文件已存在时覆盖（默认拒绝并以退出码 2 报错）
  --timeout        总时长上限（如 10m；默认不限，仅空闲超时）
  --output-format  json：输出 document_id / type / saved_path / size_bytes / content_type / cover
  --dry-run        只打印将发出的请求，不联网、不解析身份

文档没有封面时以退出码 2 报错，不会创建输出文件。

示例:
  feishu-cli doc resource download DOC_ID -o ./cover
  feishu-cli doc resource download https://xxx.feishu.cn/wiki/wikcnXXX -o ./cover.png --overwrite --output-format json`,
	Args: cobra.ExactArgs(1),
	RunE: runDocResourceDownload,
}

var docResourceUpdateCmd = &cobra.Command{
	Use:   "update <document_id|url>",
	Short: "设置 / 替换文档封面图",
	Long: `上传图片并设置为文档封面（上传 docx_image 素材 → PATCH update_cover）。

图片来源三选一:
  --file <path>       本地图片；超过 20MB 自动分片上传
  --url <https-url>   下载公开 HTTPS 图片后上传：仅 HTTPS、拒绝内网/回环地址、最多 3 次跳转、
                      只接受 png/jpeg/gif/webp/bmp/tiff、最大 20MiB
  --from-clipboard    读取系统剪贴板图片（全程内存、不落临时文件；macOS/Windows 内置，
                      Linux 需要 xclip / wl-paste / xsel 之一）

可选:
  --offset-ratio-x    视图相对原图中心的横向偏移比例（水平偏移 px / 原图宽度 px）；0 居中，正数向右
  --offset-ratio-y    视图相对原图中心的纵向偏移比例（垂直偏移 px / 原图高度 px）；0 居中，正数向上
  --type              资源类型，当前只支持 cover（默认）
  --dry-run           只打印将发出的请求，不联网、不读取剪贴板、不解析身份
  --output, -o        输出格式（json）

身份：写类，默认 Bot；Bot 不是文档协作者时用 --user-access-token 或 FEISHU_USER_ACCESS_TOKEN。

示例:
  feishu-cli doc resource update DOC_ID --file ./cover.png
  feishu-cli doc resource update DOC_ID --url "https://example.com/cover.png"
  feishu-cli doc resource update DOC_ID --from-clipboard --offset-ratio-x 0.2 --offset-ratio-y -0.1`,
	Args: cobra.ExactArgs(1),
	RunE: runDocResourceUpdate,
}

var docResourceDeleteCmd = &cobra.Command{
	Use:   "delete <document_id|url>",
	Short: "删除文档封面图",
	Long: `删除文档封面（PATCH update_cover，cover 置为 null）。文档本来没有封面时不发起修改、直接成功
（输出 deleted=false、already_empty=true），可安全重复执行。

参数:
  document_id  文档 ID 或 URL（/docx/、/wiki/）
  --type       资源类型，当前只支持 cover（默认）
  --dry-run    只打印将发出的请求，不联网、不解析身份
  --output, -o 输出格式（json）

身份：写类，默认 Bot；Bot 不是文档协作者时用 --user-access-token 或 FEISHU_USER_ACCESS_TOKEN。

示例:
  feishu-cli doc resource delete DOC_ID
  feishu-cli doc resource delete DOC_ID -o json`,
	Args: cobra.ExactArgs(1),
	RunE: runDocResourceDelete,
}

// docResourceTarget 离线解析文档参数并校验 --type；dry-run 用 docID 展示（wiki 时为占位符）。
type docResourceTarget struct {
	raw    string
	docID  string
	isWiki bool
	wikiID string
}

func parseDocResourceTarget(cmd *cobra.Command, raw string) (*docResourceTarget, error) {
	resType, _ := cmd.Flags().GetString("type")
	if resType != docResourceTypeCover {
		return nil, clierr.Usagef("不支持的 --type %q，当前只支持 cover", resType)
	}
	res, err := parseResourceArg(raw, resourceArgOptions{
		ArgName:     "<document_id|url>",
		DefaultType: client.ResourceTypeDocx,
		Allowed:     []string{client.ResourceTypeDocx},
		ResolveWiki: true,
	})
	if err != nil {
		return nil, clierr.Usage(err)
	}
	t := &docResourceTarget{raw: raw, docID: res.Token}
	if res.InputType == client.ResourceTypeWiki {
		t.isWiki, t.wikiID, t.docID = true, res.InputToken, "<wiki 解析后的 docx token>"
	}
	return t, nil
}

// dryRunSteps 在计划前补 wiki 解析步骤。
func (t *docResourceTarget) dryRunSteps(steps ...dryRunStep) []dryRunStep {
	if !t.isWiki {
		return steps
	}
	wiki := dryRunStep{
		Method: "GET",
		URL:    client.WikiNodeByTokenPath,
		Desc:   "解析 wiki 节点为底层 docx 文档",
		Params: map[string]any{"token": t.wikiID},
	}
	return append([]dryRunStep{wiki}, steps...)
}

func docPath(docID string) string { return "/open-apis/docx/v1/documents/" + docID }

func runDocResourceDownload(cmd *cobra.Command, args []string) error {
	if err := config.Validate(); err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	overwrite, _ := cmd.Flags().GetBool("overwrite")
	timeoutStr, _ := cmd.Flags().GetString("timeout")
	format, _ := cmd.Flags().GetString("output-format")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	target, err := parseDocResourceTarget(cmd, args[0])
	if err != nil {
		return err
	}
	if output == "" {
		return clierr.Usagef("--output 必填：指定封面图保存路径（不带扩展名时自动补）")
	}
	if format != "" && format != "json" {
		return clierr.Usagef("--output-format 只支持 json，得到 %q", format)
	}
	timeout, err := parseOptionalTimeout(timeoutStr)
	if err != nil {
		return err
	}
	if err := checkMediaOutputPath(output, overwrite); err != nil {
		return err
	}
	if dryRun {
		return printJSON(map[string]any{
			"dry_run":     true,
			"desc":        "读取文档封面元数据 → 下载封面图片",
			"document_id": target.docID,
			"output":      output,
			"api": target.dryRunSteps(
				dryRunStep{Method: "GET", URL: docPath(target.docID), Desc: "读取文档封面元数据（document.cover）"},
				dryRunStep{Method: "GET", URL: "/open-apis/drive/v1/medias/<cover.token>/download", Desc: "下载封面图片"},
			),
		})
	}

	userAccessToken := resolveOptionalUserTokenWithFallback(cmd)
	documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
	if err != nil {
		return err
	}
	cover, err := client.GetDocumentCover(documentID, userAccessToken)
	if err != nil {
		return err
	}
	if cover.Token == "" {
		return clierr.Usagef("文档 %s 没有封面（cover 为空），未创建输出文件", documentID)
	}
	d, err := client.OpenMediaDownload(cover.Token, userAccessToken, timeout)
	if err != nil {
		return withMediaPreviewHint(fmt.Errorf("下载封面图片失败: %w", err))
	}
	defer d.Close()
	saved, err := saveMediaDownload(d, output, overwrite)
	if err != nil {
		return err
	}
	if format == "json" {
		return printJSON(map[string]any{
			"document_id":  documentID,
			"type":         docResourceTypeCover,
			"saved_path":   saved.Path,
			"size_bytes":   saved.Size,
			"content_type": saved.ContentType,
			"cover":        cover,
		})
	}
	fmt.Printf("已下载到 %s\n", saved.Path)
	return nil
}

// docCoverSource 是封面图片来源（三选一）。
type docCoverSource struct {
	kind     string // file / url / clipboard
	filePath string
	data     []byte
	fileName string
}

func runDocResourceUpdate(cmd *cobra.Command, args []string) error {
	if err := config.Validate(); err != nil {
		return err
	}
	filePath, _ := cmd.Flags().GetString("file")
	rawURL, _ := cmd.Flags().GetString("url")
	fromClipboard, _ := cmd.Flags().GetBool("from-clipboard")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	output, _ := cmd.Flags().GetString("output")

	target, err := parseDocResourceTarget(cmd, args[0])
	if err != nil {
		return err
	}
	sources := 0
	for _, set := range []bool{filePath != "", rawURL != "", fromClipboard} {
		if set {
			sources++
		}
	}
	if sources == 0 {
		return clierr.Usagef("必须指定图片来源：--file、--url、--from-clipboard 三选一")
	}
	if sources > 1 {
		return clierr.Usagef("--file、--url、--from-clipboard 只能指定一个")
	}
	cover := &client.DocumentCover{}
	for _, name := range []string{"offset-ratio-x", "offset-ratio-y"} {
		if !cmd.Flags().Changed(name) {
			continue
		}
		v, _ := cmd.Flags().GetFloat64(name)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return clierr.Usagef("--%s 必须是有限数值", name)
		}
		if name == "offset-ratio-x" {
			cover.OffsetRatioX = &v
		} else {
			cover.OffsetRatioY = &v
		}
	}
	var src docCoverSource
	switch {
	case filePath != "":
		// 敏感目录、不存在、是目录、无权限读取均为用法错误（先于任何网络请求）
		st, statErr := safefile.StatInputFile(filePath)
		if statErr != nil {
			return statErr
		}
		if !st.Mode().IsRegular() || st.Size() == 0 {
			return clierr.Usagef("--file %s 不是非空的普通文件", filePath)
		}
		src = docCoverSource{kind: "file", filePath: filePath, fileName: filepath.Base(filePath)}
	case rawURL != "":
		if err := client.ValidateCoverImageURL(rawURL); err != nil {
			return err
		}
		src = docCoverSource{kind: "url"}
	default:
		src = docCoverSource{kind: "clipboard", fileName: "clipboard.png"}
	}

	if dryRun {
		source := map[string]string{"file": "@" + filePath, "url": rawURL, "clipboard": "<剪贴板图片>"}[src.kind]
		extra := map[string]any{"document_id": target.docID, "source": src.kind}
		if src.kind == "url" {
			extra["url_safety"] = client.CoverURLSafetyNote
		}
		coverBody := map[string]any{"token": "<file_token>"}
		if cover.OffsetRatioX != nil {
			coverBody["offset_ratio_x"] = *cover.OffsetRatioX
		}
		if cover.OffsetRatioY != nil {
			coverBody["offset_ratio_y"] = *cover.OffsetRatioY
		}
		return printDryRunPlan(cmd, "上传封面图片 → 设置文档封面", extra, target.dryRunSteps(
			dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_all", Desc: "上传封面图片（超过 20MB 时改走 upload_prepare/upload_part/upload_finish）",
				Body: map[string]any{
					"file":        source,
					"file_name":   "<封面文件名>",
					"parent_type": docCoverUploadParentType,
					"parent_node": target.docID,
					"extra":       fmt.Sprintf(`{"drive_route_token":"%s"}`, target.docID),
				}},
			dryRunStep{Method: "PATCH", URL: docPath(target.docID), Desc: "设置文档封面",
				Body: map[string]any{"update_cover": map[string]any{"cover": coverBody}}},
		))
	}

	// 剪贴板在联网前读取：读不到图片时直接以退出码 2 结束，不发任何请求
	if src.kind == "clipboard" {
		data, err := readClipboardImage()
		if err != nil {
			return err
		}
		src.data = data
	}
	userAccessToken := resolveOptionalUserToken(cmd)
	documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
	if err != nil {
		return err
	}
	if src.kind == "url" {
		img, err := client.FetchCoverImageURL(rawURL)
		if err != nil {
			return err
		}
		src.data, src.fileName = img.Data, img.FileName
	}

	var fileToken string
	if src.kind == "file" {
		fileToken, err = client.UploadDocMedia(src.filePath, docCoverUploadParentType, documentID, src.fileName, documentID, userAccessToken)
	} else {
		fileToken, err = client.UploadDocMediaBytes(src.data, docCoverUploadParentType, documentID, src.fileName, documentID, userAccessToken)
	}
	if err != nil {
		return fmt.Errorf("上传封面图片失败: %w", err)
	}
	cover.Token = fileToken
	if err := client.UpdateDocumentCover(documentID, cover, userAccessToken); err != nil {
		return fmt.Errorf("%w\n提示：封面图片已上传（file_token=%s）但设置封面失败；排查后可复用该素材而不必重新上传："+
			"feishu-cli api PATCH /open-apis/docx/v1/documents/%s --data '{\"update_cover\":{\"cover\":{\"token\":\"%s\"}}}'",
			err, fileToken, documentID, fileToken)
	}

	if output == "json" {
		return printJSON(map[string]any{
			"document_id": documentID,
			"type":        docResourceTypeCover,
			"updated":     true,
			"source":      src.kind,
			"file_token":  fileToken,
			"cover":       cover,
		})
	}
	fmt.Printf("封面已更新！\n")
	fmt.Printf("  文档 ID:    %s\n", documentID)
	fmt.Printf("  来源:       %s\n", src.kind)
	fmt.Printf("  文件 Token: %s\n", fileToken)
	return nil
}

func runDocResourceDelete(cmd *cobra.Command, args []string) error {
	if err := config.Validate(); err != nil {
		return err
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	output, _ := cmd.Flags().GetString("output")
	target, err := parseDocResourceTarget(cmd, args[0])
	if err != nil {
		return err
	}
	if dryRun {
		return printDryRunPlan(cmd, "读取封面 → 有封面时删除（没有封面时不发起修改）", map[string]any{"document_id": target.docID}, target.dryRunSteps(
			dryRunStep{Method: "GET", URL: docPath(target.docID), Desc: "读取文档封面元数据（判断是否已为空）"},
			dryRunStep{Method: "PATCH", URL: docPath(target.docID), Desc: "删除文档封面（仅在存在封面时发送）",
				Body: map[string]any{"update_cover": map[string]any{"cover": nil}}},
		))
	}

	userAccessToken := resolveOptionalUserToken(cmd)
	documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
	if err != nil {
		return err
	}
	cover, err := client.GetDocumentCover(documentID, userAccessToken)
	if err != nil {
		return err
	}
	result := map[string]any{
		"document_id":   documentID,
		"type":          docResourceTypeCover,
		"deleted":       false,
		"already_empty": true,
	}
	if cover.Token != "" {
		if err := client.UpdateDocumentCover(documentID, nil, userAccessToken); err != nil {
			return err
		}
		result["deleted"], result["already_empty"], result["previous_cover"] = true, false, cover
	}
	if output == "json" {
		return printJSON(result)
	}
	if cover.Token == "" {
		fmt.Printf("文档 %s 本来没有封面，无需删除\n", documentID)
		return nil
	}
	fmt.Printf("已删除文档 %s 的封面（原封面 token: %s）\n", documentID, cover.Token)
	return nil
}

func init() {
	docCmd.AddCommand(docResourceCmd)
	docResourceCmd.AddCommand(docResourceDownloadCmd, docResourceUpdateCmd, docResourceDeleteCmd)

	for _, c := range []*cobra.Command{docResourceDownloadCmd, docResourceUpdateCmd, docResourceDeleteCmd} {
		c.Flags().String("type", docResourceTypeCover, "资源类型（当前只支持 cover）")
		c.Flags().Bool("dry-run", false, "只打印将要发出的请求，不执行")
	}

	docResourceDownloadCmd.Flags().StringP("output", "o", "", "封面图保存路径（必填；不带扩展名时自动补）")
	docResourceDownloadCmd.Flags().Bool("overwrite", false, "目标文件已存在时覆盖（默认拒绝）")
	docResourceDownloadCmd.Flags().String("timeout", "", "总时长上限（如 10m；默认不限，仅空闲超时）")
	docResourceDownloadCmd.Flags().String("output-format", "", "输出格式（json）")
	docResourceDownloadCmd.Flags().String("user-access-token", "", "User Access Token（可选；默认优先使用 auth login 登录态，失败时回退 App Token）")

	docResourceUpdateCmd.Flags().String("file", "", "本地图片路径（超过 20MB 自动分片上传）")
	docResourceUpdateCmd.Flags().String("url", "", "HTTPS 图片链接（下载后上传）")
	docResourceUpdateCmd.Flags().Bool("from-clipboard", false, "从系统剪贴板读取图片（Linux 需要 xclip / wl-paste / xsel）")
	docResourceUpdateCmd.Flags().Float64("offset-ratio-x", 0, "封面横向偏移比例（0 居中，正数向右）")
	docResourceUpdateCmd.Flags().Float64("offset-ratio-y", 0, "封面纵向偏移比例（0 居中，正数向上）")
	docResourceUpdateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	docResourceUpdateCmd.Flags().String("user-access-token", "", "User Access Token（可选；默认 Bot 身份）")

	docResourceDeleteCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	docResourceDeleteCmd.Flags().String("user-access-token", "", "User Access Token（可选；默认 Bot 身份）")
}
