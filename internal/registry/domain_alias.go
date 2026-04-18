package registry

import (
	"fmt"
	"sort"
	"strings"
)

// domainAliases maps feishu-cli legacy domain names to meta project names.
var domainAliases = map[string][]string{
	"chat": {"im"},
}

// compositeDomains maps feishu-cli composite domains to multiple meta projects.
var compositeDomains = map[string][]string{
	"drive":      {"drive", "wiki"},
	"doc_access": {"wiki"},
}

// extraDomainScopes maps each business domain to the minimum set of scopes
// needed by its commands. Each entry contains ONLY the base scopes (or
// user-identity scopes) that every command in the domain requires — per-flag
// conditional scopes are NOT included (callers should add those on top when
// relevant flags are used).
var extraDomainScopes = map[string][]string{
	// --- Domains absent from meta_data.json (fallback-only) ---

	// doc shortcuts: +search, +create, +fetch, +update, +media-insert, +media-preview, +media-download
	"search": {"search:docs:read", "search:message"},

	// base shortcuts: 85 commands covering table/field/record/view/role/workflow/form/dashboard
	"bitable": {
		"base:app:read", "base:app:create", "base:app:update", "base:app:copy",
		"base:table:read", "base:table:create", "base:table:update", "base:table:delete",
		"base:record:read", "base:record:create", "base:record:update", "base:record:delete",
		"base:field:read", "base:field:create", "base:field:update", "base:field:delete",
		"base:view:read", "base:view:write_only",
		"base:role:read", "base:role:create", "base:role:update", "base:role:delete",
		"base:workflow:read", "base:workflow:create", "base:workflow:update",
		"base:dashboard:read", "base:dashboard:create", "base:dashboard:update", "base:dashboard:delete",
		"base:form:read", "base:form:create", "base:form:update", "base:form:delete",
		"base:history:read",
		"docs:document.media:upload", // record-upload-attachment
	},

	// contact shortcuts: +get-user (UserScopes), +search-user
	"contact": {
		"contact:user.basic_profile:readonly", // +get-user UserScopes
		"contact:user:search",                 // +search-user
	},

	// feishu-cli specific composite domain
	"doc_access": {
		"docx:document:readonly", "wiki:node:read", "contact:user.base:readonly",
	},

	// --- Supplements for meta projects with few methods ---

	// vc shortcuts: +search, +notes, +recording (base scopes only, no per-flag)
	"vc": {
		"vc:meeting.search:read", // +search
		"vc:note:read",           // +notes
		"vc:record:readonly",     // +recording
	},

	// minutes shortcuts: +search, +get, +download
	"minutes": {
		"minutes:minutes.search:read",       // +search
		"minutes:minutes:readonly",          // +get
		"minutes:minutes.basic:read",        // +get (部分租户需要)
		"minutes:minutes.artifacts:read",    // +get --with-artifacts
		"minutes:minutes.transcript:export", // vc notes --download-transcript
		"minutes:minutes.media:export",      // +download
		"minutes:minute:download",           // +download
	},

	// drive shortcuts: +upload, +download, +add-comment, +export, +export-download, +import, +move, +delete, +task_result
	"drive": {
		"drive:file:upload", "drive:file:download",
		"docx:document:readonly", "docs:document.comment:create", "docs:document.comment:write_only",
		"docs:document.content:read", "docs:document:export", "drive:drive.metadata:readonly",
		"docs:document.media:upload", "docs:document:import",
		"space:document:move", "space:document:delete",
	},

	// im shortcuts (for "chat" alias): +chat-create, +chat-messages-list, +chat-search, +chat-update,
	// +messages-mget, +messages-reply, +messages-resources-download, +messages-search, +messages-send, +threads-messages-list
	"chat": {
		"im:chat:read", "im:chat:update",
		"im:message.group_msg:get_as_user", "im:message.p2p_msg:get_as_user", // +chat-messages-list UserScopes
		"contact:user.base:readonly",           // +chat-messages-list UserScopes
		"contact:user.basic_profile:readonly",   // +messages-mget UserScopes
		"im:message:readonly",                   // +messages-resources-download
		"search:message",                        // +messages-search
		"im:message.send_as_user", "im:message", // +messages-send/reply UserScopes
		"im:chat:create_by_user",                // +chat-create UserScopes
	},

	// task shortcuts: +create, +update, +comment, +complete, +reopen, +assign, +followers, +reminder,
	// +get-my-tasks, +tasklist-create, +tasklist-task-add, +tasklist-members
	"task": {
		"task:task:read", "task:task:write",
		"task:tasklist:read", "task:tasklist:write",
		"task:comment:write",
	},

	// calendar shortcuts: +agenda, +create, +freebusy, +room-find, +rsvp, +suggestion
	"calendar": {
		"calendar:calendar.event:read",   // +agenda
		"calendar:calendar.event:create",  // +create
		"calendar:calendar.event:update",  // +create
		"calendar:calendar.event:reply",   // +rsvp
		"calendar:calendar.free_busy:read", // +freebusy, +room-find, +suggestion
	},

	// wiki shortcuts: +node-create, +move-docs-to-wiki
	"wiki": {
		"wiki:node:create", "wiki:node:read", "wiki:space:read",
		"wiki:node:move", // wiki move-docs 需要
	},

	// mail shortcuts: +message, +messages, +thread, +triage, +watch, +reply, +reply-all, +send, +forward, +draft-create, +draft-edit
	"mail": {
		"mail:user_mailbox.message:readonly",
		"mail:user_mailbox.message.address:read",
		"mail:user_mailbox.message.subject:read",
		"mail:user_mailbox.message.body:read",
		"mail:event",
		"mail:user_mailbox.message:modify",
		"mail:user_mailbox:readonly",
		"mail:user_mailbox.message:send",
		"mail:user_mailbox.mail_contact:read",
		"mail:user_mailbox.mail_contact:write",
		"mail:user_mailbox.folder:read", // triage --list-folders
	},

	// sheets shortcuts: +info, +read, +write, +append, +find, +create, +export, +merge-cells, etc.
	"sheets": {
		"sheets:spreadsheet:read", "sheets:spreadsheet:write_only", "sheets:spreadsheet:create",
		"docs:document:export", "drive:file:download",
	},

	// approval: approval:task is needed for user-token approval task queries
	"approval": {
		"approval:task",
	},

	// doc shortcuts: +search, +create, +fetch, +update, +media-insert, +media-preview, +media-download
	"docs": {
		"search:docs:read",
		"docx:document:create", "docx:document:readonly", "docx:document:write_only",
		"docs:document.media:upload", "docs:document.media:download",
	},

	// slides shortcuts: +create
	"slides": {
		"slides:presentation:create", "slides:presentation:write_only",
	},

	// whiteboard shortcuts: +query, +update
	"whiteboard": {
		"board:whiteboard:node:read", "board:whiteboard:node:create", "board:whiteboard:node:delete",
	},
}

