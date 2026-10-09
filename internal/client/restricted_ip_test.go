package client

import (
	"net"
	"testing"
)

func TestIsRestrictedRemoteIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true}, {"127.255.0.9", true}, {"::1", true}, // 回环
		{"0.0.0.0", true}, {"::", true}, {"0.1.2.3", true}, // 未指定 / 0.0.0.0/8
		{"10.0.0.1", true}, {"172.16.0.1", true}, {"172.31.255.255", true}, {"192.168.1.1", true}, // 私网
		{"fc00::1", true}, {"fd12:3456::1", true}, // IPv6 唯一本地
		{"169.254.169.254", true}, {"fe80::1", true}, // 链路本地（含云元数据地址）
		{"224.0.0.1", true}, {"239.255.255.250", true}, {"ff02::1", true}, {"ff01::1", true}, // 组播
		{"100.64.0.1", true}, {"100.127.255.255", true}, // CGNAT
		{"198.18.0.1", true}, {"198.19.255.255", true}, // 基准测试
		{"240.0.0.1", true}, {"255.255.255.255", true}, // 保留与广播
		{"2001:db8::1", true},                                 // IPv6 文档前缀
		{"::ffff:10.0.0.1", true}, {"::ffff:127.0.0.1", true}, // IPv4 映射地址按 IPv4 判定
		{"8.8.8.8", false}, {"93.184.216.34", false}, {"172.32.0.1", false}, {"172.15.255.255", false},
		{"100.63.255.255", false}, {"100.128.0.1", false}, {"198.17.255.255", false}, {"198.20.0.1", false},
		{"239.255.255.255", true}, {"223.255.255.255", false}, {"2001:4860:4860::8888", false}, {"2001:db9::1", false},
	}
	for _, tc := range cases {
		if got := isRestrictedRemoteIP(net.ParseIP(tc.ip)); got != tc.want {
			t.Errorf("isRestrictedRemoteIP(%s) = %v，期望 %v", tc.ip, got, tc.want)
		}
	}
	if !isRestrictedRemoteIP(nil) {
		t.Fatal("nil 应视为受限")
	}
}

// TestRestrictedRemoteIPMatchesPreviousCallers 抽取公共函数前后语义对比：
// 远程图片探测完全一致；封面 --url 只额外拦截不可路由的 2001:db8::/32。
func TestRestrictedRemoteIPMatchesPreviousCallers(t *testing.T) {
	oldProbe := func(ip net.IP) bool { // 抽取前 remote_image_probe.go 的 isRestrictedRemoteImageIP
		if ip == nil || isBlockedIP(ip) || ip.IsInterfaceLocalMulticast() {
			return true
		}
		if v4 := ip.To4(); v4 != nil {
			return v4[0] == 0 || (v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127) || (v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)) || v4[0] >= 240
		}
		return len(ip) == net.IPv6len && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8
	}
	oldCover := func(ip net.IP) bool { // 抽取前 doc_cover_url.go 的 isUnsafeCoverIP
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
			return true
		}
		if v4 := ip.To4(); v4 != nil {
			return v4[0] == 0 || (v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127) || (v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)) || v4[0] >= 240
		}
		return false
	}
	// IPv4 每个 /16 取首、中、尾三个地址，再加一批 IPv6 边界地址
	var ips []net.IP
	for a := 0; a < 256; a++ {
		for b := 0; b < 256; b++ {
			for _, c := range []byte{0, 128, 255} {
				ips = append(ips, net.IPv4(byte(a), byte(b), c, 1))
			}
		}
	}
	for _, s := range []string{"::", "::1", "fc00::", "fdff:ffff::1", "fe80::", "febf::1", "fec0::1", "ff00::1", "ff02::1",
		"2001:db8::", "2001:db8:ffff::1", "2001:db7::1", "2001:4860::8888", "2400:cb00::1", "64:ff9b::808:808"} {
		ips = append(ips, net.ParseIP(s))
	}
	for _, ip := range ips {
		got := isRestrictedRemoteIP(ip)
		if got != oldProbe(ip) {
			t.Fatalf("%s: 公共函数 %v 与原远程图片探测 %v 不一致", ip, got, oldProbe(ip))
		}
		inDocPrefix := ip.To4() == nil && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8
		if got != oldCover(ip) && !inDocPrefix {
			t.Fatalf("%s: 公共函数 %v 与原封面判定 %v 不一致", ip, got, oldCover(ip))
		}
	}
}
