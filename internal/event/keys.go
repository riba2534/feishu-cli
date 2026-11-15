// Package event 提供飞书 WebSocket 长连接实时事件订阅能力。
//
// 设计要点：
//   - 静态 EventKey 目录（KeyDefinition）：覆盖 IM/Contact/Calendar/Drive/Approval 等常用事件
//   - 进程模型：每个 consume 命令 = 一个 OS 进程 + 一条 WebSocket 长连接，可同时订阅多个 EventKey
//     （event consume k1 k2）。飞书按 App 维度把事件随机投递给该 App 的任一条连接，
//     同 App 多连接会互相抢事件，因此同一 App 同一机器只允许一个 consume 进程（单实例锁，
//     --force 可绕过但不安全），未订阅的已知事件类型 ACK 后本地丢弃
//   - 状态文件：~/.feishu-cli/events/<app_id>/bus.json 记录 active 进程（PID + EventKey + 启动时间，
//     多 key 进程每个 key 一条记录）
//   - 文件锁：bus.json 读写走 flock，避免多进程同时写入损坏；consume.lock 为单实例锁
//   - 进程探活：status / stop 通过 PID 信号 0 检测进程是否存活
//   - 事件去重：按 event_id（TTL 5 分钟）去重，服务端重投不会重复输出
//
// 设计取舍：
//   - 不跑独立 bus 守护进程做事件 fan-out（官方方案）；feishu-cli 简化为单进程单连接多 key
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
	// （否则连上 WS 也收不到事件）。
	//
	//   - SubscribeEventType=true：body 为 {"event_type": EventType}（VC/note/recording）。
	//   - SubscribeTypes 非空：对每个类型各调一次，body 形如 {"subscription_type": "<type>"}（审批 v4）。
	//
	// UnsubscribePath 非空时，进程退出会 best-effort POST 注销（VC 订阅是会话级）；
	// 审批订阅是持久用户级关系，不填 UnsubscribePath。
	SubscribePath      string   `json:"subscribe_path,omitempty"`
	UnsubscribePath    string   `json:"unsubscribe_path,omitempty"`
	SubscribeTypes     []string `json:"subscribe_types,omitempty"`
	SubscribeEventType bool     `json:"subscribe_event_type,omitempty"`

	// SubscribeNoBody 为 true 时订阅请求不带 body（任务 task_subscription）。
	SubscribeNoBody bool `json:"subscribe_no_body,omitempty"`
	// SubscribeBody 非空时作为固定订阅/退订 body（如邮箱 {"event_type":1}），优先于上面两种 body 规则。
	SubscribeBody map[string]any `json:"subscribe_body,omitempty"`

	// Params 声明 event consume --param k=v 可接受的参数；SubscribePath/UnsubscribePath
	// 中的 {name} 占位符按参数值（路径段转义后）替换。
	Params []ParamDef `json:"params,omitempty"`

	// Match 非空时对收到的事件做本地过滤（如画板按 whiteboard_id 过滤），返回 false 的事件 ACK 后丢弃。
	Match func(payload []byte, params map[string]string) bool `json:"-"`
}

// ParamDef 描述一个 --param 参数。
type ParamDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Multi       bool     `json:"multi,omitempty"`   // 允许逗号分隔多值或重复传入
	Values      []string `json:"values,omitempty"`  // 非空时为枚举
	Default     string   `json:"default,omitempty"` // 未传时的默认值
}

// HasSubscription 表示该 key 在 consume 前需要注册服务端订阅。
func (d KeyDefinition) HasSubscription() bool {
	return d.SubscribePath != ""
}

// IdentityTypes 返回该 key 可用于 --as 的身份。需要服务端订阅但未声明 AuthTypes 时默认 [user]；
// 不需要订阅且未声明时返回 nil（不限制，WebSocket 本身走 App 凭证）。
func (d KeyDefinition) IdentityTypes() []string {
	if len(d.AuthTypes) > 0 {
		return d.AuthTypes
	}
	if d.HasSubscription() {
		return []string{"user"}
	}
	return nil
}

