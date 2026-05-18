package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/client"
)

// scanAndUploadInlineImages 是 --inline-images-auto-scan 的内部实现
// 步骤:
//  1. 解析 HTML body 中所有 <img src="local-path">（跳过 cid:/http:/https:/data: 等已有 scheme）
//  2. 解析当前登录用户的 open_id（drive upload 的 parent_node 要求 user open_id）
//  3. 每张图生成 CID，读盘 → drive upload (parent_type=email) → 拿 file_token
//  4. 回填 inlineImagePart 列表，并把 body 中 src 改写为 cid:xxx
//
// 失败行为: 任何一步出错都返回 error，调用方应中止（不发送脏 body）
func scanAndUploadInlineImages(htmlBody, mailboxID, userToken string) (string, []inlineImagePart, error) {
	rawSrcs := client.ScanInlineImagePaths(htmlBody)
	if len(rawSrcs) == 0 {
		return htmlBody, nil, nil
	}

	// 解析 open_id：drive upload parent_node 必填
	openID, err := resolveCurrentUserOpenID(userToken)
	if err != nil {
		return "", nil, fmt.Errorf("--inline-images-auto-scan 需要 open_id：%w", err)
	}

	refs := make([]client.MailInlineImageRef, 0, len(rawSrcs))
	parts := make([]inlineImagePart, 0, len(rawSrcs))

	for _, src := range rawSrcs {
		cid, err := client.GenerateMailCID()
		if err != nil {
			return "", nil, err
		}
		ref := client.MailInlineImageRef{
			RawSrc:    src,
			LocalPath: src,
			CID:       cid,
			FileName:  filepath.Base(src),
		}
		// 读盘填充 bytes/mime（multipart/related part 必需）
		if loadErr := client.LoadInlineImageBytes(&ref); loadErr != nil {
			return "", nil, fmt.Errorf("内嵌图片 %s: %w", src, loadErr)
		}
		// 上传到飞书云盘（parent_type=email）
		fileToken, upErr := client.UploadMailInlineImage(ref.LocalPath, ref.FileName, openID, userToken)
		if upErr != nil {
			return "", nil, fmt.Errorf("上传内嵌图片 %s 失败: %w", src, upErr)
		}
		ref.FileToken = fileToken

		refs = append(refs, ref)
		parts = append(parts, inlineImagePart{
			CID:      ref.CID,
			Filename: ref.FileName,
			Bytes:    ref.Bytes,
			MIME:     ref.MIME,
		})
	}

	rewritten := client.ReplaceInlineImageSrc(htmlBody, refs)
	return rewritten, parts, nil
}

// resolveCurrentUserOpenID 拿当前 active user token 对应的真实 open_id
// 优先 cache（且校验 token 一致性），cache 不命中或与 active token 不一致就回源 /authen/v1/user_info
// 修复 codex review P2 finding：之前只读 cache 不校验 token，多 profile / 显式传 --user-access-token 时会拿到错误 open_id
func resolveCurrentUserOpenID(userToken string) (string, error) {
	if strings.TrimSpace(userToken) == "" {
		return "", fmt.Errorf("解析 open_id 需要有效的 user access token")
	}
	if cache, err := auth.LoadCurrentUserCache(); err == nil && cache != nil && cache.OpenID != "" && cache.MatchesToken(userToken) {
		return cache.OpenID, nil
	}
	info, err := client.GetCurrentUserInfo(userToken)
	if err != nil {
		return "", fmt.Errorf("调用 /authen/v1/user_info 拉取 open_id 失败（请确认 token 有效且 scope 含 auth:user.id:read）: %w", err)
	}
	if info == nil || info.OpenID == "" {
		return "", fmt.Errorf("/authen/v1/user_info 未返回 open_id")
	}
	return info.OpenID, nil
}
