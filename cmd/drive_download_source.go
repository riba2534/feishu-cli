package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
)

// driveDownloadSource 是下载前识别出的真实云盘文件。
type driveDownloadSource struct {
	FileToken string // 实际下载用的 file_token
	WikiToken string // 输入是 wiki 节点时的原 node_token
	Input     string
}

// driveExportHint 在线文档不能走文件下载接口时的改用提示。
func driveExportHint(token, objType string) string {
	return fmt.Sprintf("该 token 是在线文档（%s），不能用文件下载接口；请改用 feishu-cli drive export --token %s --doc-type %s --file-extension <pdf|docx|xlsx|...>", objType, token, objType)
}

// resolveDriveDownloadSource 下载前识别 token 的真实类型（对齐官方 drive_file_source.go）：
//  1. 离线解析：接受裸 token 或飞书 URL；URL 路径已表明是在线文档（docx/sheet/...）时直接提示改用 export；
//  2. query_by_token 识别：wiki 节点自动解包为底层文件；识别为在线文档时报错并提示改用 export；
//  3. 识别失败（权限/网络等）只在 stderr 告警，退回原解析路径（wiki 输入走 node_by_token 解包），不阻断下载。
func resolveDriveDownloadSource(raw, userToken string, warn io.Writer) (*driveDownloadSource, error) {
	opts := resourceArgOptions{ArgName: "--file-token", DefaultType: "file", ResolveWiki: true, UserAccessToken: userToken}
	res, err := parseResourceArg(raw, opts)
	if err != nil {
		return nil, clierr.Usage(err)
	}
	if res.InputType != "file" && res.InputType != client.ResourceTypeWiki {
		return nil, clierr.Usagef("%s", driveExportHint(res.InputToken, res.InputType))
	}
	src := &driveDownloadSource{FileToken: res.InputToken, Input: res.Input}

	info, qerr := client.QueryDriveToken(res.InputToken, userToken)
	if qerr == nil {
		if info.ObjType != "file" {
			return nil, clierr.Usagef("%s", driveExportHint(info.ObjToken, info.ObjType))
		}
		if info.IsWikiToken {
			src.WikiToken = res.InputToken
		}
		src.FileToken = info.ObjToken
		return src, nil
	}

	// 识别失败：只透出业务码，不回显可能含 token 的原始错误文本
	msg := "⚠️  资源类型识别（query_by_token）失败，按原 token 继续"
	if e, ok := client.AsAPIError(qerr); ok && e.Code != 0 {
		msg += fmt.Sprintf("（code=%d）", e.Code)
	} else if q := (*client.DriveTokenQueryError)(nil); errors.As(qerr, &q) {
		msg += fmt.Sprintf("（code=%d）", q.Code)
	}
	fmt.Fprintln(warn, msg)
	if res.InputType == client.ResourceTypeWiki {
		if err := resolveWikiInResource(res, opts); err != nil {
			return nil, err
		}
		if res.Type != "file" {
			return nil, clierr.Usagef("%s", driveExportHint(res.Token, res.Type))
		}
		src.WikiToken = res.InputToken
		src.FileToken = res.Token
	}
	return src, nil
}

// defaultDownloadName 默认文件名：Content-Disposition → 云盘标题 → token。
func defaultDownloadName(d *client.DriveDownload, fileToken, userToken string) string {
	if name := d.FileName(); name != "" {
		return name
	}
	if title, err := client.FetchDocMetaTitle(fileToken, "file", userToken); err == nil {
		if name := sanitizeLocalFileName(title); name != "" {
			return name
		}
	}
	return fileToken
}

// sanitizeLocalFileName 去掉路径分隔符等不安全字符，避免标题把文件写出目标目录。
func sanitizeLocalFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		}
		return r
	}, name)
	if name == "." || name == ".." {
		return ""
	}
	return name
}