// domainDescriptions provides descriptions for alias/composite/fallback domains.
var domainDescriptions = map[string]struct{ Zh, En string }{
	"chat":       {"群聊、消息、Reaction/Pin、群管理", "Message, chat, reaction, pin & group management"},
	"bitable":    {"多维表格（Base 别名）", "Base / Bitable (alias)"},
	"drive":      {"云空间上传/下载/导出/导入/评论", "Drive upload, download, export, import & comments"},
	"doc_access": {"用户 Token 访问文档/知识库", "User Token document & wiki access"},
	"search":     {"文档和消息搜索", "Document and message search"},
}

// ResolveProjects expands a domain name to meta project names.
// Returns nil if the domain is unknown.
func ResolveProjects(domain string) []string {
	if projects, ok := domainAliases[domain]; ok {
		return projects
	}
	if projects, ok := compositeDomains[domain]; ok {
		return projects
	}
	// Check if it's a direct meta project name
	spec := LoadFromMeta(domain)
	if spec != nil {
		return []string{domain}
	}
	// Check if it's a fallback-only domain
	if _, ok := extraDomainScopes[domain]; ok {
		return nil // no meta projects, but still a valid domain
	}
	return nil
}

// isKnownDomain checks if a domain name is recognized.
func isKnownDomain(domain string) bool {
	if _, ok := domainAliases[domain]; ok {
		return true
	}
	if _, ok := compositeDomains[domain]; ok {
		return true
	}
	if _, ok := extraDomainScopes[domain]; ok {
		return true
	}
	return LoadFromMeta(domain) != nil
}

