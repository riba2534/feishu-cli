package cmd

import (
	"fmt"
	"path/filepath"

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
	openID, err := resolveCurrentUserOpenID()
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

// resolveCurrentUserOpenID 从 ~/.feishu-cli/user_profile.json 读出当前登录用户 open_id
// 没有缓存或为空时返回明确错误，提示用户先 auth login
func resolveCurrentUserOpenID() (string, error) {
	cache, err := auth.LoadCurrentUserCache()
	if err != nil {
		return "", err
	}
	if cache == nil || cache.OpenID == "" {
		return "", fmt.Errorf("未找到当前用户 open_id，请先执行 `feishu-cli auth login` 完成登录")
	}
	return cache.OpenID, nil
}
