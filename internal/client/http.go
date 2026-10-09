package client

import (
	"net/http"

	"github.com/riba2534/feishu-cli/internal/config"
)

// rawHTTPClient 返回手写 HTTP 请求（绕过 SDK、自带 Bearer 头）使用的受控客户端：
//   - 共享进程级连接池（config.NewHTTPClient 复用同一个底层 Transport）；
//   - 请求 URL 走 host 白名单（只放行官方 Open API / Accounts、loopback 或显式 opt-in 的自定义 host）；
//   - 拒绝 HTTPS→HTTP 降级与带 body 的跨源重定向，跨源时剥离 Authorization 等凭证头。
//
// 客户端本身不设墙钟超时，超时与取消由请求 context 控制（Context() / ContextWithTimeout()），
// 调用方必须用 http.NewRequestWithContext 构造请求。禁止在手写 Bearer 请求里使用
// http.DefaultClient / http.Get / 裸 &http.Client{}：它们没有超时、绕过 host 白名单，
// 重定向时还可能把 User Token 带到别的主机。
func rawHTTPClient() *http.Client {
	return config.NewHTTPClient(0)
}
