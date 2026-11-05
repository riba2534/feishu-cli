package registry

import (
	"embed"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"sync"
)

//go:embed scope_priorities.json scope_overrides.json local_supplements.json
var registryFS embed.FS

// embeddedMetaJSON is set by loader_embedded.go when meta_data.json is compiled in.
var embeddedMetaJSON []byte

var (
	mergedServices    = make(map[string]map[string]interface{})
	mergedProjectList []string
	initOnce          sync.Once
)

// MergedRegistry is the top-level structure of meta_data.json.
type MergedRegistry struct {
	Version  string                   `json:"version"`
	Services []map[string]interface{} `json:"services"`
}

// Init initializes the registry: embedded baseline first, then optional remote overlay,
// then local_supplements（overlay 按 service 整体替换后，从内嵌基线补回白名单方法）.
// Remote fetch errors never fail Init. Safe to call multiple times (sync.Once).
func Init() {
	initOnce.Do(func() {
		configuredBrand = resolveBrand()
		loadEmbeddedIntoMerged()
		applyRemoteOverlay()
		applyLocalSupplements()
		rebuildProjectList()
	})
}

func loadEmbeddedIntoMerged() {
	if len(embeddedMetaJSON) == 0 {
		return
	}
	var reg MergedRegistry
	if err := json.Unmarshal(embeddedMetaJSON, &reg); err != nil {
		return
	}
	embeddedVersion = reg.Version
	runtimeVersion = reg.Version
	overlaySource = ""
	for _, svc := range reg.Services {
		name, ok := svc["name"].(string)
		if !ok || name == "" {
			continue
		}
		mergedServices[name] = svc
		embeddedServices[name] = svc
	}
}

func rebuildProjectList() {
	mergedProjectList = make([]string, 0, len(mergedServices))
	for name := range mergedServices {
		mergedProjectList = append(mergedProjectList, name)
	}
	sort.Strings(mergedProjectList)
}

// LoadFromMeta loads a service schema by project name.
func LoadFromMeta(project string) map[string]interface{} {
	Init()
	return mergedServices[project]
}

// ListFromMetaProjects lists available service project names (sorted).
func ListFromMetaProjects() []string {
	Init()
	return mergedProjectList
}

// --- scope priorities ---

const DefaultScopeScore = 0

var cachedScopePriorities map[string]int

type scopePriorityEntry struct {
	ScopeName  string `json:"scope_name"`
	FinalScore string `json:"final_score"`
	Recommend  string `json:"recommend"`
}

// LoadScopePriorities loads the scope priorities map.
// Higher score = more recommended / least privilege.
func LoadScopePriorities() map[string]int {
	if cachedScopePriorities != nil {
		return cachedScopePriorities
	}

	data, err := registryFS.ReadFile("scope_priorities.json")
	if err != nil {
		cachedScopePriorities = make(map[string]int)
		return cachedScopePriorities
	}

	var entries []scopePriorityEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		cachedScopePriorities = make(map[string]int)
		return cachedScopePriorities
	}

	m := make(map[string]int, len(entries))
	for _, entry := range entries {
		f, err := strconv.ParseFloat(entry.FinalScore, 64)
		if err != nil {
			continue
		}
		m[entry.ScopeName] = int(math.Round(f))
	}

	// Apply manual overrides from scope_overrides.json
	if overrideData, err := registryFS.ReadFile("scope_overrides.json"); err == nil {
		var wrapper struct {
			PriorityOverrides map[string]int `json:"priority_overrides"`
		}
		if json.Unmarshal(overrideData, &wrapper) == nil {
			for scope, score := range wrapper.PriorityOverrides {
				m[scope] = score
			}
		}
	}

	cachedScopePriorities = m
	return cachedScopePriorities
}

// --- auto-approve ---

var cachedAutoApproveSet map[string]bool

// LoadAutoApproveSet returns the set of auto-approve scope names.
func LoadAutoApproveSet() map[string]bool {
	if cachedAutoApproveSet != nil {
		return cachedAutoApproveSet
	}

	m := make(map[string]bool)

	// From scope_priorities.json (Recommend == "true")
	if data, err := registryFS.ReadFile("scope_priorities.json"); err == nil {
		var entries []scopePriorityEntry
		if json.Unmarshal(data, &entries) == nil {
			for _, entry := range entries {
				if entry.Recommend == "true" {
					m[entry.ScopeName] = true
				}
			}
		}
	}

	// From scope_overrides.json (recommend.allow/deny lists)
	if data, err := registryFS.ReadFile("scope_overrides.json"); err == nil {
		var wrapper struct {
			AutoApprove struct {
				Allow []string `json:"allow"`
				Deny  []string `json:"deny"`
			} `json:"recommend"`
		}
		if json.Unmarshal(data, &wrapper) == nil {
			for _, s := range wrapper.AutoApprove.Allow {
				m[s] = true
			}
			for _, s := range wrapper.AutoApprove.Deny {
				delete(m, s)
			}
		}
	}

	cachedAutoApproveSet = m
	return cachedAutoApproveSet
}

// IsAutoApproveScope returns true if the scope is auto-approve.
func IsAutoApproveScope(scope string) bool {
	return LoadAutoApproveSet()[scope]
}

// FilterAutoApproveScopes filters a scope list to only include auto-approve scopes.
func FilterAutoApproveScopes(scopes []string) []string {
	autoApprove := LoadAutoApproveSet()
	var result []string
	for _, s := range scopes {
		if autoApprove[s] {
			result = append(result, s)
		}
	}
	return result
}

// GetScopeScore returns the priority score for a scope, or DefaultScopeScore if not found.
func GetScopeScore(scope string) int {
	priorities := LoadScopePriorities()
	if score, ok := priorities[scope]; ok {
		return score
	}
	return DefaultScopeScore
}

// CollectAllScopesFromMeta collects all unique scopes from meta_data.json
// for the given identity, recursively walking nested resources.
// Results are deduplicated and sorted.
func CollectAllScopesFromMeta(identity string) []string {
	scopeSet := make(map[string]bool)
	for _, project := range ListFromMetaProjects() {
		spec := LoadFromMeta(project)
		if spec == nil {
			continue
		}
		resources, _ := spec["resources"].(map[string]interface{})
		walkResourceMethods(resources, func(methodMap map[string]interface{}) {
			if !methodSupportedForIdentity(methodMap, identity) {
				return
			}
			scopes, ok := methodMap["scopes"].([]interface{})
			if !ok {
				return
			}
			for _, s := range scopes {
				if str, ok := s.(string); ok {
					scopeSet[str] = true
				}
			}
		})
	}

	result := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		result = append(result, s)
	}
	sort.Strings(result)
	return result
}
