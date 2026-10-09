package cmd

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

// 邮件附件限制（对齐官方 shortcuts/mail/limits.go 与 emlbuilder.MaxEMLSize）。
const (
	// mailMaxEMLBytes 整封 EML（base64 编码后）上限。附件按 base64 计入，约 18MB 原始附件即触顶。
	mailMaxEMLBytes = 25 * 1024 * 1024
	// mailMaxAttachmentCount 单封邮件附件数上限（含转发携带的原附件）。
	mailMaxAttachmentCount = 250
	// mailMaxAttachmentDownloadBytes 下载原邮件单个附件的安全上限（防止无界内存占用）。
	mailMaxAttachmentDownloadBytes = 35 * 1024 * 1024
	// mailAttachmentTypeLarge 超大附件（以云文档卡片形式存在，不在 EML 中携带）。
	mailAttachmentTypeLarge = 2
)

// mailBlockedAttachmentExts 禁止作为附件发送的可执行/脚本类扩展名（取自官方 filecheck 的高危子集）。
var mailBlockedAttachmentExts = map[string]bool{
	"apk": true, "app": true, "bat": true, "bin": true, "chm": true, "cmd": true, "com": true, "cpl": true,
	"dll": true, "exe": true, "hta": true, "jar": true, "js": true, "jse": true, "lnk": true, "msc": true,
	"msi": true, "msp": true, "pif": true, "ps1": true, "reg": true, "scr": true, "sh": true, "vb": true,
	"vbe": true, "vbs": true, "ws": true, "wsc": true, "wsf": true, "wsh": true,
}

// addMailAttachFlag 给发信类命令挂 --attach。
func addMailAttachFlag(cmd *cobra.Command) {
	cmd.Flags().StringArray("attach", nil, "附件文件路径（可重复传入或逗号分隔；整封邮件编码后 ≤25MB，超出请先上传云盘并在正文放链接）")
}

// estimateBase64Size 估算 base64（每 76 字符折行 CRLF）后的字节数。
func estimateBase64Size(n int64) int64 {
	encoded := (n + 2) / 3 * 4
	return encoded + encoded/76*2 + 2
}

// loadMailAttachmentsFromFlags 读取 --attach 指定的本地文件为附件 part。
// 校验：路径安全（safefile）、普通文件、扩展名黑名单、数量与整封 EML 体积上限。
func loadMailAttachmentsFromFlags(cmd *cobra.Command) ([]mailAttachmentPart, error) {
	if cmd.Flags().Lookup("attach") == nil {
		return nil, nil
	}
	values, _ := cmd.Flags().GetStringArray("attach")
	var paths []string
	for _, v := range values {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				paths = append(paths, p)
			}
		}
	}
	if len(paths) == 0 {
		return nil, nil
	}
	if len(paths) > mailMaxAttachmentCount {
		return nil, clierr.Usagef("附件数量 %d 超过上限 %d", len(paths), mailMaxAttachmentCount)
	}
	var total int64
	parts := make([]mailAttachmentPart, 0, len(paths))
	for _, p := range paths {
		if err := safefile.ValidateInputPath(p); err != nil {
			return nil, err
		}
		name := filepath.Base(p)
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
		if mailBlockedAttachmentExts[ext] {
			return nil, clierr.Usagef("附件 %s 的扩展名 .%s 属于可执行/脚本类型，出于安全原因禁止作为邮件附件", name, ext)
		}
		info, err := os.Stat(p)
		if err != nil {
			return nil, clierr.Usagef("读取附件 %s 失败: %v", p, err)
		}
		if !info.Mode().IsRegular() {
			return nil, clierr.Usagef("附件必须是普通文件: %s", p)
		}
		total += estimateBase64Size(info.Size())
		if total > mailMaxEMLBytes {
			return nil, clierr.Usagef("附件编码后总大小超过 25MB 上限（当前已超出于 %s）；超大文件请先用 `feishu-cli drive upload` 上传云盘，再把链接写进正文", name)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, clierr.Usagef("读取附件 %s 失败: %v", p, err)
		}
		parts = append(parts, mailAttachmentPart{Filename: name, MIME: mailGuessMIME(name), Bytes: data})
	}
	return parts, nil
}

// mailGuessMIME 按扩展名推断 MIME，未知时 application/octet-stream。
func mailGuessMIME(name string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		return t
	}
	return "application/octet-stream"
}

// ==================== 转发：携带原邮件附件 ====================

// fetchMailForwardAttachments 下载原邮件的普通附件（非内联、非超大附件），用于 forward 携带原附件。
// 下载链接来自 attachments/download_url（预签名 URL，请求时不带 Authorization，防 token 外泄）。
// 返回成功下载的附件，以及被跳过的附件说明（超大附件 / 无下载链接）。
func fetchMailForwardAttachments(mailboxID, messageID string, atts []mailSourceAttachment, userAccessToken string) ([]mailAttachmentPart, []string, error) {
	var ids []string
	byID := make(map[string]mailSourceAttachment)
	var skipped []string
	for _, a := range atts {
		if a.IsInline || a.ID == "" {
			continue
		}
		if a.AttachmentType == mailAttachmentTypeLarge {
			skipped = append(skipped, fmt.Sprintf("%s（超大附件以云文档卡片形式存在，未随转发携带）", a.Filename))
			continue
		}
		ids = append(ids, a.ID)
		byID[a.ID] = a
	}
	if len(ids) == 0 {
		return nil, skipped, nil
	}
	urls, failed, err := client.GetMailAttachmentDownloadURLs(mailboxID, messageID, ids, userAccessToken)
	if err != nil {
		return nil, skipped, fmt.Errorf("获取原邮件附件下载链接失败: %w", err)
	}
	for _, id := range failed {
		skipped = append(skipped, fmt.Sprintf("%s（服务端未返回下载链接）", byID[id].Filename))
	}
	var parts []mailAttachmentPart
	var total int64
	for _, id := range ids {
		u, ok := urls[id]
		if !ok || u == "" {
			continue
		}
		data, err := downloadMailAttachment(u)
		if err != nil {
			return nil, skipped, fmt.Errorf("下载原邮件附件 %s 失败: %w", byID[id].Filename, err)
		}
		total += estimateBase64Size(int64(len(data)))
		if total > mailMaxEMLBytes {
			return nil, skipped, clierr.Usagef("原邮件附件编码后总大小超过 25MB 上限，无法随转发携带；可加 --no-original-attachments 只转发正文")
		}
		name := byID[id].Filename
		parts = append(parts, mailAttachmentPart{Filename: name, MIME: mailGuessMIME(name), Bytes: data})
	}
	return parts, skipped, nil
}

// downloadMailAttachment 下载预签名附件 URL（只允许 https、不携带鉴权头、限制最大字节数）。
// 声明为变量便于单测替换（httptest 地址是 loopback，会被 SSRF 防护拒绝）。
var downloadMailAttachment = func(rawURL string) ([]byte, error) {
	return client.DownloadPresignedBytes(rawURL, mailMaxAttachmentDownloadBytes)
}
