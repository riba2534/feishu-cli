package config

import (
	"net/http"
	"testing"
	"time"
)

// TestNewHTTPClientSharesTransport 回归：过去每次 NewHTTPClient 都 Clone 一次 DefaultTransport，
// 连接池无法复用且空闲连接随调用数增长。现在所有受控客户端共享同一个底层 Transport。
func TestNewHTTPClientSharesTransport(t *testing.T) {
	a := NewHTTPClient(10 * time.Second)
	b := NewHTTPClient(0)
	pa, ok1 := a.Transport.(*policyTransport)
	pb, ok2 := b.Transport.(*policyTransport)
	if !ok1 || !ok2 {
		t.Fatalf("受控客户端必须包一层 policyTransport（host 白名单），得到 %T / %T", a.Transport, b.Transport)
	}
	if pa.base != pb.base {
		t.Fatal("两次 NewHTTPClient 应共享同一个底层 Transport")
	}
	if pa.base == http.DefaultTransport {
		t.Fatal("共享 Transport 应是 DefaultTransport 的克隆，避免被其他代码修改全局默认值影响")
	}
	if a.CheckRedirect == nil {
		t.Fatal("受控客户端必须保留重定向安全策略")
	}

	custom := &http.Transport{}
	c := NewHTTPClientWithTransport(custom, 0)
	if pc := c.Transport.(*policyTransport); pc.base != custom {
		t.Fatal("显式传入的 Transport 应原样使用（测试注入）")
	}
}
