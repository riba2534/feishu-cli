package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// ==================== record 附件上传 / 下载 / 移除 ====================
// 端点 ground truth（lark-cli base +record-upload/download/remove-attachment --dry-run 印证）：
//
// upload（3 步编排）:
//   [1] GET  /open-apis/base/v3/bases/{bt}/tables/{tid}/fields/{fid}        校验是附件字段（本实现略过，仅做核心 2 步）
//   [2] POST /open-apis/drive/v1/medias/upload_all                          parent_type=bitable_file, parent_node={bt}
//   [3] POST /open-apis/base/v3/bases/{bt}/tables/{tid}/append_attachments  body {"attachments":{rec:{fld:[{file_token}]}}}
//
// download（2 步编排，lark base +record-download-attachment --dry-run 印证）:
//   [1] POST /open-apis/base/v3/bases/{bt}/tables/{tid}/get_attachments     body {"record_id_list":[rec]}  （总是先走，拿每个附件的 extra_info）
//   [2] GET  /open-apis/drive/v1/medias/{file_token}/download?extra=<extra_info>  逐个下载（带上附件元数据里的 extra_info）
//
// 注意：Base 附件下载必须带 get_attachments 返回的 extra_info（否则可能 403）。即使指定了 --file-token
// 也先走 get_attachments 拿元数据，再用 file-token 过滤要下载哪些（与 lark-cli 行为一致）。
//
// remove（单步）:
//   POST /open-apis/base/v3/bases/{bt}/tables/{tid}/remove_attachments      body {"attachments":{rec:{fld:[{file_token}]}}}
//
// 注意：附件 cell 的 attachments body 结构是嵌套 map record_id -> field_id -> [{file_token}]。
// upload/download 涉及真实文件 I/O，dry-run 仅打印各步请求描述符，不读写文件、不发请求。

// bitableAttachmentCellBody 构造 append/remove 的 attachments 嵌套 body。
func bitableAttachmentCellBody(recordID, fieldID string, fileTokens []string) map[string]any {
	items := make([]map[string]any, 0, len(fileTokens))
	for _, tk := range fileTokens {
		items = append(items, map[string]any{"file_token": tk})
	}
	return map[string]any{
		"attachments": map[string]any{
			recordID: map[string]any{
				fieldID: items,
			},
		},
	}
}

// renderAttachmentDryRun 打印多步编排的 dry-run 预览。
// dry-run 也尊重 --format/--jq（与 bitableRun / runFormShareToken 一致）。
// 但 download-attachment 的 --output 是「附件下载保存路径」而非结果输出文件，ParseOptions 会把它读进
// OutputFile，若不清空，dry-run 预览会被写进用户的下载目标文件（覆盖）或在 --output 是目录时报错。
// dry-run 预览本就只该打到 stdout，故这里强制 OutputFile 为空。
func renderAttachmentDryRun(cmd *cobra.Command, desc string, steps []map[string]any, extra map[string]any) error {
	o, err := output.ParseOptions(cmd)
	if err != nil {
		return err
	}
	o.OutputFile = "" // dry-run 预览只打 stdout，绝不写 --output（对 download-attachment 的下载路径语义尤其关键）
	payload := map[string]any{
		"description": desc,
		"api":         steps,
	}
	for k, v := range extra {
		payload[k] = v
	}
	return output.Render(o, payload)
}

