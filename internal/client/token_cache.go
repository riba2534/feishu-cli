package client

import (
	"context"
	"sync"
	"time"
)

// instanceTokenCache 是每个 SDK client 实例独享的 token 缓存（实现 larkcore.Cache）。
//
// SDK 默认使用进程级全局缓存，且只按 app_id 区分：GetClient 在 base_url / secret 变化时虽会重建
// client，旧配置换到的 tenant_access_token 仍会被新 client 复用（例如同一 app_id 从飞书切到 Lark
// 域名）；测试中不同桩服务器之间也会因此串 token。缓存跟随 client 实例，重建即丢弃。
type instanceTokenCache struct {
	mu      sync.Mutex
	entries map[string]tokenCacheEntry
}

type tokenCacheEntry struct {
	value   string
	expires time.Time
}

func newInstanceTokenCache() *instanceTokenCache {
	return &instanceTokenCache{entries: map[string]tokenCacheEntry{}}
}

func (c *instanceTokenCache) Set(_ context.Context, key, value string, expireTime time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = tokenCacheEntry{value: value, expires: time.Now().Add(expireTime)}
	return nil
}

// Get 未命中或已过期时返回空串（与 SDK 内置缓存语义一致，SDK 据此重新换取 token）。
func (c *instanceTokenCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return "", nil
	}
	if time.Now().After(e.expires) {
		delete(c.entries, key)
		return "", nil
	}
	return e.value, nil
}
