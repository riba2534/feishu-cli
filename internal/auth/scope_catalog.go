package auth

import (
	"github.com/riba2534/feishu-cli/internal/registry"
)

// KnownScopeDomainNames returns supported domain names.
// Delegates to registry which includes meta projects, aliases, composites, and fallbacks.
func KnownScopeDomainNames() []string {
	return registry.KnownDomainNames()
}

// ParseScopeDomains normalizes a list of domain tokens.
// Supports comma-separated, case-insensitive, and "all".
func ParseScopeDomains(input []string) ([]string, error) {
	return registry.ParseDomains(input)
}

// CollectDomainScopes collects scopes for the specified domains using the registry.
func CollectDomainScopes(domains []string, recommendedOnly bool) ([]string, error) {
	return registry.CollectDomainScopes(domains, recommendedOnly), nil
}

// DomainScopeUniverse 返回业务域覆盖的全部 scope（含推荐过滤与批量剔除前的项），用于 --exclude 校验。
func DomainScopeUniverse(domains []string) []string {
	return registry.DomainScopeUniverse(domains)
}

// IsBatchExcludedScope 报告 scope 是否会在批量（--domain / --recommend）申请时被剔除。
func IsBatchExcludedScope(scope string) bool {
	return registry.IsBatchExcludedScope(scope)
}
