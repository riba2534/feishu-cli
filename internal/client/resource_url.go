package client

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/riba2534/feishu-cli/internal/config"
)

// 资源类型取值与 Drive metas / permission / export 等接口的 type 参数一致。
const (
	ResourceTypeDoc      = "doc"
	ResourceTypeDocx     = "docx"
	ResourceTypeSheet    = "sheet"
	ResourceTypeBitable  = "bitable"
	ResourceTypeWiki     = "wiki"
	ResourceTypeFile     = "file"
	ResourceTypeFolder   = "folder"
	ResourceTypeMindnote = "mindnote"
	ResourceTypeSlides   = "slides"
)

// ResourceRef 是从飞书/Lark 资源 URL 解析出的类型与 token。
type ResourceRef struct {
	Type  string // 规范化资源类型，见 ResourceType* 常量
	Token string // URL 路径中紧跟类型前缀的第一个路径段
}

// resourceURLPathTypes 是 URL 路径前缀 → 资源类型映射（对齐官方 common/resource_url.go）。
//
// 只按 u.Path 的前缀匹配，绝不在整串 URL 里搜索标记：
// 否则 https://x.feishu.cn/docx/ABC?from=/wiki/zzz 会被误判为 wiki 节点 zzz。
// 较长前缀必须排在前面（如 /drive/folder/ 先于任何 /drive/ 前缀）。
var resourceURLPathTypes = []struct {
	Prefix string
	Type   string
}{
	{"/drive/folder/", ResourceTypeFolder},
	{"/drive/file/", ResourceTypeFile},
	{"/drive/shr/", ResourceTypeFolder},
	{"/chat/drive/", ResourceTypeFolder},
	{"/docx/", ResourceTypeDocx},
	{"/docs/", ResourceTypeDoc},
	{"/doc/", ResourceTypeDoc},
	{"/sheets/", ResourceTypeSheet},
	{"/spreadsheets/", ResourceTypeSheet},
	{"/base/", ResourceTypeBitable},
	{"/bitable/", ResourceTypeBitable},
	{"/wiki/", ResourceTypeWiki},
	{"/file/", ResourceTypeFile},
	{"/mindnotes/", ResourceTypeMindnote},
	{"/mindnote/", ResourceTypeMindnote},
	{"/slides/", ResourceTypeSlides},
}

// SupportedResourceURLPaths 返回支持的 URL 路径前缀（用于错误提示与帮助文本）。
func SupportedResourceURLPaths() string {
	prefixes := make([]string, 0, len(resourceURLPathTypes))
	for _, m := range resourceURLPathTypes {
		prefixes = append(prefixes, m.Prefix)
	}
	return strings.Join(prefixes, "、")
}

// resourceHostSuffixes 是接受的飞书 / Lark 文档域名（含其子域）。
var resourceHostSuffixes = []string{"feishu.cn", "larksuite.com", "larkoffice.com"}

// IsFeishuResourceHost 判断 hostname 是否为飞书/Lark 官方文档域名或其子域。
// 按域名边界匹配（"." 分隔），evilfeishu.cn 这类伪造域名不会命中。
func IsFeishuResourceHost(hostname string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")
	for _, suffix := range resourceHostSuffixes {
		if h == suffix || strings.HasSuffix(h, "."+suffix) {
			return true
		}
	}
	return false
}