// KnownDomainNames returns all valid domain names (sorted):
// meta projects (excluding those with auth_domain) + aliases + composites + fallbacks.
func KnownDomainNames() []string {
	seen := make(map[string]bool)

	// Meta projects without auth_domain
	for _, p := range ListFromMetaProjects() {
		if !HasAuthDomain(p) {
			seen[p] = true
		}
	}

	// Aliases
	for alias := range domainAliases {
		seen[alias] = true
	}

	// Composites
	for comp := range compositeDomains {
		seen[comp] = true
	}

	// Fallbacks
	for fb := range extraDomainScopes {
		seen[fb] = true
	}

	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// ParseDomains normalizes and validates a list of domain tokens.
// Supports comma-separated values, case-insensitive, and "all".
func ParseDomains(input []string) ([]string, error) {
	parts := make([]string, 0, len(input))
	for _, item := range input {
		for _, piece := range strings.Split(item, ",") {
			piece = strings.TrimSpace(piece)
			if piece == "" {
				continue
			}
			parts = append(parts, piece)
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}

	for _, p := range parts {
		if strings.EqualFold(p, "all") {
			return KnownDomainNames(), nil
		}
	}

	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, item := range parts {
		item = strings.ToLower(item)
		if !isKnownDomain(item) {
			return nil, fmt.Errorf("未知授权域 %q，可选值: %s, all", item, strings.Join(KnownDomainNames(), ", "))
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out, nil
}

// GetDomainDescription returns the localized description for a domain.
// Checks service_descriptions.json first, then domainDescriptions.
func GetDomainDescription(domain, lang string) string {
	if desc := GetServiceDescription(domain, lang); desc != "" {
		return desc
	}
	if dd, ok := domainDescriptions[domain]; ok {
		if lang == "en" {
			return dd.En
		}
		return dd.Zh
	}
	return ""
}

// GetDomainTitle returns the localized title for a domain.
// Checks service_descriptions.json first, then falls back to the domain name.
func GetDomainTitle(domain, lang string) string {
	if title := GetServiceTitle(domain, lang); title != "" {
		return title
	}
	return domain
}

// CollectDomainScopes collects scopes for the specified domains using the registry.
// It resolves aliases/composites, collects priority-based scopes from meta_data,
// expands auth_domain children, merges fallback scopes, and optionally filters
// to auto-approve scopes.
func CollectDomainScopes(domains []string, recommendedOnly bool) []string {
	scopeSet := make(map[string]bool)

	// 1. Expand domains to meta projects and collect priority-based scopes
	projectSet := make(map[string]bool)
	for _, domain := range domains {
		if projects, ok := domainAliases[domain]; ok {
			for _, p := range projects {
				projectSet[p] = true
			}
		} else if projects, ok := compositeDomains[domain]; ok {
			for _, p := range projects {
				projectSet[p] = true
			}
		} else if LoadFromMeta(domain) != nil {
			projectSet[domain] = true
		}
	}

	// 2. Expand auth_domain children
	expanded := make(map[string]bool)
	for p := range projectSet {
		expanded[p] = true
		for _, child := range GetAuthChildren(p) {
			expanded[child] = true
		}
	}

	// 3. Collect scopes from meta for all expanded projects
	projects := make([]string, 0, len(expanded))
	for p := range expanded {
		projects = append(projects, p)
	}
	for _, s := range CollectScopesForProjects(projects, "user") {
		scopeSet[s] = true
	}

	// 4. Add extra scopes (from shortcuts / manual supplements)
	// Check both the original domain name and alias-resolved meta project names.
	checked := make(map[string]bool)
	for _, domain := range domains {
		// Check original domain name (e.g., "chat", "bitable", "vc")
		if !checked[domain] {
			checked[domain] = true
			if extra, ok := extraDomainScopes[domain]; ok {
				for _, s := range extra {
					scopeSet[s] = true
				}
			}
		}
		// Check alias-resolved names (e.g., "chat" -> check "im" too)
		if targets, ok := domainAliases[domain]; ok {
			for _, t := range targets {
				if !checked[t] {
					checked[t] = true
					if extra, ok := extraDomainScopes[t]; ok {
						for _, s := range extra {
							scopeSet[s] = true
						}
					}
				}
			}
		}
	}

	// 5. Build sorted result
	result := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		result = append(result, s)
	}
	sort.Strings(result)

	// 6. Filter to auto-approve if requested
	if recommendedOnly {
		result = FilterAutoApproveScopes(result)
	}

	return result
}
