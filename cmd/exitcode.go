package cmd

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"syscall"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/v2/internal/auth"
	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

// authErrorCodes 归入退出码 3（鉴权 / 权限 / 应用凭证）的飞书业务码。
//
// 判定一律走 client.HasAPICode（词边界安全），不对错误文本做裸数字 substring 匹配。
// 资源级"无权限"（如文档 1063004、1770032）属于业务错误，仍归退出码 1：
// 它们需要资源 owner 授权，重新登录或开通 scope 都修不好。
var authErrorCodes = []int{
	99991661, // 缺少 Authorization 头 / access token
	99991662, // 应用在租户内已停用
	99991663, // access token 无效（含 tenant_access_token 换取失败）
	99991664, // app_access_token 无效
	99991668, // user access token 无效/过期，或该接口不支持 User Token
	99991671, // token 格式错误
	99991672, // 应用未开通所需 scope
	99991673, // 应用状态不可用（未安装 / 未发布）
	99991676, // token 缺少所需 scope
	99991677, // user access token 已过期
	99991679, // 用户未授权所需 scope
	99991543, // app_id / app_secret 错误
	10014,    // app secret invalid（旧 TAT 端点）
	20026,    // refresh_token 格式过旧（终态）
	20037,    // refresh_token 已过期（终态）
	20050,    // refresh 端点临时错误
	20064,    // refresh_token 已吊销（终态）
	20073,    // refresh_token 已被使用（终态）
	230027,   // 用户未授权应用 / 缺少必要权限
}

// cobraUsageErrorPrefixes 是 cobra / pflag（v1.8 / v1.0.5）用法错误的固定文案前缀。
// cobra 以普通 error 返回这些错误、不提供类型，只能按文案识别；用 HasPrefix 而不是 Contains，
// 避免把业务错误 msg 里恰好出现的 "invalid argument " 之类字样误判为用法错误。
var cobraUsageErrorPrefixes = []string{
	"unknown command ",
	"unknown flag: ",
	"unknown shorthand flag: ",
	"flag needs an argument: ",
	"bad flag syntax: ",
	"invalid argument ",
	"required flag(s) ",
	"accepts ",           // accepts N arg(s) / accepts at most / accepts between
	"requires at least ", // requires at least N arg(s)
	"if any flags in the group ",
	"at least one of the flags in the group ",
}

// networkErrorPhrases 网络层错误的稳定文案片段（小写）。
// 用于兜底识别以 %v 拼接、丢失了错误链的网络错误。
var networkErrorPhrases = []string{
	"dial tcp",
	"i/o timeout",
	"no such host",
	"connection refused",
	"connection reset by peer",
	"network is unreachable",
	"tls handshake timeout",
	"client.timeout exceeded",
	"context deadline exceeded",
	"server misbehaving",
}

// exitCodeFor 把命令返回的错误映射为进程退出码（见 clierr 包的常量说明）。
//
// 优先级：信号中断(130) > 需要确认(10) > 用户取消(1) > 用法(2) > 网络(4) > 鉴权(3) > 一般(1)。
// 网络先于鉴权：token 刷新因断网失败时，重试网络即可恢复，不应提示重新登录。
func exitCodeFor(err error) int {
	if err == nil {
		return clierr.ExitOK
	}
	// 收到 SIGINT/SIGTERM 后命令因 context 取消而失败：沿用 shell 约定 130
	if rootInterrupted() {
		return clierr.ExitInterrupted
	}
	kinds := clierr.Kinds(err)
	has := func(k clierr.Kind) bool {
		for _, got := range kinds {
			if got == k {
				return true
			}
		}
		return false
	}
	switch {
	case has(clierr.KindConfirmationRequired):
		return clierr.ExitConfirmationRequired
	case has(clierr.KindCancelled):
		return clierr.ExitGeneral
	case has(clierr.KindUsage):
		return clierr.ExitUsage
	case has(clierr.KindNetwork) || isNetworkError(err):
		return clierr.ExitNetwork
	case has(clierr.KindAuth) || isAuthError(err):
		return clierr.ExitAuth
	case isCobraUsageError(err):
		return clierr.ExitUsage
	}
	return clierr.ExitGeneral
}

// isCobraUsageError 识别 cobra / pflag 直接返回的用法错误。
func isCobraUsageError(err error) bool {
	msg := err.Error()
	for _, p := range cobraUsageErrorPrefixes {
		if strings.HasPrefix(msg, p) {
			return true
		}
	}
	return false
}

// isNetworkError 判断错误是否由网络层引起（超时、连接失败、DNS 等）。
func isNetworkError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	// *url.Error 也会包装 host 白名单拒绝等本地策略错误，只有超时才算网络错误
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return true
	}
	// 飞书 SDK 把 *url.Error 转成了不带 Unwrap 的自有类型
	var sdkTimeout *larkcore.ClientTimeoutError
	var sdkDial *larkcore.DialFailedError
	var sdkServerTimeout *larkcore.ServerTimeoutError
	if errors.As(err, &sdkTimeout) || errors.As(err, &sdkDial) || errors.As(err, &sdkServerTimeout) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, p := range networkErrorPhrases {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// isAuthError 判断错误是否属于鉴权 / 权限 / 应用凭证问题。
func isAuthError(err error) bool {
	if errors.Is(err, auth.ErrNoUserTokenConfigured) || errors.Is(err, auth.ErrAppMismatch) || errors.Is(err, auth.ErrUnboundToken) {
		return true
	}
	for _, code := range authErrorCodes {
		if client.HasAPICode(err, code) {
			return true
		}
	}
	return false
}