// AllowsIdentity 判断 identity（user/bot）是否可用于该 key。
func (d KeyDefinition) AllowsIdentity(identity string) bool {
	types := d.IdentityTypes()
	if len(types) == 0 {
		return true
	}
	for _, t := range types {
		if t == identity {
			return true
		}
	}
	return false
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
		Params:         approvalSubscriptionParams,
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
		Params:         approvalSubscriptionParams,
	},

	// ---------- 视频会议（用户参会生命周期 + 纪要/录制；均需 User 身份 pre-consume）----------
	{
		Key:         "vc.meeting.participant_meeting_started_v1",
		EventType:   "vc.meeting.participant_meeting_started_v1",
		Description: "当前用户参与的会议已开始",
		Domain:      "vc",
		Scopes:      []string{"vc:meeting.meetingevent:read"},
		AuthTypes:   []string{"user"},
		RequiredConsoleEvents: []string{
			"vc.meeting.participant_meeting_started_v1",
		},
		SubscribePath:      "/open-apis/vc/v1/meetings/subscription",
		UnsubscribePath:    "/open-apis/vc/v1/meetings/unsubscription",
		SubscribeEventType: true,
	},
	{
		Key:         "vc.meeting.participant_meeting_joined_v1",
		EventType:   "vc.meeting.participant_meeting_joined_v1",
		Description: "当前用户已加入会议",
		Domain:      "vc",
		Scopes:      []string{"vc:meeting.meetingevent:read"},
		AuthTypes:   []string{"user"},
		RequiredConsoleEvents: []string{
			"vc.meeting.participant_meeting_joined_v1",
		},
		SubscribePath:      "/open-apis/vc/v1/meetings/subscription",
		UnsubscribePath:    "/open-apis/vc/v1/meetings/unsubscription",
		SubscribeEventType: true,
	},
	{
		Key:         "vc.meeting.participant_meeting_ended_v1",
		EventType:   "vc.meeting.participant_meeting_ended_v1",
		Description: "当前用户参与的会议已结束",
		Domain:      "vc",
		Scopes:      []string{"vc:meeting.meetingevent:read"},
		AuthTypes:   []string{"user"},
		RequiredConsoleEvents: []string{
			"vc.meeting.participant_meeting_ended_v1",
		},
		SubscribePath:      "/open-apis/vc/v1/meetings/subscription",
		UnsubscribePath:    "/open-apis/vc/v1/meetings/unsubscription",
		SubscribeEventType: true,
	},
	{
		Key:         "vc.note.generated_v1",
		EventType:   "vc.note.generated_v1",
		Description: "智能纪要已生成（会议/录制/上传等来源）",
		Domain:      "vc",
		Scopes:      []string{"vc:note:read"},
		AuthTypes:   []string{"user"},
		RequiredConsoleEvents: []string{
			"vc.note.generated_v1",
		},
		SubscribePath:      "/open-apis/vc/v1/notes/subscription",
		UnsubscribePath:    "/open-apis/vc/v1/notes/unsubscription",
		SubscribeEventType: true,
	},
	{
		Key:         "vc.recording.recording_started_v1",
		EventType:   "vc.recording.recording_started_v1",
		Description: "录制开始（recording_bean；仅飞书软件侧）",
		Domain:      "vc",
		Scopes:      []string{"vc:recording:read"},
		AuthTypes:   []string{"user"},
		RequiredConsoleEvents: []string{
			"vc.recording.recording_started_v1",
		},
		SubscribePath:      "/open-apis/vc/v1/recordings/subscription",
		UnsubscribePath:    "/open-apis/vc/v1/recordings/unsubscription",
		SubscribeEventType: true,
	},
	{
		Key:         "vc.recording.recording_transcript_generated_v1",
		EventType:   "vc.recording.recording_transcript_generated_v1",
		Description: "录制逐字稿条目已生成（recording_bean；仅飞书软件侧）",
		Domain:      "vc",
		Scopes:      []string{"vc:recording:read"},
		AuthTypes:   []string{"user"},
		RequiredConsoleEvents: []string{
			"vc.recording.recording_transcript_generated_v1",
		},
		SubscribePath:      "/open-apis/vc/v1/recordings/subscription",
		UnsubscribePath:    "/open-apis/vc/v1/recordings/unsubscription",
		SubscribeEventType: true,
	},
	{
		Key:         "vc.recording.recording_ended_v1",
		EventType:   "vc.recording.recording_ended_v1",
		Description: "录制结束并上传成功（recording_bean；仅飞书软件侧）",
		Domain:      "vc",
		Scopes:      []string{"vc:recording:read"},
		AuthTypes:   []string{"user"},
		RequiredConsoleEvents: []string{
			"vc.recording.recording_ended_v1",
		},
		SubscribePath:      "/open-apis/vc/v1/recordings/subscription",
		UnsubscribePath:    "/open-apis/vc/v1/recordings/unsubscription",
		SubscribeEventType: true,
	},

	// ---------- 妙记（User 身份 pre-consume，退出时注销）----------
	{
		Key:         "minutes.minute.generated_v1",
		EventType:   "minutes.minute.generated_v1",
		Description: "妙记已生成（当前用户可见的妙记）",
		Domain:      "minutes",
		Scopes:      []string{"minutes:minutes.basic:read"},
		AuthTypes:   []string{"user"},
		RequiredConsoleEvents: []string{
			"minutes.minute.generated_v1",
		},
		SubscribePath:      "/open-apis/minutes/v1/minutes/subscription",
		UnsubscribePath:    "/open-apis/minutes/v1/minutes/unsubscription",
		SubscribeEventType: true,
	},

	// ---------- 云文档评论（user/bot 均可；订阅按 App+身份+事件类型共享，退出时不注销）----------
	{
		Key:         "drive.notice.comment_add_v1",
		EventType:   "drive.notice.comment_add_v1",
		Description: "云文档评论/回复通知了当前用户或 Bot（如评论里 @ 了它）",
		Domain:      "drive",
		Scopes:      []string{"docs:document.comment:read"},
		AuthTypes:   []string{"user", "bot"},
		RequiredConsoleEvents: []string{
			"drive.notice.comment_add_v1",
		},
		SubscribePath:      "/open-apis/drive/v1/user/subscription",
		SubscribeEventType: true,
	},

	// ---------- 任务（user/bot 均可；订阅无 body，退出时不注销）----------
	{
		Key:         "task.task.update_user_access_v2",
		EventType:   "task.task.update_user_access_v2",
		Description: "当前用户或应用可见的任务被创建/删除/更新",
		Domain:      "task",
		Scopes:      []string{"task:task:read"},
		AuthTypes:   []string{"user", "bot"},
		RequiredConsoleEvents: []string{
			"task.task.update_user_access_v2",
		},
		SubscribePath:   "/open-apis/task/v2/task_v2/task_subscription?user_id_type=open_id",
		SubscribeNoBody: true,
	},

	// ---------- 画板（按画板订阅，必填 whiteboard_id；退出时注销）----------
	{
		Key:         "board.whiteboard.updated_v1",
		EventType:   "board.whiteboard.updated_v1",
		Description: "画板内容更新（需 --param whiteboard_id=<id>，只输出该画板的事件）",
		Domain:      "board",
		Scopes:      []string{"board:whiteboard:node:read"},
		AuthTypes:   []string{"user", "bot"},
		RequiredConsoleEvents: []string{
			"board.whiteboard.updated_v1",
		},
		Params: []ParamDef{
			{Name: "whiteboard_id", Required: true, Description: "要订阅的画板 ID，订阅按画板维度"},
		},
		SubscribePath:      "/open-apis/board/v1/whiteboards/{whiteboard_id}/subscribe",
		UnsubscribePath:    "/open-apis/board/v1/whiteboards/{whiteboard_id}/unsubscribe",
		SubscribeEventType: true,
		Match:              matchWhiteboardID,
		PayloadSchema: `{
  "schema": "2.0",
  "header": {"event_id": "...", "event_type": "board.whiteboard.updated_v1"},
  "event": {
    "whiteboard_id": "画板 ID",
    "operator_ids": [{"open_id": "ou_xxx", "union_id": "on_xxx", "user_id": "xxx"}]
  }
}`,
	},

	// ---------- 邮箱（User 身份订阅邮箱事件，退出时注销；复用本进程的单条长连接）----------
	{
		Key:         "mail.user_mailbox.event.message_received_v1",
		EventType:   "mail.user_mailbox.event.message_received_v1",
		Description: "邮箱收到新邮件（事件只含 mail_address/message_id 等元数据，正文用 mail message 读取）",
		Domain:      "mail",
		Scopes:      []string{"mail:event", "mail:user_mailbox.event.mail_address:read"},
		AuthTypes:   []string{"user"},
		RequiredConsoleEvents: []string{
			"mail.user_mailbox.event.message_received_v1",
		},
		Params: []ParamDef{
			{Name: "mailbox", Default: "me", Description: "要订阅的邮箱地址，默认 me（当前用户主邮箱）"},
		},
		SubscribePath:   "/open-apis/mail/v1/user_mailboxes/{mailbox}/event/subscribe",
		UnsubscribePath: "/open-apis/mail/v1/user_mailboxes/{mailbox}/event/unsubscribe",
		SubscribeBody:   map[string]any{"event_type": 1},
	},
}

