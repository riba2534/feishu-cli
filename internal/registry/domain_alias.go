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
		"minutes:permission:apply",          // minutes apply-permission
	},

	// drive shortcuts: +upload, +download, +add-comment, +export, +export-download, +import, +move, +delete, +task_result
	"drive": {
		"drive:file:upload", "drive:file:download",
		"docx:document:readonly", "docs:document.comment:create", "docs:document.comment:write_only",
		"docs:document.content:read", "docs:document:export", "drive:drive.metadata:readonly",
		"docs:document.media:upload", "docs:document:import",
		"space:document:move", "space:document:delete",
		"docs:secure_label:readonly", "docs:secure_label:write_only", // drive secure-label list / set
		"docs:permission.member:apply", // drive apply-permission
	},

	// im shortcuts (for "chat" alias): +chat-create, +chat-messages-list, +chat-search, +chat-update,
	// +messages-mget, +messages-reply, +messages-resources-download, +messages-search, +messages-send, +threads-messages-list
	"chat": {
		"im:chat:read", "im:chat:update",
		"im:message.group_msg:get_as_user", "im:message.p2p_msg:get_as_user", // +chat-messages-list UserScopes
		"contact:user.base:readonly",            // +chat-messages-list UserScopes
		"contact:user.basic_profile:readonly",   // +messages-mget UserScopes
		"im:message:readonly",                   // +messages-resources-download
		"search:message",                        // +messages-search
		"im:message.send_as_user", "im:message", // +messages-send/reply UserScopes
		"im:chat:create_by_user", // +chat-create UserScopes
	},

	// task shortcuts: +create, +update, +comment, +complete, +reopen, +assign, +followers, +reminder,
	// +get-my-tasks, +tasklist-create, +tasklist-task-add, +tasklist-members
	"task": {
		"task:task:read", "task:task:write",
		"task:tasklist:read", "task:tasklist:write",
		"task:comment:write",
		"task:attachment:write", // task upload-attachment
	},

	// okr 命令（--as user 时）：cycle list/detail、progress list/get/create/update/delete、upload-image
	"okr": {
		"okr:okr.period:readonly",      // cycle list（okr/v1/periods）
		"okr:okr.content:readonly",     // cycle detail（v2 cycles/{id}/objectives、objectives/{id}/key_results）
		"okr:okr.progress:readonly",    // progress list / get
		"okr:okr.progress:writeonly",   // progress create / update
		"okr:okr.progress:delete",      // progress delete
		"okr:okr.progress.file:upload", // upload-image
	},

	// apps 命令（妙搭，仅 User Token）：list / create / update / access-scope / html-publish
	"apps": {
		"spark:app:read", "spark:app:write",
	},

	// markdown 命令（云盘原生 .md 文件）：create / fetch / overwrite / patch / diff
	"markdown": {
		"drive:drive.metadata:readonly", "drive:file:download", "drive:file:upload",
	},

	// im 命令补充（chat 别名同样生效）：msg flag list / create / cancel（必须 User Token）
	"im": {
		"im:feed.flag:read", "im:feed.flag:write",
	},

	// calendar shortcuts: +agenda, +create, +freebusy, +room-find, +rsvp, +suggestion
	"calendar": {
		"calendar:calendar.event:read",     // +agenda
		"calendar:calendar.event:create",   // +create
		"calendar:calendar.event:update",   // +create
		"calendar:calendar.event:reply",    // +rsvp
		"calendar:calendar.free_busy:read", // +freebusy, +room-find, +suggestion
	},

	// wiki shortcuts: +node-create, +move-docs-to-wiki, +update
	"wiki": {
		"wiki:node:create", "wiki:node:read", "wiki:node:update", "wiki:space:read",
		"wiki:node:move",        // wiki move-docs 需要
		"wiki:space:write_only", // wiki space update / delete-space（仅 User Token）
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
		"mail:user_mailbox.rule:read",   // rule-list / rule-get
		"mail:user_mailbox.rule:write",  // rule-create / rule-update / rule-delete / rule-reorder
	},

	// sheets shortcuts: +info, +read, +write, +append, +find, +create, +export, +merge-cells, etc.
	"sheets": {
		"sheets:spreadsheet:read", "sheets:spreadsheet:write_only", "sheets:spreadsheet:create",
		"docs:document:export", "drive:file:download",
	},

	// approval shortcuts: current official user-token definition/instance/task flows.
	"approval": {
		"approval:approval:read",
		"approval:instance:read", "approval:instance:write",
		"approval:task:read", "approval:task:write",
	},

	// 注意：attendance 顶层命令（user-task query / user-stats query）走 tenant_access_token
	// （larksuite/oapi-sdk-go v3.5.3 中 Attendance.UserTask.Query / UserStatsData.Query 的
	// SupportedAccessTokenTypes 仅含 Tenant），故不进入 auth login --domain 列表——
	// `attendance:task:readonly` 权限需在飞书开放平台「应用权限管理」页面授予应用。

	// doc shortcuts: +search, +create, +fetch, +update, +media-insert, +media-preview, +media-download
	"docs": {
		"search:docs:read",
		"docx:document:create", "docx:document:readonly", "docx:document:write_only",
		"docs:document.media:upload", "docs:document.media:download",
	},

	// slides 命令所需 scope（对齐官方 shortcuts/slides 各命令 Scopes；不依赖 meta/overlay 是否收录对应方法）:
	//   create → create / write_only（含图片 docs:document.media:upload）
	//   add-slide / delete-slide / replace-slide / update-slide → update / write_only
	//   get → read；screenshot → screenshot；media-upload → docs:document.media:upload
	// wiki URL 输入另需 wiki:node:read，属按输入追加的条件 scope，不放进域默认集合。
	"slides": {
		"slides:presentation:create", "slides:presentation:write_only",
		"slides:presentation:update", "slides:presentation:read",
		"slides:presentation:screenshot",
		"docs:document.media:upload",
	},

	// whiteboard shortcuts: +query, +update
	"whiteboard": {
		"board:whiteboard:node:read", "board:whiteboard:node:create", "board:whiteboard:node:delete",
	},

	// event shortcuts: list / schema / consume / status / stop
	// WebSocket 实时事件订阅；具体 scope 因 EventKey 而异，这里列出常用 EventKey 的并集。
	// 完整对照见 `feishu-cli event schema <key>`。
	"event": {
		// IM 事件最常用
		"im:message.p2p_msg:readonly",
		"im:message", "im:message:readonly", "im:message.reactions:read",
		"im:chat:read", "im:chat.members:read", "im:chat.members:bot_access",
		// 联系人
		"contact:user.base:readonly",
		// 日历
		"calendar:calendar.event:read", "calendar:calendar.acl:read",
		// 云盘
		"drive:drive",
		// 审批
		"approval:approval",
		// VC
		"vc:meeting",
	},
}

