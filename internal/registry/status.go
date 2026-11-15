package registry

import "time"

// CatalogInfo 描述当前进程实际使用的 OpenAPI catalog。
// source: embedded（仅编译期基线）/ cache（磁盘 overlay）/ runtime（本次进程远端拉取）。
type CatalogInfo struct {
	Source          string `json:"source"`
	Brand           string `json:"brand"`
	EmbeddedVersion string `json:"embedded_version"`
	CacheVersion    string `json:"cache_version,omitempty"`
	RuntimeVersion  string `json:"runtime_version"`
	ServiceCount    int    `json:"service_count"`
	MethodCount     int    `json:"method_count"`
	RemoteEnabled   bool   `json:"remote_enabled"`
	LastCheckAt     int64  `json:"last_check_at,omitempty"`
	CachePath       string `json:"cache_path,omitempty"`
	// Supplements 是 overlay 之后按 local_supplements.json 白名单从内嵌基线补回的方法；
	// 仅在远端 overlay 缺失这些方法时非空（离线/embedded 模式下它们本就在内嵌基线里）。
	Supplements []string `json:"supplements,omitempty"`
}

// Status 返回当前 catalog 来源、版本和规模。会触发 Init。
func Status() CatalogInfo {
	Init()
	runtimeVer := runtimeVersion
	if runtimeVer == "" {
		runtimeVer = embeddedVersion
	}
	info := CatalogInfo{
		Source:          overlaySourceOrEmbedded(),
		Brand:           configuredBrand,
		EmbeddedVersion: embeddedVersion,
		RuntimeVersion:  runtimeVer,
		ServiceCount:    len(mergedServices),
		MethodCount:     CountMethods(mergedServices),
		RemoteEnabled:   remoteEnabled(),
		CachePath:       overlayCachePath(),
		Supplements:     append([]string(nil), appliedSupplements...),
	}
	if cm, err := loadCacheMeta(); err == nil {
		info.CacheVersion = cm.Version
		info.LastCheckAt = cm.LastCheckAt
	}
	return info
}

func overlayCachePath() string {
	if !cacheEnabled() {
		return ""
	}
	return cachePath()
}

func overlaySourceOrEmbedded() string {
	switch overlaySource {
	case "cache", "runtime":
		return overlaySource
	default:
		return "embedded"
	}
}

// LastCheckTime 把 LastCheckAt unix 秒格式化；0 返回空串。
func LastCheckTime(unix int64) string {
	if unix <= 0 {
		return ""
	}
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}
