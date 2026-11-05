package cmd

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// drive preview / drive cover：上传文件（type=file）的预览产物与封面（对齐官方 drive +preview / +cover）。

// drivePreviewTypeNames preview_type 代码 → 名称（官方 drive_preview_common.go 码表）。
var drivePreviewTypeNames = map[string]string{
	"0": "pdf", "1": "png", "2": "pages", "3": "video", "4": "mp4_360p", "5": "mp4_480p", "6": "mp4_720p",
	"7": "jpg", "8": "html", "9": "pdf_lin", "10": "xod", "11": "jpg_lin", "12": "png_lin", "13": "archive",
	"14": "text", "15": "pdf_part", "16": "source_file", "17": "video_meta", "18": "wps", "19": "split_png",
	"20": "media_result", "21": "mime", "22": "spilt_img_txt", "23": "mp4_1080p", "24": "image_meta",
	"25": "doc_part", "26": "watermark_pdf", "27": "file_watermark",
}

// drivePreviewImageTypes 是 --type image 的别名命中范围（优先选可下载的）。
var drivePreviewImageTypes = map[string]bool{"png": true, "jpg": true, "jpg_lin": true, "png_lin": true, "split_png": true}

// drivePreviewStatusNames preview_status 代码 → 名称；只有 0（READY）可下载。
var drivePreviewStatusNames = map[string]string{
	"0": "READY", "1": "PROCESSING", "2": "FAILED", "3": "FAILED_NOT_RETRY", "4": "INVALID_EXTENSION",
	"5": "FILE_TOO_LARGE", "6": "EMPTY_FILE", "7": "NO_SUPPORT", "8": "INVALID_PREVIEW_TYPE", "9": "NEED_PASSWORD",
	"10": "FILE_INVALID", "11": "TOO_MANY_PAGES",
}

type drivePreviewCandidate struct {
	Type         string `json:"type"`
	TypeCode     string `json:"type_code"`
	Status       string `json:"status"`
	StatusCode   string `json:"status_code"`
	Downloadable bool   `json:"downloadable"`
}

func normalizeDrivePreviewCandidates(items []map[string]interface{}) []drivePreviewCandidate {
	out := make([]drivePreviewCandidate, 0, len(items))
	for _, it := range items {
		code := jsonScalarString(it["preview_type"])
		status := jsonScalarString(it["preview_status"])
		c := drivePreviewCandidate{TypeCode: code, StatusCode: status}
		if name, ok := drivePreviewTypeNames[code]; ok {
			c.Type = name
		} else {
			c.Type = "unknown_" + code
		}
		if name, ok := drivePreviewStatusNames[status]; ok {
			c.Status = name
		} else {
			c.Status = "UNKNOWN"
		}
		c.Downloadable = status == "0"
		out = append(out, c)
	}
	return out
}

// selectDrivePreviewCandidate 按类型名、类型代码或 image 别名匹配候选。
func selectDrivePreviewCandidate(cands []drivePreviewCandidate, requested string) (drivePreviewCandidate, bool) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	for _, c := range cands {
		if requested == c.Type || requested == c.TypeCode {
			return c, true
		}
	}
	if requested == "image" {
		var first *drivePreviewCandidate
		for i, c := range cands {
			if !drivePreviewImageTypes[c.Type] {
				continue
			}
			if c.Downloadable {
				return c, true
			}
			if first == nil {
				first = &cands[i]
			}
		}
		if first != nil {
			return *first, true
		}
	}
	return drivePreviewCandidate{}, false
}

func jsonScalarString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	}
	return ""
}

// driveFileTarget 解析 drive preview/cover 的位置参数：裸 file token、/file/ 链接或包裹上传文件的 /wiki/ 链接。
// wiki 解包需联网，返回的 resolvedResource.WikiNode 为 nil 时由调用方在身份解析后调用 resolveWikiInResource。
func driveFileTarget(raw string) (*resolvedResource, resourceArgOptions, error) {
	opts := resourceArgOptions{
		ArgName:     "<file_token>",
		DefaultType: client.ResourceTypeFile,
		Allowed:     []string{client.ResourceTypeFile},
		ResolveWiki: true,
	}
	res, err := parseResourceArg(raw, opts)
	if err != nil {
		return nil, opts, clierr.Usage(err)
	}
	return res, opts, nil
}