// domainDescriptions provides descriptions for alias/composite/fallback domains.
var domainDescriptions = map[string]struct{ Zh, En string }{
	"chat":       {"群聊、消息、Reaction/Pin、群管理", "Message, chat, reaction, pin & group management"},
	"bitable":    {"多维表格（Base 别名）", "Base / Bitable (alias)"},
	"drive":      {"云空间上传/下载/导出/导入/评论", "Drive upload, download, export, import & comments"},
	"doc_access": {"用户 Token 访问文档/知识库", "User Token document & wiki access"},
	"search":     {"文档和消息搜索", "Document and message search"},
	"event":      {"WebSocket 实时事件订阅（IM/联系人/日历/云盘/审批/VC）", "WebSocket real-time event subscription"},
	"okr":        {"OKR 周期、目标与进展记录", "OKR cycles, objectives & progress records"},
	"apps":       {"妙搭应用（spark）开发与发布", "Miaoda (spark) apps"},
	"markdown":   {"云盘原生 Markdown 文件读写", "Drive-native Markdown files"},
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

// batchExcludedScopes 是批量申请（--domain / --recommend）时一律剔除的 scope。
//
// im:message.send_as_user 在部分租户即使个人助手也需管理员审核，混在批量申请里会让整次授权
// 卡在审批；需要时用显式 --scope 单独申请（与官方 lark-cli login.go batchExcludedScopes 一致）。
var batchExcludedScopes = map[string]bool{
	"im:message.send_as_user": true,
}

// IsBatchExcludedScope 报告 scope 是否会在批量申请时被剔除。
func IsBatchExcludedScope(scope string) bool {
	return batchExcludedScopes[scope]
}

// FilterBatchExcludedScopes 从按业务域推导出的 scope 列表中剔除 batchExcludedScopes，保持原顺序。
func FilterBatchExcludedScopes(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if !batchExcludedScopes[s] {
			out = append(out, s)
		}
	}
	return out
}

// CollectDomainScopes collects scopes for the specified domains using the registry.
// It resolves aliases/composites, collects priority-based scopes from meta_data,
// expands auth_domain children, merges fallback scopes, and optionally filters
// to auto-approve scopes. 批量申请统一剔除 batchExcludedScopes（显式 --scope 不受影响）。
func CollectDomainScopes(domains []string, recommendedOnly bool) []string {
	return FilterBatchExcludedScopes(collectDomainScopes(domains, recommendedOnly))
}

// DomainScopeUniverse 返回业务域覆盖的全部 scope（不做推荐过滤，也不剔除 batchExcludedScopes）。
// 供 auth login --exclude 校验：排除一个已被批量策略剔除的 scope 是合法的空操作，而不是拼写错误。
func DomainScopeUniverse(domains []string) []string {
	return collectDomainScopes(domains, false)
}

func collectDomainScopes(domains []string, recommendedOnly bool) []string {
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
