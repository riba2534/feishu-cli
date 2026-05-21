// Package event 提供飞书 WebSocket 长连接实时事件订阅能力。
//
// 设计要点：
//   - 静态 EventKey 目录（KeyDefinition）：覆盖 IM/Contact/Calendar/Drive/Approval 等常用事件
//   - 进程模型：每个 consume 命令 = 一个独立 OS 进程 + 一个 WebSocket 长连接（一个 EventKey）
//   - 状态文件：~/.feishu-cli/events/<app_id>/bus.json 记录所有 active 进程（PID + EventKey + 启动时间）
//   - 文件锁：bus.json 读写走 flock，避免多进程同时写入损坏
//   - 进程探活：status / stop 通过 PID 信号 0 检测进程是否存活
//
// 与 lark-cli 的差异：
//   - lark-cli 用 Unix domain socket 跑独立 bus 守护进程做事件 fan-out；feishu-cli 简化为
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
//   - PayloadSchema: 事件 payload 的字段说明，schema 命令展示（手工 curated，避免引入 reflect 依赖）
type KeyDefinition struct {
	Key           string   `json:"key"`
	EventType     string   `json:"event_type"`
	Description   string   `json:"description"`
	Domain        string   `json:"domain"`
	Scopes        []string `json:"scopes,omitempty"`
	PayloadSchema string   `json:"payload_schema,omitempty"`
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
		Scopes:      []string{"im:message", "im:message.group_msg"},
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
	},
	{
		Key:         "im.message.recalled_v1",
		EventType:   "im.message.recalled_v1",
		Description: "消息被撤回",
		Domain:      "im",
		Scopes:      []string{"im:message"},
	},
	{
		Key:         "im.message.reaction.created_v1",
		EventType:   "im.message.reaction.created_v1",
		Description: "消息表情回复被添加",
		Domain:      "im",
		Scopes:      []string{"im:message"},
	},
	{
		Key:         "im.message.reaction.deleted_v1",
		EventType:   "im.message.reaction.deleted_v1",
		Description: "消息表情回复被删除",
		Domain:      "im",
		Scopes:      []string{"im:message"},
	},
	{
		Key:         "im.chat.updated_v1",
		EventType:   "im.chat.updated_v1",
		Description: "群聊信息更新",
		Domain:      "im",
		Scopes:      []string{"im:chat", "im:chat:readonly"},
	},
	{
		Key:         "im.chat.member.user.added_v1",
		EventType:   "im.chat.member.user.added_v1",
		Description: "用户进群",
		Domain:      "im",
		Scopes:      []string{"im:chat", "im:chat.members"},
	},
	{
		Key:         "im.chat.member.user.deleted_v1",
		EventType:   "im.chat.member.user.deleted_v1",
		Description: "用户离群",
		Domain:      "im",
		Scopes:      []string{"im:chat", "im:chat.members"},
	},
	{
		Key:         "im.chat.member.bot.added_v1",
		EventType:   "im.chat.member.bot.added_v1",
		Description: "Bot 被拉入群",
		Domain:      "im",
		Scopes:      []string{"im:chat"},
	},
	{
		Key:         "im.chat.member.bot.deleted_v1",
		EventType:   "im.chat.member.bot.deleted_v1",
		Description: "Bot 被移出群",
		Domain:      "im",
		Scopes:      []string{"im:chat"},
	},
	{
		Key:         "im.chat.disbanded_v1",
		EventType:   "im.chat.disbanded_v1",
		Description: "群聊被解散",
		Domain:      "im",
		Scopes:      []string{"im:chat"},
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

	// ---------- 审批 ----------
	{
		Key:         "approval_instance",
		EventType:   "approval_instance",
		Description: "审批实例状态变更",
		Domain:      "approval",
		Scopes:      []string{"approval:approval"},
	},
	{
		Key:         "approval_task",
		EventType:   "approval_task",
		Description: "审批任务变更",
		Domain:      "approval",
		Scopes:      []string{"approval:approval"},
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
