# 日历和日程详细参考

## 时间格式

时间点推荐 **RFC3339**（带时区）：`2024-01-21T14:00:00+08:00`；写命令也接受 `2024-01-21 14:00`（本地时区）与 Unix 秒/毫秒。

全天日程（如请假）在 `get-event` / `list-events` / `agenda` 输出中 `is_all_day=true`，`start_time`/`end_time`
为 `YYYY-MM-DD`，结束日已换算为**包含端**（服务端 `end.date` 是排他的次日）。

带 `[calendar_id] <event_id>` 的命令（get-event / update-event / delete-event / event-share / event-transfer）
只传 event_id 时使用 `primary`（当前身份的主日历）。

## 日历操作

### 列出日历

```bash
feishu-cli calendar list [--page-size 20]
```

### 获取日历详情

```bash
feishu-cli calendar get <calendar_id> [-o json]
```

### 获取主日历

```bash
feishu-cli calendar primary [-o json]
```

## 日程 CRUD

### 创建日程

```bash
feishu-cli calendar create-event \
  [--calendar-id <id>] \
  --summary "会议标题" \
  --start "2024-01-21T14:00:00+08:00" \
  --end "2024-01-21T15:00:00+08:00" \
  [--description "会议描述"] \
  [--location "会议室名称"] \
  [--rrule "FREQ=WEEKLY;BYDAY=MO"] \
  [--attendee-ids ou_xxx,oc_xxx,omm_xxx] \
  [--vchat] [--dry-run] [--as auto|user|bot]
```

必填参数：`--summary`、`--start`、`--end`（`--end` 必须晚于 `--start`）；`--calendar-id` 默认 `primary`。

- `--attendee-ids` 按前缀识别：`ou_` 用户、`oc_` 群、`omm_` 会议室、含 `@` 为外部邮箱。日程创建后再添加参与人，
  **添加失败会自动删除刚创建的日程**（回滚，输出"已回滚"；回滚也失败时给出残留 event_id）。User 身份会把本人一并加入参与人。
- `--vchat` 同时创建飞书视频会议，`get-event` 的 `vchat.meeting_url` 为会议链接。

`--rrule` 传 RFC5545 RRULE 字符串创建重复日程，`--start`/`--end` 为首个实例时间。详见「重复日程（RRULE）操作指引」。

### 列出日程

```bash
feishu-cli calendar list-events \
  <calendar_id> \
  [--start-time "2024-01-01T00:00:00+08:00"] \
  [--end-time "2024-01-31T23:59:59+08:00"] \
  [--page-size 50] \
  [--page-token <token>]
```

### 获取日程详情

```bash
feishu-cli calendar get-event [calendar_id] <event_id> [--share-link] [-o json]
```

输出含 `self_rsvp_status`（本人答复）、`free_busy_status`、`vchat.meeting_url`（视频会议链接）、`reminders`、
`recurrence` / `is_exception` / `recurring_event_id`。

**分享日程用分享链接**：`--share-link` 或 `calendar event-share [calendar_id] <event_id>` 获取
`https://<domain>/calendar/share?token=...`。`app_link` 带查看者本人的 calendarId，只能本人打开，
不要用来分享给他人/群/文档，也不要自行拼接。share_info 偶发 190010（日历限流），CLI 已自动退避重试。

### 更新日程

```bash
feishu-cli calendar update-event \
  [calendar_id] \
  <event_id> \
  [--summary "新标题"] \
  [--start "2024-01-21T15:00:00+08:00" --end "2024-01-21T16:00:00+08:00"] \
  [--description "新描述"] \
  [--location "新地点"] \
  [--rrule "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"] \
  [--apply-to single|all|this-and-following] [--notify=false] [--dry-run]
```

至少提供一个可更新字段（含 `--rrule`）。**`--start` 与 `--end` 必须成对传**：实测只传一端时服务端返回成功但时间不变，
CLI 在本地直接报用法错误。重复日程的修改范围见「重复日程（RRULE）操作指引」。

### 删除日程

```bash
feishu-cli calendar delete-event [calendar_id] <event_id> [--apply-to single|all|this-and-following] [--notify=false] [--dry-run] [--yes]
```

删除前先读取日程判断类型，并在 stderr（`-o json` 的 `kind` / `scope` 字段）说明影响范围。
主日程 ID 删整条序列但**不级联**已单独修改过的例外，实例 ID 只删这一次；`--apply-to all` 连同例外一起删除。
详见「重复日程（RRULE）操作指引」。删除后服务端保留 `status=cancelled` 的记录，`get-event` 仍可查到但已非活动日程。

### 转让组织者

```bash
feishu-cli calendar event-transfer [calendar_id] <event_id> --to-user-id ou_xxx [--remove-original-organizer] [--transfer-series] --yes
```

不可撤销（纪要、附件一并转给新组织者），需 `--yes`。重复日程会转让整个序列，必须加 `--transfer-series` 表示知情。
`--as` 必须是日程当前组织者。日程在共享日历上时须传该日历 ID（193110）。

## 搜索日程

