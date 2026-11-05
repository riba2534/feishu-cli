package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/textutil"
	"github.com/spf13/cobra"
)

// permDocTypes 是权限 API（/drive/v1/permissions/:token/...）接受的 type 取值。
var permDocTypes = []string{"doc", "docx", "sheet", "bitable", "wiki", "file", "folder", "mindnote", "minutes", "slides"}

// permWikiSpaceMemberKinds 是 member_type=wikispaceid 时 body.type 的取值（对齐官方 drive +member-add/remove）。
var permWikiSpaceMemberKinds = []string{"wiki_space_member", "wiki_space_viewer", "wiki_space_editor"}

// resolvePermTarget 解析 perm 子命令的 <doc_token> 位置参数与 --doc-type。
//
//   - 裸 token：原样使用，类型取 --doc-type（默认 docx），行为与历史一致；
//   - http(s) URL：离线解析路径段（与 drive inspect 同一套 ParseResourceURL；/minutes/ 识别为妙记），
//     推断 token 与类型，/wiki/ 链接对应 doc-type=wiki（权限 API 直接接受 wiki 节点，不解包）；
//     显式传了 --doc-type 时以显式值为准，与 URL 类型冲突时报用法错误；
//   - 无法识别的 URL、非飞书域名等报 clierr.Usagef（exit 2），不再把 URL 当 token 拼进请求路径（服务端 404）。
//
// 不发起网络请求，可在 --dry-run 与凭据检查之前调用。
func resolvePermTarget(cmd *cobra.Command, raw string) (token, docType string, err error) {
	docType, _ = cmd.Flags().GetString("doc-type")
	docType = strings.TrimSpace(docType)
	raw = strings.TrimSpace(raw)
	if !client.LooksLikeURL(raw) {
		return raw, docType, nil
	}
	explicit := ""
	if cmd.Flags().Changed("doc-type") {
		explicit = client.NormalizeResourceType(docType)
	}
	if minuteToken, mErr := client.ParseMinuteURL(raw); mErr == nil {
		if explicit != "" && explicit != "minutes" {
			return "", "", clierr.Usagef("显式指定的 --doc-type %q 与 URL 路径推断的类型 \"minutes\" 冲突；请去掉 --doc-type 或改成一致的值", explicit)
		}
		return minuteToken, "minutes", nil
	}
	res, err := parseResourceArg(raw, resourceArgOptions{
		ArgName:      "<doc_token>",
		ExplicitType: explicit,
		Allowed:      permDocTypes,
	})
	if err != nil {
		return "", "", clierr.Usage(err)
	}
	return res.Token, res.Type, nil
}

// resolvePermMemberKind 校验 --member-kind：仅 member_type=wikispaceid 时允许且必填（写入 body.type），
// 此时不允许同时传 --perm-type（知识空间角色由 member-kind 表达）。
func resolvePermMemberKind(memberType, memberKind, permType string) (string, error) {
	memberKind = strings.ToLower(strings.TrimSpace(memberKind))
	if memberType != "wikispaceid" {
		if memberKind != "" {
			return "", clierr.Usagef("--member-kind 仅在 --member-type wikispaceid 时有效")
		}
		return "", nil
	}
	if memberKind == "" {
		return "", clierr.Usagef("--member-type wikispaceid 时 --member-kind 必填，可选值: %s", strings.Join(permWikiSpaceMemberKinds, ", "))
	}
	if err := validateEnum(memberKind, "--member-kind", permWikiSpaceMemberKinds); err != nil {
		return "", err
	}
	if strings.TrimSpace(permType) != "" {
		return "", clierr.Usagef("--member-type wikispaceid 时不支持 --perm-type，知识空间角色请用 --member-kind %s", strings.Join(permWikiSpaceMemberKinds, "|"))
	}
	return memberKind, nil
}

// permAPIPath 拼接权限 API 路径（token 做 PathEscape），供 --dry-run 预览使用。
func permAPIPath(format string, segments ...string) string {
	args := make([]any, len(segments))
	for i, s := range segments {
		args[i] = url.PathEscape(s)
	}
	return fmt.Sprintf(format, args...)
}

// printPermDryRun 打印 perm 子命令的单步请求预览（不读凭据、不联网）。
func printPermDryRun(cmd *cobra.Command, desc, method, path string, params, body map[string]any) error {
	return printDryRunPlan(cmd, desc, map[string]any{"as": permIdentityLabel(cmd)}, []dryRunStep{{Method: method, URL: path, Params: params, Body: body}})
}

// permIdentityLabel 返回 dry-run 中展示的身份：不解析 token，只看显式参数（未传 --as 时与 resolvePermIdentity 的历史默认一致）。
func permIdentityLabel(cmd *cobra.Command) string {
	if as, _ := cmd.Flags().GetString("as"); strings.TrimSpace(as) != "" {
		return strings.TrimSpace(as)
	}
	if strings.TrimSpace(resolveFlagUserToken(cmd)) != "" {
		return "user"
	}
	return "bot"
}

var nonJSONStatusPattern = regexp.MustCompile(`StatusCode:\s*(\d+)`)

// sanitizeNonJSONResponseError 把 SDK 的「response content-type not json, response: StatusCode: 404, Header:map[...] Body: ...」
// 改写为不含传输头的简短错误：响应头里的 trace/CDN 字段对排查参数问题无用，只会淹没真正的原因。
// 非该类错误原样返回（保留错误链）。
func sanitizeNonJSONResponseError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	idx := strings.Index(msg, "response content-type not json")
	if idx < 0 {
		return err
	}
	prefix := strings.TrimRight(strings.TrimSpace(msg[:idx]), ":：")
	status := "未知"
	if m := nonJSONStatusPattern.FindStringSubmatch(msg); len(m) == 2 {
		status = m[1]
	}
	body := ""
	if i := strings.LastIndex(msg, "Body:"); i >= 0 {
		body = strings.TrimSpace(msg[i+len("Body:"):])
	}
	text := fmt.Sprintf("服务端返回非 JSON 响应（HTTP %s）", status)
	if body != "" {
		text += "：" + textutil.TruncateUTF8(body, 200)
	}
	if prefix != "" {
		text = prefix + ": " + text
	}
	text += "\n提示：请确认 <doc_token> 是裸 token 或受支持的飞书文档链接，且 --doc-type 与资源实际类型一致"
	return errors.New(text)
}
