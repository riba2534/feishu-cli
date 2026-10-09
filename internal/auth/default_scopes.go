package auth

import (
	"sort"
	"strings"
	"unicode"
)

var defaultLoginScopes = []string{
	// 用于 /authen/v1/user_info 和“当前登录用户是谁”这类最基础能力。
	"auth:user.id:read",
}

// NormalizeScopeList 规范化 scope 列表：按逗号与任意空白切分、去重，再用单空格拼接。
//
// OAuth 2.0（RFC 6749 §3.3）要求 scope 以空格分隔；服务端把 "a,b" 当成一个非法 scope 拒绝
// （invalid or malformed scopes）。用户习惯写逗号分隔，这里统一转成线上格式。
func NormalizeScopeList(scope string) string {
	return strings.Join(UniqueScopeList(scope), " ")
}

// DefaultLoginScopes 返回 auth login 默认申请的最小核心 user scopes。
//
// 设计目的：
//   - 让首次 auth login 稳定成功，不再因默认 scope 过多触发飞书数量上限
//   - 让后续缺权限场景通过 `auth check` + `auth login --scope "..."` 按需补授权
func DefaultLoginScopes() string {
	return strings.Join(defaultLoginScopes, " ")
}

// DefaultLoginScopeList returns auth login 默认申请的最小核心 user scopes 切片。
func DefaultLoginScopeList() []string {
	return append([]string(nil), defaultLoginScopes...)
}

// SplitScopes 按逗号与任意空白（空格、制表符、换行）切分 scope 字符串，丢弃空项，不去重。
func SplitScopes(scope string) []string {
	return strings.FieldsFunc(scope, func(r rune) bool {
		return r == ',' || r == '，' || unicode.IsSpace(r)
	})
}

// UniqueScopeList 把 scope 字符串（逗号或空白分隔，可混用）切成去重、保序的切片。
func UniqueScopeList(scope string) []string {
	seen := make(map[string]struct{}, 64)
	parts := make([]string, 0, 64)
	for _, item := range SplitScopes(scope) {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		parts = append(parts, item)
	}
	return parts
}

// MergeScopeLists merges multiple scope slices while preserving first-seen order.
func MergeScopeLists(groups ...[]string) []string {
	seen := make(map[string]struct{}, 64)
	parts := make([]string, 0, 64)
	for _, group := range groups {
		for _, item := range group {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			parts = append(parts, item)
		}
	}
	return parts
}

// JoinScopes joins multiple scope slices into a normalized space-separated string.
func JoinScopes(groups ...[]string) string {
	return strings.Join(MergeScopeLists(groups...), " ")
}

// SortScopeList returns a sorted copy, useful in tests/help output.
func SortScopeList(scopes []string) []string {
	out := append([]string(nil), scopes...)
	sort.Strings(out)
	return out
}
