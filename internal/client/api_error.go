package client

import (
	"errors"
	"fmt"
	"github.com/riba2534/feishu-cli/v2/internal/textutil"
	"net/http"
	"net/url"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/v2/internal/apidiag"
	"github.com/riba2534/feishu-cli/v2/internal/config"
)

// APIError 飞书 OpenAPI 业务错误（含 HTTP 非 2xx 但响应体是飞书 JSON 信封的情况）。
//
// Error() 形如 "<Action>失败: code=<N>, msg=<M>, log_id=<L>"：保留本仓统一的 code=N 形态，
// HasAPICode 与按业务码分支的逻辑照常可用；缺失 scope、字段校验等其余诊断由根命令附加打印。
type APIError struct {
	Action string // 中文动作（不含"失败"），如 "查询密级标签"；为空时不带前缀
	apidiag.Info
}

func (e *APIError) Error() string {
	var b strings.Builder
	if e.Action != "" {
		b.WriteString(e.Action)
		b.WriteString("失败: ")
	}
	fmt.Fprintf(&b, "code=%d, msg=%s", e.Code, e.Msg)
	if e.LogID != "" {
		fmt.Fprintf(&b, ", log_id=%s", e.LogID)
	}
	return b.String()
}

// AsAPIError 从错误链中取出 *APIError。
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr != nil {
		return apiErr, true
	}
	return nil, false
}

// ParseAPIResponse 先按飞书业务信封解析响应体，再看 HTTP 状态码。
//
// 飞书大量业务错误随 HTTP 400/403 下发；若先判 `StatusCode != 200` 就返回 "HTTP 400, body: ..."，
// 按业务码分支的专门提示（如 secure-label、外部群）永远走不到，log_id 等诊断也被丢弃。
//
//   - body 是带非零 code 的飞书信封 → 返回 *APIError（无论 HTTP 状态码）；
//   - HTTP 2xx 且 code == 0（或无 code 字段）→ 返回 nil；
//   - HTTP 非 2xx 且 body 不是飞书信封 → 返回 "<Action>失败: HTTP <status>, body: <预览>"。
func ParseAPIResponse(action string, status int, header http.Header, body []byte) error {
	if info, ok := apidiag.Parse(status, header, body); ok {
		return &APIError{Action: action, Info: info}
	}
	if status >= 200 && status < 300 {
		return nil
	}
	prefix := ""
	if action != "" {
		prefix = action + "失败: "
	}
	return fmt.Errorf("%sHTTP %d, body: %s", prefix, status, bodyPreview(body))
}

// CheckAPIResponse 是 ParseAPIResponse 针对 SDK 原始响应（client.Get/Post/Do 返回值）的便捷封装。
//
// 用法：
//
//	resp, err := cli.Get(Context(), apiPath, nil, tokenType, opts...)
//	if err != nil {
//		return fmt.Errorf("查询密级标签失败: %w", err)
//	}
//	if err := CheckAPIResponse("查询密级标签", resp); err != nil {
//		if apiErr, ok := AsAPIError(err); ok && apiErr.Code == 1063xxx { /* 专门提示 */ }
//		return err
//	}
//	// 解析 resp.RawBody 的 data ...
func CheckAPIResponse(action string, resp *larkcore.ApiResp) error {
	if resp == nil {
		return fmt.Errorf("%s失败: 响应为空", action)
	}
	return ParseAPIResponse(action, resp.StatusCode, resp.Header, resp.RawBody)
}

func bodyPreview(body []byte) string {
	const max = 512
	s := strings.TrimSpace(string(body))
	if len(s) > max {
		return textutil.TruncateUTF8(s, max) + "...(已截断)"
	}
	return s
}