// approvalSubscriptionParams 审批订阅关系类型：不传时两种都注册（与旧版一致）。
var approvalSubscriptionParams = []ParamDef{
	{
		Name:        "subscription_type",
		Multi:       true,
		Values:      []string{"INVOLVED_APPROVAL", "MANAGED_APPROVAL"},
		Description: "要注册的审批订阅关系（逗号分隔多值）；不传时 INVOLVED_APPROVAL 与 MANAGED_APPROVAL 都注册",
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

// commonAckEventTypes 是官方 catch-all 模式登记的常见事件类型（shortcuts/event/subscribe.go），
// 与本目录 keyRegistry 取并集后作为"未订阅也 ACK"的集合。
var commonAckEventTypes = []string{
	"im.message.receive_v1",
	"im.message.message_read_v1",
	"im.message.reaction.created_v1",
	"im.message.reaction.deleted_v1",
	"im.chat.member.bot.added_v1",
	"im.chat.member.bot.deleted_v1",
	"im.chat.member.user.added_v1",
	"im.chat.member.user.withdrawn_v1",
	"im.chat.member.user.deleted_v1",
	"im.chat.updated_v1",
	"im.chat.disbanded_v1",
	"contact.user.created_v3",
	"contact.user.updated_v3",
	"contact.user.deleted_v3",
	"contact.department.created_v3",
	"contact.department.updated_v3",
	"contact.department.deleted_v3",
	"calendar.calendar.acl.created_v4",
	"calendar.calendar.event.changed_v4",
	"approval.approval.updated",
	"application.application.visibility.added_v6",
	"task.task.update_tenant_v1",
	"task.task.comment_updated_v1",
}

// ackOnlyEventTypes 返回需要"收到即 ACK"的已知事件类型（不含卡片回调），去重保序。
func ackOnlyEventTypes() []string {
	seen := map[string]bool{}
	var out []string
	add := func(et string) {
		if et == "" || seen[et] {
			return
		}
		seen[et] = true
		out = append(out, et)
	}
	for _, def := range keyRegistry {
		if def.CardCallback {
			continue
		}
		add(def.EventType)
	}
	for _, et := range commonAckEventTypes {
		add(et)
	}
	return out
}