func isLoopbackResourceHost(hostname string) bool {
	h := strings.Trim(strings.TrimSpace(hostname), "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// IsSafeResourceToken 判断 token 是否只由字母、数字、'_'、'-' 组成（长度 1-128）。
// 飞书各类资源 token（docx/sheet/wiki/folder/file/fake_office_...）都满足该形状；
// 拒绝 '/'、'?'、'#'、'%'、'.'、空白与控制字符，防止拼进 API 路径时发生路径注入。
func IsSafeResourceToken(token string) bool {
	if token == "" || len(token) > 128 {
		return false
	}
	for _, r := range token {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// LooksLikeURL 判断输入是否按 URL 处理（包含 "://"）。
func LooksLikeURL(raw string) bool {
	return strings.Contains(strings.TrimSpace(raw), "://")
}

// NormalizeResourceType 规范化用户传入的资源类型（小写并收敛常见别名）。
// 未知取值原样（小写）返回，由调用方按命令支持范围校验。
func NormalizeResourceType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "sheets", "spreadsheet", "spreadsheets":
		return ResourceTypeSheet
	case "base":
		return ResourceTypeBitable
	case "slide", "presentation":
		return ResourceTypeSlides
	case "mindnotes":
		return ResourceTypeMindnote
	}
	return t
}

// ParseResourceURL 解析飞书/Lark 资源 URL，返回资源类型与 token。
//
// 规则：
//   - 协议必须是 https（http 仅允许 localhost/回环地址，便于本地测试），禁止 userinfo；
//   - 域名必须是 *.feishu.cn / *.larksuite.com / *.larkoffice.com（显式开启
//     allow_custom_base_url 的私有化部署除外）；
//   - 只按 URL path 的前缀推断类型，query / fragment 中出现的 /wiki/ 等字样一律忽略；
//   - token 取前缀后的第一个路径段，必须满足 IsSafeResourceToken。
func ParseResourceURL(rawURL string) (ResourceRef, error) {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return ResourceRef{}, fmt.Errorf("URL 不能为空")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ResourceRef{}, fmt.Errorf("URL 格式无效: %q", raw)
	}
	if u.User != nil {
		return ResourceRef{}, fmt.Errorf("URL 不能包含用户信息 (userinfo): %q", raw)
	}
	hostname := strings.ToLower(u.Hostname())
	switch strings.ToLower(u.Scheme) {
	case "https":
		if !IsFeishuResourceHost(hostname) && !isLoopbackResourceHost(hostname) && !config.AllowCustomBaseURL() {
			return ResourceRef{}, fmt.Errorf("不支持的域名 %q：仅接受飞书/Lark 文档域名（*.feishu.cn、*.larksuite.com、*.larkoffice.com）；私有化部署请开启 allow_custom_base_url", hostname)
		}
	case "http":
		if !isLoopbackResourceHost(hostname) {
			return ResourceRef{}, fmt.Errorf("文档 URL 必须使用 https 协议: %q", raw)
		}
	default:
		return ResourceRef{}, fmt.Errorf("不支持的 URL 协议 %q，仅支持 https", u.Scheme)
	}
	lowerEscaped := strings.ToLower(u.EscapedPath())
	if strings.Contains(lowerEscaped, "%2f") || strings.Contains(lowerEscaped, "%5c") {
		return ResourceRef{}, fmt.Errorf("URL 路径包含非法的转义分隔符: %q", raw)
	}

	for _, m := range resourceURLPathTypes {
		if !strings.HasPrefix(u.Path, m.Prefix) {
			continue
		}
		token := u.Path[len(m.Prefix):]
		if i := strings.IndexByte(token, '/'); i >= 0 {
			token = token[:i]
		}
		if token == "" {
			return ResourceRef{}, fmt.Errorf("URL 路径 %q 缺少 token", u.Path)
		}
		if !IsSafeResourceToken(token) {
			return ResourceRef{}, fmt.Errorf("URL 中的 token %q 格式无效（只允许字母、数字、_ 和 -）", token)
		}
		return ResourceRef{Type: m.Type, Token: token}, nil
	}
	return ResourceRef{}, fmt.Errorf("无法从 URL 路径 %q 识别资源类型；支持的路径前缀: %s", u.Path, SupportedResourceURLPaths())
}

// 各品牌的标准文档入口域名：服务端会把 /docx/<token> 等路径重定向到租户自己的域名。
// 注意：裸 larksuite.com 不提供该重定向（实测超时），Lark 必须使用 www 子域。
const (
	feishuResourceURLBase = "https://www.feishu.cn"
	larkResourceURLBase   = "https://www.larksuite.com"
)

// ResourceURLBase 返回当前配置品牌（按 base_url 判定飞书 / Lark）的标准文档入口域名。
func ResourceURLBase() string {
	brand := config.BrandFeishu
	if cfg := config.Get(); cfg != nil {
		brand = config.ParseBrand(cfg.BaseURL)
	}
	return ResourceURLBaseForBrand(brand)
}

// ResourceURLBaseForBrand 返回指定品牌的标准文档入口域名。
func ResourceURLBaseForBrand(brand config.Brand) string {
	if brand == config.BrandLark {
		return larkResourceURLBase
	}
	return feishuResourceURLBase
}

// resourceURLPaths 是资源类型 → 用户可访问 URL 路径（BuildResourceURL 使用）。
var resourceURLPaths = map[string]string{
	ResourceTypeDocx:     "/docx/",
	ResourceTypeDoc:      "/docs/",
	ResourceTypeSheet:    "/sheets/",
	ResourceTypeBitable:  "/base/",
	ResourceTypeWiki:     "/wiki/",
	ResourceTypeFile:     "/file/",
	ResourceTypeFolder:   "/drive/folder/",
	ResourceTypeMindnote: "/mindnotes/",
	ResourceTypeSlides:   "/slides/",
}

// BuildResourceURL 按当前品牌生成资源的标准访问链接（ParseResourceURL 的逆操作）。
//
// 用于创建类接口不返回 url 时的兜底：链接指向品牌标准域名（www.feishu.cn / www.larksuite.com），
// 由服务端重定向到租户域名，并非猜测租户的自定义域名。kind 未知、token 为空或含非法字符时返回 ""，
// 调用方应只在结果非空时写入，避免覆盖接口已返回的真实链接。
func BuildResourceURL(kind, token string) string {
	return BuildResourceURLForBrand(ResourceURLBase(), kind, token)
}

// BuildResourceURLForBrand 与 BuildResourceURL 相同，但显式指定入口域名（便于测试）。
func BuildResourceURLForBrand(base, kind, token string) string {
	token = strings.TrimSpace(token)
	if !IsSafeResourceToken(token) {
		return ""
	}
	path, ok := resourceURLPaths[NormalizeResourceType(kind)]
	if !ok {
		return ""
	}
	return strings.TrimRight(base, "/") + path + token
}