// driveFileTargetToken 返回 dry-run 中展示的 file token（wiki 链接未解包时用占位符）。
func driveFileTargetToken(res *resolvedResource) string {
	if res.InputType == client.ResourceTypeWiki {
		return "<wiki 节点底层 file token>"
	}
	return res.Token
}

// resolveDriveFileTarget 身份解析之后解包 wiki 链接，返回最终 file token。
func resolveDriveFileTarget(res *resolvedResource, opts resourceArgOptions, token string) (string, error) {
	if res.InputType != client.ResourceTypeWiki {
		return res.Token, nil
	}
	opts.UserAccessToken = token
	if err := resolveWikiInResource(res, opts); err != nil {
		return "", err
	}
	return res.Token, nil
}

var drivePreviewCmd = &cobra.Command{
	Use:   "preview <file_token|url>",
	Short: "列出或下载上传文件的预览产物（PDF / 图片 / HTML / 文本 / 源文件）",
	Long: `列出或下载云盘上传文件（type=file）的服务端预览产物，对齐官方 drive +preview。

流程:
  1. POST /open-apis/drive/v1/medias/:file_token/preview_result 列出候选（类型 + 状态）
  2. 选中 READY 的候选后 GET .../preview_download?preview_type=<代码>&version=<版本> 下载
  --type source_file 直接下载源文件（preview_type=16），不查候选

参数:
  <file_token|url>  上传文件 token、/file/ 链接，或包裹上传文件的 /wiki/ 链接（wiki 在凭据解析后解包）
  --list-only       只列出候选，不下载（与 --type / --output 互斥）
  --type            要下载的类型：pdf | pdf_lin | html | text | image（png/jpg 任一）| source_file，或类型代码
  --version         文件版本（默认最新）
  --output, -o      保存路径（--type 时必填；不带扩展名时按内容补全）；在任何网络请求之前校验
  --overwrite       目标文件已存在时覆盖（默认拒绝）
  --as              bot | user | auto（默认 auto：User 优先；未配置回退 Bot；已配置但不可用 fail-closed）
  --dry-run         只打印请求计划（不读取凭据、不联网）

已知限制（实测）:
  部分文件的 PDF 类候选（pdf / pdf_lin）显示 READY，但 preview_download 返回 HTTP 200 空 body，
  带 version 或稍后重试结果相同（不是生成中，轮询无效）。此时命令报错且不写空文件，
  请改用 --type source_file 下载源文件，或用 drive cover 取封面图。

权限: drive:file:download

示例:
  feishu-cli drive preview boxcnxxx --list-only
  feishu-cli drive preview boxcnxxx --type source_file -o ./report
  feishu-cli drive preview https://example.feishu.cn/file/boxcnxxx --type image -o ./thumb --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		listOnly, _ := cmd.Flags().GetBool("list-only")
		reqType, _ := cmd.Flags().GetString("type")
		version, _ := cmd.Flags().GetString("version")
		output, _ := cmd.Flags().GetString("output")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		reqType = strings.ToLower(strings.TrimSpace(reqType))
		version = strings.TrimSpace(version)
		output = strings.TrimSpace(output)

		res, opts, err := driveFileTarget(args[0])
		if err != nil {
			return err
		}
		if listOnly {
			if reqType != "" {
				return clierr.Usagef("--type 不能与 --list-only 同时使用")
			}
			if output != "" {
				return clierr.Usagef("--output 不能与 --list-only 同时使用")
			}
		} else {
			if reqType == "" {
				return clierr.Usagef("请指定 --list-only 或 --type")
			}
			if output == "" {
				return clierr.Usagef("--type 时 --output 必填")
			}
			if err := checkMediaOutputPath(output, overwrite); err != nil {
				return err
			}
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			fileToken := driveFileTargetToken(res)
			var steps []dryRunStep
			if res.InputType == client.ResourceTypeWiki {
				steps = append(steps, dryRunStep{Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node", Params: map[string]any{"token": res.InputToken}, Desc: "解包 wiki 节点为底层 file token"})
			}
			dlParams := map[string]any{}
			if version != "" {
				dlParams["version"] = version
			}
			if reqType == "source_file" || reqType == "16" {
				dlParams["preview_type"] = client.MediaPreviewTypeSourceFile
				steps = append(steps, dryRunStep{Method: "GET", URL: client.DrivePreviewDownloadPath(fileToken), Params: dlParams, Desc: "下载源文件"})
			} else {
				var body map[string]any
				if version != "" {
					body = map[string]any{"version": version}
				}
				steps = append(steps, dryRunStep{Method: "POST", URL: client.DrivePreviewResultPath(fileToken), Body: body, Desc: "列出预览候选"})
				if !listOnly {
					dlParams["preview_type"] = "<选中候选的 type_code>"
					if version == "" {
						dlParams["version"] = "<preview_result 返回的 version>"
					}
					steps = append(steps, dryRunStep{Method: "GET", URL: client.DrivePreviewDownloadPath(fileToken), Params: dlParams, Desc: "下载选中的预览产物"})
				}
			}
			extra := map[string]any{"file_token": fileToken}
			if listOnly {
				extra["mode"] = "list"
			} else {
				extra["mode"] = "download"
				extra["requested_type"] = reqType
				extra["output"] = output
			}
			return printDryRunJSON("Preview Drive file", extra, steps)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		fileToken, err := resolveDriveFileTarget(res, opts, token)
		if err != nil {
			return err
		}

		if reqType == "source_file" || reqType == "16" {
			return downloadDrivePreview(fileToken, client.MediaPreviewTypeSourceFile, "source_file", version, output, overwrite, token)
		}

		result, err := client.GetDrivePreviewResult(fileToken, version, token)
		if err != nil {
			if client.HasAPICode(err, 1) {
				return fmt.Errorf("%w\n提示：该文件没有可用的预览候选；可改用 --type source_file --output <路径> 下载源文件", err)
			}
			return err
		}
		cands := normalizeDrivePreviewCandidates(result.PreviewResults)
		if listOnly {
			out := map[string]any{"mode": "list", "file_token": fileToken, "candidates": cands}
			if result.Version != "" {
				out["version"] = result.Version
			}
			return printJSON(out)
		}
		cand, ok := selectDrivePreviewCandidate(cands, reqType)
		if !ok {
			names := make([]string, 0, len(cands))
			for _, c := range cands {
				names = append(names, c.Type)
			}
			return fmt.Errorf("文件 %s 没有 %q 类型的预览候选（可用: %s）；可用 --list-only 查看，或用 --type source_file 下载源文件", fileToken, reqType, strings.Join(names, ", "))
		}
		if !cand.Downloadable {
			return fmt.Errorf("预览类型 %s 当前不可下载（status=%s）；稍后用 --list-only 复查", cand.Type, cand.Status)
		}
		if version == "" {
			version = result.Version
		}
		return downloadDrivePreview(fileToken, cand.TypeCode, cand.Type, version, output, overwrite, token)
	},
}

// printDryRunJSON 以与 printDryRunPlan 相同的 JSON 结构输出预览。drive preview/cover 的 -o 是保存路径，
// 不能复用 printDryRunPlan（它把 --output 当输出格式）。
func printDryRunJSON(desc string, extra map[string]any, steps []dryRunStep) error {
	plan := map[string]any{"dry_run": true, "desc": desc, "api": steps}
	for k, v := range extra {
		plan[k] = v
	}
	return printJSON(plan)
}

// downloadDrivePreview 下载 preview_type 指定的产物。服务端返回空 body（Content-Length: 0）时报错且不写文件。
func downloadDrivePreview(fileToken, typeCode, typeName, version, output string, overwrite bool, token string) error {
	params := url.Values{"preview_type": {typeCode}}
	if version != "" {
		params.Set("version", version)
	}
	d, err := client.OpenDrivePreviewDownload(fileToken, params, token, 0)
	if err != nil {
		return withMediaPreviewHint(err)
	}
	defer d.Close()
	if d.Size() == 0 {
		return fmt.Errorf("预览产物 %s 的下载结果为空（服务端返回 HTTP 200 空 body，未写入文件）；"+
			"该现象实测与生成进度无关，重试无效。可改用 --type source_file 下载源文件，或用 drive cover 取封面图", typeName)
	}
	saved, err := saveMediaDownload(d, output, overwrite)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{
		"mode":          "download",
		"file_token":    fileToken,
		"selected_type": typeName,
		"output_path":   saved.Path,
		"size_bytes":    saved.Size,
		"content_type":  saved.ContentType,
	})
}

// driveCoverSpec 封面规格 → preview_download 参数（官方 driveCoverSpecs）。
type driveCoverSpec struct {
	Name, Desc, BusType, Platform, Policy string
	Width, Height                         int
}

var driveCoverSpecs = []driveCoverSpec{
	{Name: "default", Desc: "标准大封面（1280x1280）", BusType: "cover", Platform: "pc"},
	{Name: "icon", Desc: "列表小图标（120x120）", BusType: "icon"},
	{Name: "grid", Desc: "宫格/卡片流封面（360x360）", BusType: "grid"},
	{Name: "small", Desc: "PC 小图（480x480）", BusType: "small_graph", Platform: "pc"},
	{Name: "middle", Desc: "中等封面（720x720）", BusType: "middle"},
	{Name: "big", Desc: "移动端大封面（850x850）", BusType: "big", Platform: "mobile"},
	{Name: "square", Desc: "方形裁剪封面（360x360）", Width: 360, Height: 360, Policy: "near"},
}

func findDriveCoverSpec(name string) (driveCoverSpec, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, s := range driveCoverSpecs {
		if s.Name == name {
			return s, true
		}
	}
	return driveCoverSpec{}, false
}

// driveCoverParams 构造封面下载 query（preview_type=1 + 规格参数）。
func driveCoverParams(spec driveCoverSpec, version string) url.Values {
	p := url.Values{"preview_type": {"1"}}
	if spec.BusType != "" {
		p.Set("bus_type", spec.BusType)
	}
	if spec.Platform != "" {
		p.Set("platform", spec.Platform)
	}
	if spec.Width > 0 {
		p.Set("width", strconv.Itoa(spec.Width))
	}
	if spec.Height > 0 {
		p.Set("height", strconv.Itoa(spec.Height))
	}
	if spec.Policy != "" {
		p.Set("policy", spec.Policy)
	}
	if version != "" {
		p.Set("version", version)
	}
	return p
}

func driveCoverSpecNames() []string {
	out := make([]string, 0, len(driveCoverSpecs))
	for _, s := range driveCoverSpecs {
		out = append(out, s.Name)
	}
	return out
}

var driveCoverCmd = &cobra.Command{
	Use:   "cover <file_token|url>",
	Short: "下载上传文件的封面缩略图（多种规格）",
	Long: `下载云盘上传文件（type=file）的封面缩略图（GET .../medias/:file_token/preview_download?preview_type=1&bus_type=...），
对齐官方 drive +cover。文档（docx）页面封面请用 doc resource download --type cover。

参数:
  <file_token|url>  上传文件 token、/file/ 链接，或包裹上传文件的 /wiki/ 链接
  --spec            封面规格：default | icon | grid | small | middle | big | square
  --list-only       只列出可用规格（本地列表，不联网、不需要凭据）
  --version         文件版本（默认最新）
  --output, -o      保存路径（--spec 时必填；不带扩展名时按内容补全，通常为 .png）；在任何网络请求之前校验
  --overwrite       目标文件已存在时覆盖（默认拒绝）
  --as              bot | user | auto（默认 auto）
  --dry-run         只打印请求（不读取凭据、不联网）

文件没有该规格封面时服务端返回 404（如 csv），命令给出中文提示并以非 0 退出。

权限: drive:file:download

示例:
  feishu-cli drive cover boxcnxxx --list-only
  feishu-cli drive cover boxcnxxx --spec default -o ./cover.png
  feishu-cli drive cover boxcnxxx --spec square -o ./sq --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		listOnly, _ := cmd.Flags().GetBool("list-only")
		specName, _ := cmd.Flags().GetString("spec")
		version, _ := cmd.Flags().GetString("version")
		output, _ := cmd.Flags().GetString("output")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		version = strings.TrimSpace(version)
		output = strings.TrimSpace(output)

		res, opts, err := driveFileTarget(args[0])
		if err != nil {
			return err
		}
		if listOnly {
			if strings.TrimSpace(specName) != "" || output != "" {
				return clierr.Usagef("--list-only 不能与 --spec / --output 同时使用")
			}
			items := make([]map[string]any, 0, len(driveCoverSpecs))
			for _, s := range driveCoverSpecs {
				items = append(items, map[string]any{"spec": s.Name, "description": s.Desc})
			}
			return printJSON(map[string]any{"mode": "list", "file_token": driveFileTargetToken(res), "candidates": items})
		}
		if strings.TrimSpace(specName) == "" {
			return clierr.Usagef("请指定 --list-only 或 --spec（可选: %s）", strings.Join(driveCoverSpecNames(), ", "))
		}
		spec, ok := findDriveCoverSpec(specName)
		if !ok {
			return clierr.Usagef("不支持的 --spec %q，可选: %s", specName, strings.Join(driveCoverSpecNames(), ", "))
		}
		if output == "" {
			return clierr.Usagef("--spec 时 --output 必填")
		}
		if err := checkMediaOutputPath(output, overwrite); err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		params := driveCoverParams(spec, version)

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			fileToken := driveFileTargetToken(res)
			var steps []dryRunStep
			if res.InputType == client.ResourceTypeWiki {
				steps = append(steps, dryRunStep{Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node", Params: map[string]any{"token": res.InputToken}, Desc: "解包 wiki 节点为底层 file token"})
			}
			q := map[string]any{}
			for k, v := range params {
				q[k] = v[0]
			}
			steps = append(steps, dryRunStep{Method: "GET", URL: client.DrivePreviewDownloadPath(fileToken), Params: q, Desc: "下载封面"})
			return printDryRunJSON("Download Drive file cover", map[string]any{"file_token": fileToken, "spec": spec.Name, "output": output}, steps)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		fileToken, err := resolveDriveFileTarget(res, opts, token)
		if err != nil {
			return err
		}
		d, err := client.OpenDrivePreviewDownload(fileToken, params, token, 0)
		if err != nil {
			if client.HasHTTPStatus(err, 404) {
				return fmt.Errorf("%w\n提示：该文件没有 %s 规格的封面（部分类型如 csv 不生成封面）；可换其他 --spec 或用 drive preview --list-only 查看预览候选", err, spec.Name)
			}
			return withMediaPreviewHint(err)
		}
		defer d.Close()
		if d.Size() == 0 {
			return fmt.Errorf("封面 %s 的下载结果为空（服务端返回空 body，未写入文件）", spec.Name)
		}
		saved, err := saveMediaDownload(d, output, overwrite)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{
			"mode":         "download",
			"file_token":   fileToken,
			"spec":         spec.Name,
			"output_path":  saved.Path,
			"size_bytes":   saved.Size,
			"content_type": saved.ContentType,
		})
	},
}