var bitableRecordUploadAttachmentCmd = &cobra.Command{
	Use:   "upload-attachment",
	Short: "上传本地文件并追加到记录的附件单元格",
	Long: `上传一个或多个本地文件，把返回的 file_token 追加到记录的附件字段单元格。

编排（2 步核心）:
  [1] POST /open-apis/drive/v1/medias/upload_all  上传到 Base（parent_type=bitable_file）
  [2] POST /open-apis/base/v3/bases/{bt}/tables/{tid}/append_attachments  追加 file_token 到单元格

必填:
  --base-token   多维表格 base_token
  --table-id     目标数据表
  --record-id    目标记录
  --field-id     附件字段 ID
  --file         本地文件路径（可重复，追加多个附件到同一单元格）

示例:
  feishu-cli bitable record upload-attachment --base-token <bt> --table-id <tid> \
    --record-id <rec> --field-id <fld> --file ./report.pdf --file ./shot.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		baseToken, err := resolveBaseToken(cmd)
		if err != nil {
			return err
		}
		tableID, _ := cmd.Flags().GetString("table-id")
		recordID, _ := cmd.Flags().GetString("record-id")
		fieldID, _ := cmd.Flags().GetString("field-id")
		files, _ := cmd.Flags().GetStringArray("file")
		if tableID == "" || recordID == "" || fieldID == "" {
			return fmt.Errorf("--table-id / --record-id / --field-id 必填")
		}
		if len(files) == 0 {
			return fmt.Errorf("--file 必填（可重复）")
		}
		if len(files) > 50 {
			return fmt.Errorf("单次最多 50 个文件，当前 %d 个", len(files))
		}

		// 注意：append_attachments 是 table 级端点（.../tables/{tid}/append_attachments），
		// 不在 records 子路径下。
		appendPath := client.BaseV3Path("bases", baseToken, "tables", tableID, "append_attachments")

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			steps := []map[string]any{
				{
					"desc":   "[1] 上传本地文件到 Base 作为附件媒体（multipart/form-data）",
					"method": "POST",
					"url":    "/open-apis/drive/v1/medias/upload_all",
					"body": map[string]any{
						"parent_type": "bitable_file",
						"parent_node": baseToken,
						"files":       files,
					},
				},
				{
					"desc":   "[2] 把上传得到的 file_token 追加到附件单元格",
					"method": "POST",
					"url":    appendPath,
					"body":   bitableAttachmentCellBody(recordID, fieldID, []string{"<uploaded_file_token>"}),
				},
			}
			return renderAttachmentDryRun(
				cmd,
				"2 步编排：上传本地文件到 Base → 追加 file_token 到附件单元格",
				steps,
				map[string]any{"base_token": baseToken, "table_id": tableID, "record_id": recordID, "field_id": fieldID},
			)
		}

		token, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return err
		}

		// [1] 逐个上传，收集 file_token
		var fileTokens []string
		for _, fp := range files {
			stat, serr := os.Stat(fp)
			if serr != nil {
				return fmt.Errorf("读取文件失败 %s: %w", fp, serr)
			}
			if stat.IsDir() {
				return fmt.Errorf("--file 必须指向文件，不是目录: %s", fp)
			}
			fmt.Fprintf(os.Stderr, "上传: %s (%d bytes)\n", filepath.Base(fp), stat.Size())
			ft, _, uerr := client.UploadMedia(fp, "bitable_file", baseToken, filepath.Base(fp), token)
			if uerr != nil {
				return fmt.Errorf("上传 %s 失败: %w", fp, uerr)
			}
			fileTokens = append(fileTokens, ft)
		}

		// [2] 追加到单元格
		body := bitableAttachmentCellBody(recordID, fieldID, fileTokens)
		data, err := client.BaseV3Call("POST", appendPath, nil, body, token)
		if err != nil {
			return err
		}
		return renderBitableResult(cmd, data)
	},
}

var bitableRecordDownloadAttachmentCmd = &cobra.Command{
	Use:   "download-attachment",
	Short: "下载记录的附件（按 record-id，可选 file-token 过滤）",
	Long: `下载记录附件到本地。提供 --file-token 直接下载指定附件；省略则先读取记录附件元数据再下载全部。

编排:
  [1] POST /open-apis/base/v3/bases/{bt}/tables/{tid}/get_attachments  读取附件元数据（省略 --file-token 时）
  [2] GET  /open-apis/drive/v1/medias/{file_token}/download            逐个下载

必填:
  --base-token   多维表格 base_token
  --table-id     目标数据表
  --record-id    目标记录

可选:
  --file-token   附件 file_token（可重复）；省略则下载该记录所有附件
  --output       保存路径；单个 file_token 时可为文件路径，多个或省略时必须是已存在目录
  --overwrite    已存在时覆盖

示例:
  feishu-cli bitable record download-attachment --base-token <bt> --table-id <tid> \
    --record-id <rec> --file-token <ft> --output ./downloads/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		baseToken, err := resolveBaseToken(cmd)
		if err != nil {
			return err
		}
		tableID, _ := cmd.Flags().GetString("table-id")
		recordID, _ := cmd.Flags().GetString("record-id")
		fileTokens, _ := cmd.Flags().GetStringArray("file-token")
		outputPath, _ := cmd.Flags().GetString("output")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		if tableID == "" || recordID == "" {
			return fmt.Errorf("--table-id / --record-id 必填")
		}

		// get_attachments 同为 table 级端点（不在 records 子路径下）。
		getAttachmentsPath := client.BaseV3Path("bases", baseToken, "tables", tableID, "get_attachments")

		// 输出目录校验：多个或省略 file-token 时 --output 必须是已存在目录
		multi := len(fileTokens) != 1
		if multi && outputPath != "" {
			if stat, serr := os.Stat(outputPath); serr != nil || !stat.IsDir() {
				return fmt.Errorf("下载多个附件或省略 --file-token 时 --output 必须是已存在目录")
			}
		}

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			steps := []map[string]any{
				{
					"desc":   "[1] 读取记录附件元数据（拿每个附件的 extra_info；--file-token 时仍先读以取 extra）",
					"method": "POST",
					"url":    getAttachmentsPath,
					"body":   map[string]any{"record_id_list": []string{recordID}},
				},
				{
					"desc":   "[2] 通过 Base 附件流逐个下载（带上元数据返回的 extra_info）",
					"method": "GET",
					"url":    "/open-apis/drive/v1/medias/<file_token>/download",
					"params": map[string]any{"extra": "<extra_info_if_present>"},
				},
			}
			return renderAttachmentDryRun(
				cmd,
				"2 步编排：读取记录附件元数据（取 extra_info）→ 带 extra 逐个下载请求的附件文件",
				steps,
				map[string]any{"base_token": baseToken, "table_id": tableID, "record_id": recordID, "file_tokens": fileTokens, "output": outputPath},
			)
		}

		token, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return err
		}

		// [1] 总是先读元数据，拿每个附件的 extra_info（Base 附件下载需要带 extra，否则可能 403）。
		body := map[string]any{"record_id_list": []string{recordID}}
		data, derr := client.BaseV3Call("POST", getAttachmentsPath, nil, body, token)
		if derr != nil {
			return derr
		}
		metas := extractAttachmentMetas(data)
		if len(metas) == 0 {
			return fmt.Errorf("记录 %s 没有可下载的附件", recordID)
		}

		// 用 --file-token 过滤要下载哪些；省略则全下。
		selected := selectAttachmentMetas(metas, fileTokens)
		if len(selected) == 0 {
			return fmt.Errorf("记录 %s 中找不到指定的 --file-token 附件", recordID)
		}
		// 多个 --file-token 时部分缺失不再静默成功：对每个未匹配 token 打 stderr 警告，
		// 继续下载已匹配到的（不 abort）。
		for _, tk := range missingFileTokens(fileTokens, selected) {
			fmt.Fprintf(os.Stderr, "警告: file-token %s 在记录 %s 中未找到，跳过\n", tk, recordID)
		}

		// [2] 逐个下载，带上元数据返回的 extra_info。
		downloadedTokens := make([]string, 0, len(selected))
		saved := make([]string, 0, len(selected))
		usedPaths := make(map[string]bool) // 本次下载已占用的路径，用于同名附件去重
		for _, m := range selected {
			ft := m.FileToken
			downloadedTokens = append(downloadedTokens, ft)
			// 文件名优先用附件原始名（get_attachments 返回的 name），回退 file_token；
			// safeOutputPath 去掉路径分隔符/特殊字符，避免目录逃逸。
			name := safeOutputPath(strings.TrimSpace(m.Name), "")
			if name == "" {
				name = ft
			}
			finalPath := outputPath
			if finalPath == "" {
				finalPath = name
			} else if stat, serr := os.Stat(finalPath); serr == nil && stat.IsDir() {
				finalPath = filepath.Join(finalPath, name)
			}
			// 同一目录内多个同名附件（或与本次已下载文件重名）时，加 file_token 前缀保唯一，避免互相覆盖。
			if usedPaths[finalPath] {
				finalPath = filepath.Join(filepath.Dir(finalPath), ft+"_"+filepath.Base(finalPath))
			}
			usedPaths[finalPath] = true
			if _, serr := os.Stat(finalPath); serr == nil && !overwrite {
				return fmt.Errorf("文件已存在: %s（用 --overwrite 覆盖）", finalPath)
			}
			fmt.Fprintf(os.Stderr, "下载附件: %s -> %s\n", ft, finalPath)
			if derr := client.DownloadMedia(ft, finalPath, client.DownloadMediaOptions{UserAccessToken: token, Extra: m.ExtraInfo}); derr != nil {
				return fmt.Errorf("下载 %s 失败: %w", ft, derr)
			}
			saved = append(saved, finalPath)
		}

		// download 的 --output 是附件保存路径（下载目标），不能被结果渲染器当成结果输出文件占用，
		// 否则结果 JSON 会覆盖刚下载的附件。故结果直接 printJSON 到 stdout。
		return printJSON(map[string]any{
			"record_id":   recordID,
			"file_tokens": downloadedTokens,
			"saved_paths": saved,
		})
	},
}