// APIErrorHint 按飞书业务码给出可执行的修复建议，覆盖鉴权 / 权限 / 应用凭证 / 限流等跨领域错误码；
// 领域专属错误码返回 ""，由各命令自行提示。返回值以 "提示：" 开头。
func APIErrorHint(info apidiag.Info) string {
	switch info.Code {
	case 99991672:
		return appScopeHint(info.MissingScopes)
	case 99991679, 99991676:
		return userScopeHint(info.MissingScopes)
	case 99991668:
		// 同一业务码有两种语义：接口不支持 User Token，或 User Token 无效/过期
		if strings.Contains(strings.ToLower(info.Msg), "not support") {
			return "提示：该接口不支持 User Access Token（只收应用身份），请改用 `--as bot` 重新调用。"
		}
		return "提示：User Access Token 无效或已过期。运行 `feishu-cli auth status` 检查，必要时重新 `feishu-cli auth login`。"
	case 99991677:
		return "提示：User Access Token 已过期。运行 `feishu-cli auth refresh` 刷新，refresh_token 也失效时重新 `feishu-cli auth login`。"
	case 99991661:
		return "提示：请求未携带 Access Token。检查 `--as` 身份选择与 `feishu-cli auth status` 的登录状态。"
	case 99991663, 99991664:
		return "提示：Access Token 无效。Bot 身份请检查 app_id / app_secret 是否正确且应用已发布；User 身份请重新 `feishu-cli auth login`。"
	case 99991671:
		return "提示：Token 格式错误（User Token 应以 u- 开头，Tenant Token 以 t- 开头），请检查 --user-access-token / FEISHU_USER_ACCESS_TOKEN。"
	case 99991543, 10014:
		return "提示：app_id 或 app_secret 不正确。检查 --bot-app-id/--bot-app-secret、FEISHU_APP_ID/FEISHU_APP_SECRET 或 config.yaml。"
	case 99991662, 99991673:
		return "提示：应用在当前租户不可用（未安装、未发布或已停用），请联系租户管理员在管理后台检查应用状态。"
	case 20026, 20037, 20064, 20073:
		return "提示：refresh_token 已失效（过期、被吊销或已被使用），无法自动续期，请重新 `feishu-cli auth login`。"
	case 20050:
		return "提示：token 刷新服务暂时不可用，请稍后重试。"
	case 99991400:
		return "提示：请求被限流，请降低并发或稍后重试。"
	case 1770035:
		return "提示：单次建块请求最多包含 5 个画板块（图片、附件不受此限），请把画板分到多次请求中创建；doc import / doc add 已自动分批。"
	}
	return ""
}

// appScopeHint 99991672：应用自身未开通 scope。重新登录修不好，必须到开放平台为应用开通并发布。
func appScopeHint(scopes []string) string {
	link := appScopeConsoleURL(scopes)
	var b strings.Builder
	b.WriteString("提示：应用未开通所需权限（重新 auth login 无法解决）。请应用管理员在开放平台为应用开通")
	if len(scopes) > 0 {
		fmt.Fprintf(&b, "任一 scope（%s）", strings.Join(scopes, ", "))
	} else {
		b.WriteString("所需 scope")
	}
	b.WriteString("并发布新版本")
	if link != "" {
		fmt.Fprintf(&b, "：%s", link)
	}
	b.WriteString("。以 User 身份调用时，开通后还需 `feishu-cli auth login --scope \"<scope>\"` 重新授权。")
	return b.String()
}

// userScopeHint 99991679 / 99991676：用户授权未覆盖所需 scope。
func userScopeHint(scopes []string) string {
	if len(scopes) == 0 {
		return "提示：当前 User Token 未授权所需 scope。运行 `feishu-cli auth login --scope \"<所需 scope>\"` 补充授权（可先用 `feishu-cli auth check --scope` 预检）；若以 Bot 身份调用，请改为在开放平台为应用开通该 scope。"
	}
	hint := fmt.Sprintf("提示：当前 User Token 未授权所需 scope。运行 `feishu-cli auth login --scope %q` 补充授权", scopes[0])
	if len(scopes) > 1 {
		hint += fmt.Sprintf("（%s 任选其一即可）", strings.Join(scopes, ", "))
	}
	return hint + "；若应用本身也未开通该 scope，需先在开放平台开通。"
}

// appScopeConsoleURL 生成开放平台"权限管理"页链接（与服务端 99991672 文案中的链接同形）。
func appScopeConsoleURL(scopes []string) string {
	cfg := config.Get()
	if cfg == nil || strings.TrimSpace(cfg.AppID) == "" {
		return ""
	}
	base := config.OfficialOpenBase(config.ParseBrand(cfg.BaseURL))
	u := fmt.Sprintf("%s/app/%s/auth", base, url.PathEscape(strings.TrimSpace(cfg.AppID)))
	if len(scopes) > 0 {
		u += "?q=" + url.QueryEscape(strings.Join(scopes, ",")) + "&op_from=openapi"
	}
	return u
}
