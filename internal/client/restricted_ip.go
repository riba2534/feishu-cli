// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.）：
// 受限网段参考其 internal/validate/url.go 的 isRestrictedDownloadIP。

package client

import "net"

// isRestrictedRemoteIP 判断访问用户给出的公网 URL（远程图片探测、封面 --url 下载）时 IP 是否落在受限网段：
// 回环、未指定、组播、链路本地、私有（含 IPv6 fc00::/7）、0.0.0.0/8、100.64.0.0/10（CGNAT）、
// 198.18.0.0/15（基准测试）、240.0.0.0/4（保留与广播），以及 IPv6 文档前缀 2001:db8::/32。
// nil 视为受限。
//
// 注意：这是 doc script 远程图片探测与文档封面 --url 共用的判定，与预签名下载用的 isBlockedIP 相互独立，
// 修改这里不影响 isBlockedIP 的调用方。
func isRestrictedRemoteIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 0: // 0.0.0.0/8 RFC 1122 "this network"
			return true
		case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127: // 100.64.0.0/10 RFC 6598 CGNAT
			return true
		case v4[0] == 198 && (v4[1] == 18 || v4[1] == 19): // 198.18.0.0/15 RFC 2544 基准测试
			return true
		case v4[0] >= 240: // 240.0.0.0/4 保留与广播
			return true
		}
		return false
	}
	// IPv6 文档前缀 2001:db8::/32（不可路由）
	return len(ip) == net.IPv6len && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8
}
