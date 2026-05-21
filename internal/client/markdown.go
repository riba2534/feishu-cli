package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// OverwriteFileWithToken 覆盖现有 Drive 文件（如 .md）的内容。
//
// 飞书 Open API `POST /open-apis/drive/v1/files/upload_all` 支持 `file_token` 参数：
// 当 `file_token` 存在时，平台将上传的字节流写到该文件的新版本（保留 token、刷新 version、size）；
// 当 `file_token` 缺省时，则按 `parent_type` + `parent_node` 在指定目录新建文件。
//
// 但飞书 SDK v3.5.3 的 `UploadAllFileReqBody` 没有暴露 `file_token` 字段（只有 file_name/parent_type/parent_node/size/checksum/file），
// 所以这里直接用 `client.Post` + `*larkcore.Formdata` 自己拼 multipart——translator 检测到 `*Formdata` 会自动
// 切到 FileUpload 多部分序列化路径（见 SDK core/reqtranslator.go:payload）。
//
// 参数：
//   - fileToken：要覆盖的目标文件 token（必填）
//   - fileName：写入后的文件名（必填；通常和原文件名一致，传别的名字会改名）
//   - content：新文件字节内容
//   - userAccessToken：User Access Token，为空时回退 Tenant Token
//
// 权限：drive:file:upload + drive:drive.metadata:readonly（用户身份必须对该文件有编辑权限）。
//
// 注意：本函数仅适合 ≤ 20MB 的小文件（API 单次上传上限）；大文件覆盖需要分片接口，本镜像未实现。
func OverwriteFileWithToken(fileToken, fileName string, content []byte, userAccessToken string) (string, error) {
	if fileToken == "" {
		return "", fmt.Errorf("file_token 不能为空")
	}
	if fileName == "" {
		return "", fmt.Errorf("file_name 不能为空")
	}

	cli, err := GetClient()
	if err != nil {
		return "", err
	}

	// 参考 lark-cli `shortcuts/markdown/helpers.go:uploadMarkdownFileAll`：
	// 覆盖与新建同一 endpoint，区别仅是多带一个 `file_token` 字段；parent_type 仍是 explorer。
	fd := larkcore.NewFormdata().
		AddField("file_name", fileName).
		AddField("parent_type", "explorer").
		AddField("file_token", fileToken).
		AddField("size", fmt.Sprintf("%d", len(content))).
		AddFile("file", bytes.NewReader(content))

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(ContextWithTimeout(downloadTimeout), "/open-apis/drive/v1/files/upload_all", fd, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("覆盖文件失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("覆盖文件失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			FileToken string `json:"file_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析覆盖响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return "", fmt.Errorf("覆盖文件失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	returned := apiResp.Data.FileToken
	if returned == "" {
		returned = fileToken
	}
	return returned, nil
}

// OverwriteFileFromPathWithToken 把本地文件内容覆盖到远端 Drive 文件。
// 仅适合 ≤ 20MB 的文件。
func OverwriteFileFromPathWithToken(filePath, fileToken, fileName, userAccessToken string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("读取本地文件失败: %w", err)
	}
	return OverwriteFileWithToken(fileToken, fileName, data, userAccessToken)
}