// attachmentMeta 单个附件的下载元数据（file_token + extra_info + 原始文件名）。
type attachmentMeta struct {
	FileToken string
	ExtraInfo string
	Name      string // 附件原始文件名（get_attachments 返回），下载时优先用作本地文件名
}

// extractAttachmentMetas 从 get_attachments 返回结构中提取全部附件元数据（file_token + extra_info + name）。
// 真实响应形如 {"attachments":{rec_id:{field_id:[{"file_token":"...","name":"...","extra_info":"...","size":N}]}}}，
// 这里用递归遍历容错任意嵌套：凡遇到含 file_token 的对象即取其同级 name/extra_info。
// extra_info 是 Base 附件下载需要带上的 extra 查询参数（lark base +record-download-attachment 印证），
// 同节点上与 file_token 并列；缺失则为空（下载时不带 extra）。
func extractAttachmentMetas(data map[string]any) []attachmentMeta {
	var metas []attachmentMeta
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if ft, ok := t["file_token"].(string); ok && ft != "" {
				extra, _ := t["extra_info"].(string)
				name, _ := t["name"].(string)
				metas = append(metas, attachmentMeta{FileToken: ft, ExtraInfo: extra, Name: name})
			}
			for _, child := range t {
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(data)
	return metas
}

// selectAttachmentMetas 用 --file-token 过滤要下载的附件；wanted 为空则返回全部。
// 保留 wanted 中存在于 metas 的项（按 metas 出现顺序），未匹配的 token 被忽略。
func selectAttachmentMetas(metas []attachmentMeta, wanted []string) []attachmentMeta {
	if len(wanted) == 0 {
		return metas
	}
	want := make(map[string]bool, len(wanted))
	for _, w := range wanted {
		want[w] = true
	}
	out := make([]attachmentMeta, 0, len(wanted))
	for _, m := range metas {
		if want[m.FileToken] {
			out = append(out, m)
		}
	}
	return out
}

// missingFileTokens 返回 wanted 中没有出现在 selected 里的 token（保持 wanted 顺序、去重）。
// wanted 为空时返回 nil（全下场景无「缺失」概念）。
func missingFileTokens(wanted []string, selected []attachmentMeta) []string {
	if len(wanted) == 0 {
		return nil
	}
	got := make(map[string]bool, len(selected))
	for _, m := range selected {
		got[m.FileToken] = true
	}
	var missing []string
	seen := make(map[string]bool, len(wanted))
	for _, w := range wanted {
		if got[w] || seen[w] {
			continue
		}
		seen[w] = true
		missing = append(missing, w)
	}
	return missing
}

var bitableRecordRemoveAttachmentCmd = &cobra.Command{
	Use:   "remove-attachment",
	Short: "从记录附件单元格移除 file_token",
	Long: `从记录的附件字段单元格移除一个或多个 file_token。

端点:
  POST /open-apis/base/v3/bases/{bt}/tables/{tid}/remove_attachments
  body {"attachments":{record_id:{field_id:[{file_token}]}}}

必填:
  --base-token   多维表格 base_token
  --table-id     目标数据表
  --record-id    目标记录
  --field-id     附件字段 ID
  --file-token   要移除的附件 file_token（可重复，单次≤50）

示例:
  feishu-cli bitable record remove-attachment --base-token <bt> --table-id <tid> \
    --record-id <rec> --field-id <fld> --file-token <ft>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		recordID, _ := cmd.Flags().GetString("record-id")
		fieldID, _ := cmd.Flags().GetString("field-id")
		fileTokens, _ := cmd.Flags().GetStringArray("file-token")
		if tableID == "" || recordID == "" || fieldID == "" {
			return fmt.Errorf("--table-id / --record-id / --field-id 必填")
		}
		if len(fileTokens) == 0 {
			return fmt.Errorf("--file-token 必填（可重复）")
		}
		if len(fileTokens) > 50 {
			return fmt.Errorf("单次最多 50 个 token，当前 %d 个", len(fileTokens))
		}
		body := bitableAttachmentCellBody(recordID, fieldID, fileTokens)
		return bitableRun(cmd, func(bt string) bitableReq {
			// remove_attachments 同为 table 级端点（不在 records 子路径下）。
			return bitableReq{method: "POST", path: client.BaseV3Path("bases", bt, "tables", tableID, "remove_attachments"), body: body}
		})
	},
}

func init() {
	// 挂到 bitable_record.go 中定义的 bitableRecordCmd 组
	bitableRecordCmd.AddCommand(bitableRecordUploadAttachmentCmd)
	addBitableWriteFlags(bitableRecordUploadAttachmentCmd)
	bitableRecordUploadAttachmentCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableRecordUploadAttachmentCmd.Flags().String("record-id", "", "record_id（必填）")
	bitableRecordUploadAttachmentCmd.Flags().String("field-id", "", "附件字段 field_id（必填）")
	bitableRecordUploadAttachmentCmd.Flags().StringArray("file", nil, "本地文件路径（可重复，单次≤50）")

	// download-attachment 的 --output 是附件保存路径（下载目标），与 output 包结果输出的 --output 语义冲突。
	// 故不调 addBitableWriteFlags（它会注册 --format/--jq，使 ParseOptions 抢用 --output 把结果 JSON 覆盖附件），
	// 改手动注册最小 flag 集；结果走 printJSON 到 stdout。
	bitableRecordCmd.AddCommand(bitableRecordDownloadAttachmentCmd)
	addBaseTokenFlag(bitableRecordDownloadAttachmentCmd)
	bitableRecordDownloadAttachmentCmd.Flags().String("user-access-token", "", "User Access Token")
	output.AddDryRunFlag(bitableRecordDownloadAttachmentCmd)
	bitableRecordDownloadAttachmentCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableRecordDownloadAttachmentCmd.Flags().String("record-id", "", "record_id（必填）")
	bitableRecordDownloadAttachmentCmd.Flags().StringArray("file-token", nil, "附件 file_token（可重复；省略则下载全部）")
	bitableRecordDownloadAttachmentCmd.Flags().String("output", "", "附件保存路径（文件或目录）")
	bitableRecordDownloadAttachmentCmd.Flags().Bool("overwrite", false, "已存在时覆盖")

	bitableRecordCmd.AddCommand(bitableRecordRemoveAttachmentCmd)
	addBitableWriteFlags(bitableRecordRemoveAttachmentCmd)
	bitableRecordRemoveAttachmentCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableRecordRemoveAttachmentCmd.Flags().String("record-id", "", "record_id（必填）")
	bitableRecordRemoveAttachmentCmd.Flags().String("field-id", "", "附件字段 field_id（必填）")
	bitableRecordRemoveAttachmentCmd.Flags().StringArray("file-token", nil, "要移除的附件 file_token（可重复，单次≤50）")
}