func init() {
	driveCmd.AddCommand(drivePreviewCmd, driveCoverCmd)

	f := drivePreviewCmd.Flags()
	f.Bool("list-only", false, "只列出预览候选，不下载")
	f.String("type", "", "下载类型：pdf | pdf_lin | html | text | image | source_file（或类型代码）")
	f.String("version", "", "文件版本（默认最新）")
	f.StringP("output", "o", "", "保存路径（--type 时必填）")
	f.Bool("overwrite", false, "目标文件已存在时覆盖")
	f.Bool("dry-run", false, "只打印请求计划")
	f.String("user-access-token", "", "User Access Token（覆盖登录态）")
	addAsFlag(drivePreviewCmd)

	f = driveCoverCmd.Flags()
	f.String("spec", "", "封面规格：default | icon | grid | small | middle | big | square")
	f.Bool("list-only", false, "只列出可用规格（不联网）")
	f.String("version", "", "文件版本（默认最新）")
	f.StringP("output", "o", "", "保存路径（--spec 时必填）")
	f.Bool("overwrite", false, "目标文件已存在时覆盖")
	f.Bool("dry-run", false, "只打印请求")
	f.String("user-access-token", "", "User Access Token（覆盖登录态）")
	addAsFlag(driveCoverCmd)
}
