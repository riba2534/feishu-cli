package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/runctx"
	"github.com/riba2534/feishu-cli/internal/safefile"
)

// apiStdinFileName 是 --file - 时 multipart part 的 filename（文件名语义通常由 file_name 等表单字段决定）。
const apiStdinFileName = "stdin"

// apiUpload 是 api --file 解析后的上传计划。
type apiUpload struct {
	field   string
	path    string // 本地路径；stdin 时为空
	stdin   bool
	size    int64 // 本地文件大小（stdin 在 load 之后才知道）
	fields  map[string]string
	content []byte
}

func (u *apiUpload) fileName() string {
	if u.stdin {
		return apiStdinFileName
	}
	return filepath.Base(u.path)
}

// parseAPIFileFlag 解析 --file [field=]path|- 并做全部本地校验（不联网、不读凭据）：
// 互斥（--data-file / --page-all）、HTTP method、路径存在且不是目录/敏感目录。
func parseAPIFileFlag(raw, method string) (*apiUpload, error) {
	if apiDataFile != "" {
		return nil, clierr.Usagef("--file 与 --data-file 不能同时使用；配合 --file 时用 --data 传表单字段（JSON 对象）")
	}
	if apiPageAll {
		return nil, clierr.Usagef("--file 与 --page-all 不能同时使用")
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return nil, clierr.Usagef("--file 只能用于 POST/PUT/PATCH/DELETE，得到 %s", method)
	}

	u := &apiUpload{field: "file"}
	value := raw
	if idx := strings.IndexByte(raw, '='); idx > 0 {
		u.field, value = raw[:idx], raw[idx+1:]
	}
	if value == "-" {
		u.stdin = true
		return u, nil
	}
	if value == "" {
		return nil, clierr.Usagef("--file 缺少文件路径，格式为 [field=]path 或 [field=]-")
	}
	// curl 风格 @path：只有字面路径不存在时才剥掉 @，不误伤真的以 @ 开头的文件名
	if strings.HasPrefix(value, "@") && len(value) > 1 {
		if _, err := os.Stat(value); err != nil {
			value = value[1:]
		}
	}
	info, err := safefile.StatInputFile(value)
	if err != nil {
		return nil, err
	}
	u.path = value
	u.size = info.Size()
	return u, nil
}

// apiFormFields 把 --data 转成 multipart 文本字段：必须是 JSON 对象，标量原样转字符串，
// 嵌套对象/数组按 JSON 文本发送，null 跳过。
func apiFormFields(body any) (map[string]string, error) {
	fields := map[string]string{}
	if body == nil {
		return fields, nil
	}
	obj, ok := body.(map[string]any)
	if !ok {
		return nil, clierr.Usagef("配合 --file 时 --data 必须是 JSON 对象（顶层键作为表单字段），如 '{\"file_type\":\"stream\"}'")
	}
	for k, v := range obj {
		switch tv := v.(type) {
		case nil:
			continue
		case string:
			fields[k] = tv
		case json.Number:
			fields[k] = tv.String()
		case bool:
			fields[k] = fmt.Sprintf("%v", tv)
		default:
			b, err := json.Marshal(tv)
			if err != nil {
				return nil, clierr.Usagef("--data 字段 %q 无法序列化: %w", k, err)
			}
			fields[k] = string(b)
		}
	}
	return fields, nil
}

// load 读入文件内容；stdin 为空视为用法错误。
func (u *apiUpload) load() error {
	if u.stdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("--file 读取 stdin 失败: %w", err)
		}
		if len(data) == 0 {
			return clierr.Usagef("--file - 从 stdin 读到空内容，请通过管道传入文件内容")
		}
		u.content = data
		u.size = int64(len(data))
		return nil
	}
	data, err := safefile.ReadInputFile(u.path)
	if err != nil {
		return err
	}
	u.content = data
	u.size = int64(len(data))
	return nil
}

// printAPIUploadDryRun 预览 multipart 请求：只展示文件名与大小，不输出文件内容。
func printAPIUploadDryRun(method, path string, q larkcore.QueryParams, u *apiUpload, tokenTypes []larkcore.AccessTokenType, hasUserToken bool) error {
	file := map[string]any{
		"field":     u.field,
		"file_name": u.fileName(),
	}
	if u.stdin {
		file["source"] = "stdin"
	} else {
		file["path"] = u.path
		file["size"] = u.size
	}
	out := map[string]any{
		"method":            method,
		"path":              path,
		"query":             toPlainQuery(q),
		"content_type":      "multipart/form-data",
		"form_fields":       u.fields,
		"file":              file,
		"supported_tokens":  tokenTypesToString(tokenTypes),
		"will_use_user_tok": hasUserToken,
		"dry_run":           true,
	}
	return printJSON(out)
}

// runAPIUpload 发送 multipart 请求并复用 api 的响应渲染与业务错误处理。
func runAPIUpload(method, apiPath string, q larkcore.QueryParams, u *apiUpload, userToken string) error {
	ctx, cancel := context.WithTimeout(runctx.Root(), time.Duration(apiTimeoutSec)*time.Second)
	defer cancel()
	resp, err := client.DoAPIMultipart(ctx, method, apiPath, url.Values(q), u.fields,
		client.MultipartFile{Field: u.field, FileName: u.fileName(), Content: u.content}, userToken)
	if err != nil {
		return err
	}
	return emitAPIBody(resp.StatusCode, resp.Header, resp.Body)
}