走 `POST /open-apis/calendar/v4/calendars/{id}/events/search_event`。`--calendar-id` 可省略（默认 `primary`），`--query` 可空（纯 filter 搜索）。`--start`/`--end` 写入 `filter.time_range`（RFC3339 或 YYYY-MM-DD；只给一边时补同一天边界；start>end 发网前失败）。

```bash
feishu-cli calendar event-search \
  [--calendar-id <id>] \
  [--query "关键词"] \
  [--start "2024-01-01T00:00:00+08:00"] \
  [--end "2024-12-31T23:59:59+08:00"] \
  [--attendee-ids ou_xxx,oc_xxx,omm_xxx] \
  [--page-size 20]
```

`--page-size` 范围 1-30（默认 20，越界报错不截断）。`--attendee-ids` 按前缀拆分：`ou_` → 用户、`oc_` → 群、`omm_` → 会议室。`-o json` 输出 `{events, next_page_token, has_more}`：`has_more` 取服务端字段，即使 `next_page_token` 为空也原样给出。

## 回复日程邀请

```bash
feishu-cli calendar event-reply <calendar_id> <event_id> --status <accept|decline|tentative>
```

与 `calendar rsvp` 等价，**必需 User Token**（答复是本人动作；此前版本默认 Bot 身份，必然答复失败）。

| 状态 | 说明 |
|------|------|
| `accept` | 接受 |
| `decline` | 拒绝 |
| `tentative` | 暂定 |

## 参与人管理

### 添加参与人

```bash
feishu-cli calendar attendee add <calendar_id> <event_id> \
  [--user-ids ou_xxx,ou_yyy] \
  [--chat-ids oc_xxx] \
  [--room-ids omm_xxx] \
  [--attendee-ids ou_xxx,oc_xxx,omm_xxx,user@example.com]
```

至少指定一类。会议室用 `--room-ids`（先 `calendar room-find` 找空闲会议室）；`--attendee-ids` 按前缀自动识别。

### 移除参与人

```bash
feishu-cli calendar attendee remove <calendar_id> <event_id> \
  [--user-ids ou_xxx] [--chat-ids oc_xxx] [--room-ids omm_xxx] \
  [--attendee-ids <ou_/oc_/omm_/邮箱/attendee_id>] [--notify=false] [--dry-run]
```

走 `attendees/batch_delete`；`--attendee-ids` 中无法按前缀识别的值视为 `attendee list` 返回的 `attendee_id`。

### 列出参与人

```bash
feishu-cli calendar attendee list <calendar_id> <event_id> \
  [--type user|chat|resource|third_party] [--page-all] [--page-size 50]
```

群参与人（`type=chat`）不输出 `rsvp_status`：服务端对群条目恒返回 `needs_action`，没有意义。

## 忙闲查询

```bash
feishu-cli calendar freebusy \
  [--start "2024-01-01T00:00:00+08:00"] \
  [--end "2024-01-02T00:00:00+08:00"] \
  [--user-id ou_xxx[,ou_yyy]] \
  [--type busy|raw_busy|free|common_free] [--min-duration 30m]
```

- `--start` 默认今天 00:00，`--end` 默认起始日当天结束；也接受 `YYYY-MM-DD`。
- `--user-id` 不传时默认**当前登录用户**；Bot 身份没有"本人"，必须指定（否则本地报用法错误）。
- `busy`（默认）：按时间排序并合并重叠/相邻区间（区间数 ≠ 日程数）；`raw_busy` 保留原始条目与 `rsvp_status`；
  `free` 每人空闲；`common_free` 多人共同空闲。
- JSON：单人 + busy 仍输出数组 `[{start_time,end_time}]`；多人或其他视图输出 `{"users":[...]}` / `{"common_free":[...]}`。

## 命令别名

`calendar` 命令支持别名 `cal`：

```bash
feishu-cli cal list
feishu-cli cal primary
```

## 权限要求

| 权限 | 说明 |
|------|------|
| `calendar:calendar:readonly` | 读取日历和日程 |
| `calendar:calendar` | 创建/修改/删除日程（需单独申请） |

## Token 策略

- **读类**（`calendar list/primary/get/freebusy/suggestion/room-find`、`calendar event get/list`、`calendar attendee list`）：登录后默认 User Token（自动从 `~/.feishu-cli/token.json` 加载），未登录回落 App Token。
- **身份可选 `--as bot|user|auto`**（`calendar agenda` / `calendar event-search`）：默认 auto（User 优先，未配置回落 Bot；已配置 User 但刷新失败 fail-closed，避免 `primary` 查到错误主体日历）。`--as bot` 走 App Token。
- **写类 `--as bot|user|auto`，默认 auto**（`calendar create-event/update-event/delete-event/event-share/event-transfer`、`calendar attendee add/remove`）：
  已登录用 User Token（操作本人日程），未配置 User Token 时回落 App Token，已配置但刷新失败 fail-closed。
  **行为变更**：此前版本这些写命令默认 App Token（Bot）——从 agenda 拿到本人日程 ID 再用 Bot 改删大概率失败。
  需要操作应用（Bot）日历时显式传 `--as bot`。
- **必需 User Token**：`calendar rsvp`、`calendar event-reply`（以本人身份答复邀请）。
