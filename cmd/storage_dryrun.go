package cmd

import (
	"github.com/riba2534/feishu-cli/internal/client"
)

// driveFileUploadSteps 返回云盘文件上传（/drive/v1/files/upload_*）的 --dry-run 步骤，
// 与 client.UploadFileWithToken / OverwriteDriveFileFromPath 的分支一致：
// ≤20MB 走 upload_all，>20MB 走 upload_prepare → upload_part × N → upload_finish；
// overwriteToken 非空时携带 file_token 原地覆盖为新版本。
func driveFileUploadSteps(filePath, fileName, parentNode, overwriteToken string, size int64) []dryRunStep {
	if client.DriveNeedsMultipart(size) {
		prepare := map[string]any{
			"file_name":   fileName,
			"parent_type": "explorer",
			"parent_node": parentNode,
			"size":        size,
		}
		if overwriteToken != "" {
			prepare["file_token"] = overwriteToken
		}
		return []dryRunStep{
			{Method: "POST", URL: "/open-apis/drive/v1/files/upload_prepare", Desc: "Initialize multipart upload", Body: prepare},
			{Method: "POST", URL: "/open-apis/drive/v1/files/upload_part", Desc: "Upload file parts (repeated)", Body: map[string]any{
				"upload_id": "<upload_id>", "seq": "<chunk_index>", "size": "<chunk_size>", "file": "<chunk_binary>",
			}},
			{Method: "POST", URL: "/open-apis/drive/v1/files/upload_finish", Desc: "Finalize multipart upload", Body: map[string]any{
				"upload_id": "<upload_id>", "block_num": "<block_num>",
			}},
		}
	}
	body := map[string]any{
		"file_name":   fileName,
		"parent_type": "explorer",
		"parent_node": parentNode,
		"size":        size,
		"file":        "@" + filePath,
	}
	if overwriteToken != "" {
		body["file_token"] = overwriteToken
	}
	return []dryRunStep{{Method: "POST", URL: "/open-apis/drive/v1/files/upload_all", Desc: "Upload file (single request, ≤20MB)", Body: body}}
}

// mediaUploadAllStep 返回素材上传（/drive/v1/medias/upload_all）的 --dry-run 步骤，与 client.UploadMediaWithExtra 一致。
func mediaUploadAllStep(filePath, fileName, parentType, parentNode, extra string, size int64) dryRunStep {
	body := map[string]any{
		"file_name":   fileName,
		"parent_type": parentType,
		"parent_node": parentNode,
		"size":        size,
		"file":        "@" + filePath,
	}
	if extra != "" {
		body["extra"] = extra
	}
	return dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_all", Desc: "Upload media", Body: body}
}
