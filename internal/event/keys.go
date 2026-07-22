// Package event 提供飞书 WebSocket 长连接实时事件订阅能力。
//
// 设计要点：
//   - 静态 EventKey 目录（KeyDefinition）：覆盖 IM/Contact/Calendar/Drive/Approval 等常用事件
//   - 进程模型：每个 consume 命令 = 一个独立 OS 进程 + 一个 WebSocket 长连接（一个 EventKey）
//   - 状态文件：~/.feishu-cli/events/<app_id>/bus.json 记录所有 active 进程（PID + EventKey + 启动时间）
//   - 文件锁：bus.json 读写走 flock，避免多进程同时写入损坏
//   - 进程探活：status / stop 通过 PID 信号 0 检测进程是否存活
//
// 设计取舍：
//   - 不跑独立 bus 守护进程做事件 fan-out；feishu-cli 简化为
//     每个 consume 直接连 WebSocket（一个 EventKey 一个连接），不做事件分发——足够覆盖
//     AI Agent 单 EventKey 订阅的主线场景
//   - 重连策略复用 oapi-sdk-go v3 ws.Client.WithAutoReconnect（默认开启，无限重试）
package event

// KeyDefinition 描述一个可订阅的 EventKey。
//
// 字段说明：
//   - Key: feishu-cli CLI 层面的 EventKey（与 EventType 通常一致）
//   - EventType: oapi-sdk-go dispatcher 注册的事件类型（飞书开放平台定义的 schema.event_type）
//   - Description: 简短中文描述，list 命令展示
//   - Domain: 分组（im/contact/calendar/drive/approval/vc/meeting）
//   - Scopes: 所需 App scope；list / consume 时展示给用户
//   - AuthTypes: 可用身份类型（bot/user），schema 命令展示
//   - RequiredConsoleEvents: 开放平台后台必须勾选并发布的 EventType
//   - PayloadSchema: 事件 payload 的字段说明，schema 命令展示（手工 curated，避免引入 reflect 依赖）
type KeyDefinition struct {
	Key                   string   `json:"key"`
	EventType             string   `json:"event_type"`
	Description           string   `json:"description"`
	Domain                string   `json:"domain"`
	Scopes                []string `json:"scopes,omitempty"`
	AuthTypes             []string `json:"auth_types,omitempty"`
	RequiredConsoleEvents []string `json:"required_console_events,omitempty"`
	PayloadSchema         string   `json:"payload_schema,omitempty"`

	// CardCallback 为 true 时该 Key 走卡片回调帧（callback 分发通道），
	// 由 OnP2CardActionTrigger 处理而非 OnCustomizedEvent（两者是不同的 WS 帧类型）。
	CardCallback bool `json:"card_callback,omitempty"`

	// SubscribePath 非空时，consume 启动前需以 User 身份 POST 该端点完成服务端订阅注册
	// （否则连上 WS 也收不到事件）。对 SubscribeTypes 中每个类型各调一次，
	// body 形如 {"subscription_type": "<type>"}。订阅是持久的用户级关系，进程退出不注销。
	SubscribePath  string   `json:"subscribe_path,omitempty"`
	SubscribeTypes []string `json:"subscribe_types,omitempty"`
}

// keyRegistry 是手工维护的常用 EventKey 列表。
//
// 新增 EventKey 时只需追加一条；feishu-cli event list 会自动按 Domain 分组展示。
// 参考飞书开放平台事件订阅文档：https://open.feishu.cn/document/server-docs/event-subscription-guide/event-list
var keyRegistry = []KeyDefinition{
	// ---------- IM 消息 ----------
	{
		Key:         "im.message.receive_v1",
		EventType:   "im.message.receive_v1",
		Description: "接收消息（用户/群聊发给 Bot 的消息）",
		Domain:      "im",
		Scopes:      []string{"im:message.p2p_msg:readonly"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"im.message.receive_v1",
		},
		PayloadSchema: `{
  "schema": "2.0",
  "header": {"event_id": "...", "event_type": "im.message.receive_v1", "create_time": "..."},
  "event": {
    "sender": {"sender_id": {"open_id": "ou_xxx"}, "sender_type": "user"},
    "message": {
      "message_id": "om_xxx",
      "chat_id": "oc_xxx",
      "chat_type": "p2p|group",
      "message_type": "text|post|image|...",
      "content": "{\"text\":\"hello\"}"
    }
  }
}`,
	},
	{
		Key:         "im.message.message_read_v1",
		EventType:   "im.message.message_read_v1",
		Description: "消息已读回执",
		Domain:      "im",
		Scopes:      []string{"im:message", "im:message:readonly"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"im.message.message_read_v1",
		},
	},
	{
		Key:         "im.message.recalled_v1",
		EventType:   "im.message.recalled_v1",
		Description: "消息被撤回",
		Domain:      "im",
		Scopes:      []string{"im:message:readonly"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"im.message.recalled_v1",
		},
	},
	{
		Key:         "im.message.reaction.created_v1",
		EventType:   "im.message.reaction.created_v1",
		Description: "消息表情回复被添加",
		Domain:      "im",
		Scopes:      []string{"im:message:readonly", "im:message.reactions:read"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"im.message.reaction.created_v1",
		},
	},
	{
		Key:         "im.message.reaction.deleted_v1",
		EventType:   "im.message.reaction.deleted_v1",
		Description: "消息表情回复被删除",
		Domain:      "im",
		Scopes:      []string{"im:message:readonly", "im:message.reactions:read"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"im.message.reaction.deleted_v1",
		},
	},
	{
		Key:         "im.chat.updated_v1",
		EventType:   "im.chat.updated_v1",
		Description: "群聊信息更新",
		Domain:      "im",
		Scopes:      []string{"im:chat:read"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"im.chat.updated_v1",
		},
	},
	{
		Key:         "im.chat.member.user.added_v1",
		EventType:   "im.chat.member.user.added_v1",
		Description: "用户进群",
		Domain:      "im",
		Scopes:      []string{"im:chat.members:read"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"im.chat.member.user.added_v1",
		},
	},
	{
		Key:         "im.chat.member.user.deleted_v1",
		EventType:   "im.chat.member.user.deleted_v1",
		Description: "用户离群",
		Domain:      "im",
		Scopes:      []string{"im:chat.members:read"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"im.chat.member.user.deleted_v1",
		},
	},
	{
		Key:         "im.chat.member.bot.added_v1",
		EventType:   "im.chat.member.bot.added_v1",
		Description: "Bot 被拉入群",
		Domain:      "im",
		Scopes:      []string{"im:chat.members:bot_access"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"im.chat.member.bot.added_v1",
		},
	},
	{
		Key:         "im.chat.member.bot.deleted_v1",
		EventType:   "im.chat.member.bot.deleted_v1",
		Description: "Bot 被移出群",
		Domain:      "im",
		Scopes:      []string{"im:chat.members:bot_access"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"im.chat.member.bot.deleted_v1",
		},
	},
	{
		Key:         "im.chat.disbanded_v1",
		EventType:   "im.chat.disbanded_v1",
		Description: "群聊被解散",
		Domain:      "im",
		Scopes:      []string{"im:chat:read"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"im.chat.disbanded_v1",
		},
	},

	// ---------- 联系人 ----------
	{
		Key:         "contact.user.created_v3",
		EventType:   "contact.user.created_v3",
		Description: "新增员工",
		Domain:      "contact",
		Scopes:      []string{"contact:user.base:readonly"},
	},
	{
		Key:         "contact.user.updated_v3",
		EventType:   "contact.user.updated_v3",
		Description: "员工信息变更",
		Domain:      "contact",
		Scopes:      []string{"contact:user.base:readonly"},
	},
	{
		Key:         "contact.user.deleted_v3",
		EventType:   "contact.user.deleted_v3",
		Description: "员工离职",
		Domain:      "contact",
		Scopes:      []string{"contact:user.base:readonly"},
	},

	// ---------- 日历 ----------
	{
		Key:         "calendar.calendar.event.changed_v4",
		EventType:   "calendar.calendar.event.changed_v4",
		Description: "日程变更（创建/更新/删除）",
		Domain:      "calendar",
		Scopes:      []string{"calendar:calendar.event:read"},
	},
	{
		Key:         "calendar.calendar.acl.created_v4",
		EventType:   "calendar.calendar.acl.created_v4",
		Description: "日历权限变更",
		Domain:      "calendar",
		Scopes:      []string{"calendar:calendar.acl:read"},
	},

	// ---------- 云盘 ----------
	{
		Key:         "drive.file.title_updated_v1",
		EventType:   "drive.file.title_updated_v1",
		Description: "文档标题修改",
		Domain:      "drive",
		Scopes:      []string{"drive:drive"},
	},
	{
		Key:         "drive.file.permission_member_added_v1",
		EventType:   "drive.file.permission_member_added_v1",
		Description: "文档协作者添加",
		Domain:      "drive",
		Scopes:      []string{"drive:drive"},
	},

	// ---------- 交互回调 ----------
	{
		Key:         "card.action.trigger",
		EventType:   "card.action.trigger",
		Description: "卡片交互回调（按钮点击/表单提交/下拉选择等），交互式 Bot 的核心事件",
		Domain:      "im",
		Scopes:      []string{"im:message"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"card.action.trigger",
		},
		CardCallback: true,
		PayloadSchema: `{
  "schema": "2.0",
  "header": {"event_id": "...", "event_type": "card.action.trigger", "token": "..."},
  "event": {
    "operator": {"open_id": "ou_xxx"},
    "token": "卡片更新凭证（可用于回写卡片）",
    "action": {
      "tag": "button|input|select_static|...",
      "value": {"自定义键": "值"},
      "form_value": {"表单字段": "值"}
    },
    "context": {"open_message_id": "om_xxx", "open_chat_id": "oc_xxx"}
  }
}`,
	},
	{
		Key:         "application.bot.menu_v6",
		EventType:   "application.bot.menu_v6",
		Description: "用户点击 Bot 自定义菜单",
		Domain:      "application",
		Scopes:      []string{"im:message"},
		AuthTypes:   []string{"bot"},
		RequiredConsoleEvents: []string{
			"application.bot.menu_v6",
		},
		PayloadSchema: `{
  "schema": "2.0",
  "header": {"event_id": "...", "event_type": "application.bot.menu_v6"},
  "event": {
    "operator": {"operator_id": {"open_id": "ou_xxx"}},
    "event_key": "自定义菜单的 event_key",
    "timestamp": 1700000000
  }
}`,
	},

	// ---------- 审批 ----------
	// 注意：审批事件除后台勾选事件外，还需以 User 身份注册服务端订阅关系
	// （consume 会自动完成，需已 auth login）；订阅是持久用户级关系，进程退出不注销。
	{
		Key:         "approval.instance.status_changed_v4",
		EventType:   "approval.instance.status_changed_v4",
		Description: "审批实例状态变更（对发起人/审批参与人可见时触发）",
		Domain:      "approval",
		Scopes:      []string{"approval:instance:read"},
		AuthTypes:   []string{"user"},
		RequiredConsoleEvents: []string{
			"approval.instance.status_changed_v4",
		},
		SubscribePath:  "/open-apis/approval/v4/instances/subscription",
		SubscribeTypes: []string{"INVOLVED_APPROVAL", "MANAGED_APPROVAL"},
	},
	{
		Key:         "approval.task.status_changed_v4",
		EventType:   "approval.task.status_changed_v4",
		Description: "审批任务状态变更（对发起人/任务审批人可见时触发）",
		Domain:      "approval",
		Scopes:      []string{"approval:task:read"},
		AuthTypes:   []string{"user"},
		RequiredConsoleEvents: []string{
			"approval.task.status_changed_v4",
		},
		SubscribePath:  "/open-apis/approval/v4/tasks/subscription",
		SubscribeTypes: []string{"INVOLVED_APPROVAL", "MANAGED_APPROVAL"},
	},

	// ---------- 视频会议 ----------
	{
		Key:         "vc.meeting.meeting_started_v1",
		EventType:   "vc.meeting.meeting_started_v1",
		Description: "VC 会议开始",
		Domain:      "vc",
		Scopes:      []string{"vc:meeting"},
	},
	{
		Key:         "vc.meeting.meeting_ended_v1",
		EventType:   "vc.meeting.meeting_ended_v1",
		Description: "VC 会议结束",
		Domain:      "vc",
		Scopes:      []string{"vc:meeting"},
	},
}

// ListAll 返回所有已注册 EventKey，按 Domain + Key 排序（list 命令使用）。
func ListAll() []KeyDefinition {
	out := make([]KeyDefinition, len(keyRegistry))
	copy(out, keyRegistry)
	return out
}

// Lookup 按 Key 查找 EventKey 定义；返回 (def, true) 表示命中，否则 (零值, false)。
func Lookup(key string) (KeyDefinition, bool) {
	for _, def := range keyRegistry {
		if def.Key == key {
			return def, true
		}
	}
	return KeyDefinition{}, false
}

// Domains 返回所有出现过的 Domain，去重后按字典序排序（list 分组展示用）。
func Domains() []string {
	seen := map[string]bool{}
	var out []string
	for _, def := range keyRegistry {
		if !seen[def.Domain] {
			seen[def.Domain] = true
			out = append(out, def.Domain)
		}
	}
	return out
}
