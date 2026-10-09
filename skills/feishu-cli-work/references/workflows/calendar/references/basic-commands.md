# 日历和日程命令参考

只列非显然的参数、默认值与约束；完整参数以 `feishu-cli calendar <cmd> --help` 为准。`calendar` 有别名 `cal`。

## 时间格式

- 写命令（`create-event` / `update-event`）的时间点推荐带时区的 **RFC3339**（`2026-01-21T14:00:00+08:00`）；
  也接受 `2026-01-21 14:00`（本地时区）与 Unix 秒/毫秒。传 `YYYY-MM-DD` 会被当作当天 00:00 的具体时刻，**不会**建成全天日程。
- 全天日程在 `get-event` / `agenda` 输出中 `is_all_day=true`，`start_time`/`end_time` 为 `YYYY-MM-DD`，结束日已换算为
  **包含端**（服务端 `end.date` 是排他的次日）；文本输出为「全天日程 起 ~ 止」。
- 带 `[calendar_id] <event_id>` 的命令（get-event / update-event / delete-event / event-share / event-transfer）只传
  event_id 时使用 `primary`（当前身份的主日历）。

## 日历

```bash
feishu-cli calendar list [-o json]          # --page-size 最小 50（服务端校验），默认 50
feishu-cli calendar get <calendar_id> [-o json]
feishu-cli calendar primary [-o json]       # 当前身份的主日历
```

## 日程 CRUD

### 创建日程

```bash
feishu-cli calendar create-event \
  [--calendar-id <id>] \
  --summary "会议标题" \
  --start "2026-01-21T14:00:00+08:00" \
  --end "2026-01-21T15:00:00+08:00" \
  [--description "会议描述"] [--location "会议室名称"] \
  [--rrule "FREQ=WEEKLY;BYDAY=MO"] \
  [--attendee-ids ou_xxx,oc_xxx,omm_xxx,user@example.com] \
  [--vchat] [--dry-run] [--as auto|user|bot] [-o json]
```

- 必填 `--summary`、`--start`、`--end`（`--end` 必须晚于 `--start`）；`--calendar-id` 默认 `primary`。
- `--attendee-ids` 按前缀识别：`ou_` 用户、`oc_` 群、`omm_` 会议室、含 `@` 为外部邮箱。日程创建后再添加参与人并立即通知；
  **添加失败会自动删除刚创建的日程**（输出"回滚成功"；回滚也失败时给出残留 event_id，需手动删除）。User 身份会把本人一并加入参与人。
- `--vchat` 同时创建飞书视频会议，`get-event` 的 `vchat.meeting_url` 为会议链接。
- `--dry-run` 只打印将发出的请求（不联网、不解析身份），带参与人时会列出第二步添加参与人的请求。
- `--rrule` 创建重复日程，`--start`/`--end` 为首个实例时间，见 workflow.md 的「重复日程（RRULE）操作指引」。

### 列出日程（原始日程）

```bash
feishu-cli calendar list-events <calendar_id> \
  [--start-time "2026-01-01T00:00:00+08:00"] [--end-time "2026-01-31T23:59:59+08:00"] \
  [--page-size 50] [--page-token <token>] [-o json]
```

返回日历里的原始日程：重复日程只有主体（起始时间可能是多年前），已删除日程以 `status=cancelled` 出现。
按天/按时间段看安排请用 `calendar agenda`。

### 获取日程详情

```bash
feishu-cli calendar get-event [calendar_id] <event_id> [--share-link] [-o json]
```

- `event_id` 也接受重复日程的实例 ID。输出含 `self_rsvp_status`（本人答复）、`free_busy_status`、`vchat.meeting_url`、
  `reminders`、`recurrence` / `is_exception` / `recurring_event_id`。**不含参与人**，参与人用 `attendee list`。
- **分享日程用分享链接**：`--share-link`（JSON 增加 `share_link`）或 `calendar event-share [calendar_id] <event_id>`。
  文本输出不显示 `app_link`；`-o json` 仍带 `app_link` 字段，但它带查看者本人的 calendarId，只能本人打开，不要用来分享，也不要自行拼接。
  share_info 偶发 190010（日历限流），CLI 已自动退避重试。

### 更新日程

```bash
feishu-cli calendar update-event [calendar_id] <event_id> \
  [--summary "新标题"] \
  [--start "2026-01-21T15:00:00+08:00" --end "2026-01-21T16:00:00+08:00"] \
  [--description "新描述"] [--location "新地点"] \
  [--rrule "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"] \
  [--apply-to single|all|this-and-following] [--notify=false] [--dry-run] [--yes] [-o json]
```

- 至少提供一个可更新字段（含 `--rrule`）。**`--start` 与 `--end` 必须成对传**：只改一端时服务端返回成功但时间不变，
  CLI 在本地报用法错误（exit 2）。
- `-o json`：不传 `--apply-to` 或 `single` 时输出更新后的日程对象；`all` / `this-and-following` 输出
  `{kind, scope, master_event_id, follow_event, exceptions, ...}` 汇总。重复日程范围见 workflow.md。

### 删除日程

```bash
feishu-cli calendar delete-event [calendar_id] <event_id> \
  [--apply-to single|all|this-and-following] [--notify=false] [--dry-run] [--yes] [-o json]
```

- 删除前先读取日程判型，`-o json` 输出 `{kind, apply_to, scope, master_event_id, recurrence_truncated, exceptions}`；
  `kind` 取值 `normal` / `master` / `instance` / `exception`。
- 主体 ID 不带 `--apply-to` 删整条序列但**不级联**已单独修改过的例外；实例 ID 只删这一次；`--apply-to all` 连同例外一起删除。
- 只有 `all` / `this-and-following` 需要 `--yes`；普通删除不需要确认，执行前自行向用户确认目标。
- 删除后服务端保留 `status=cancelled` 的记录，`get-event` 仍可查到但已非活动日程。

### 转让组织者

```bash
feishu-cli calendar event-transfer [calendar_id] <event_id> --to-user-id ou_xxx \
  [--remove-original-organizer] [--transfer-series] [--dry-run] --yes
```

- **不可撤销**（会议纪要、附件一并转给新组织者），需 `--yes`；先 `--dry-run` 并向用户确认接收人。
- 重复日程会转让整个序列（接口不支持只转让某一次），必须加 `--transfer-series` 表示知情。
- 当前身份（`--as`）必须是日程的组织者；日程在共享日历上时须传该日历 ID（否则 193110）。
- 默认原组织者保留为参与人；`--remove-original-organizer` 转让后将其移除（共享日历上服务端强制移除）。

## 搜索日程

```bash
feishu-cli calendar event-search \
  [--calendar-id <id>] [--query "关键词"] \
  [--start "2026-01-01"] [--end "2026-12-31T23:59:59+08:00"] \
  [--attendee-ids ou_xxx,oc_xxx,omm_xxx] [--page-size 20] [--page-token <token>] [-o json]
```

`--calendar-id` 默认 `primary`；`--query` 可空。`--start`/`--end` 接受 RFC3339 或 `YYYY-MM-DD`，只给一边时补同一天边界，
start>end 发网前失败。`--page-size` 1-30（默认 20，越界报错）。`-o json` 输出 `{events, next_page_token, has_more}`。

## 回复日程邀请

```bash
feishu-cli calendar event-reply <calendar_id> <event_id> --status accept|decline|tentative
```

与 `calendar rsvp` 等价，**必需 User Token**（行为变更：此前版本默认 Bot 身份，必然答复失败）。`calendar_id` 可写 `primary`。

## 参与人管理

```bash
# 添加（立即通知被添加者；无 --dry-run）
feishu-cli calendar attendee add <calendar_id> <event_id> \
  [--user-ids ou_xxx,ou_yyy] [--chat-ids oc_xxx] [--room-ids omm_xxx] \
  [--attendee-ids ou_xxx,oc_xxx,omm_xxx,user@example.com]

# 移除（POST attendees/batch_delete）
feishu-cli calendar attendee remove <calendar_id> <event_id> \
  [--user-ids ou_xxx] [--chat-ids oc_xxx] [--room-ids omm_xxx] \
  [--attendee-ids <ou_/oc_/omm_/邮箱/attendee_id>] [--notify=false] [--dry-run]

# 列出
feishu-cli calendar attendee list <calendar_id> <event_id> \
  [--type user|chat|resource|third_party] [--page-all] [-o json]
```

- 至少指定一类 ID。`--user-ids` / `--chat-ids` / `--room-ids` 分别只接受 `ou_` / `oc_` / `omm_` 前缀；会议室先用 `calendar room-find` 找空闲。
- `attendee remove --attendee-ids` 中无法按前缀识别的值视为 `attendee list` 返回的 `attendee_id`。
- `attendee list --type` 可逗号分隔多选，`resource` 为会议室；`--page-all` 上限 50 页。
- 群参与人（`type=chat`）不输出 `rsvp_status`：服务端对群条目恒返回 `needs_action`，没有意义。
- 被移除的参与人仍在列表中，`rsvp_status=removed`。

## 忙闲查询

```bash
feishu-cli calendar freebusy \
  [--start "2026-01-01T09:00:00+08:00"] [--end "2026-01-01T18:00:00+08:00"] \
  [--user-id ou_xxx[,ou_yyy]] \
  [--type busy|raw_busy|free|common_free] [--min-duration 30m] [-o json]
```

- `--start` 默认今天 00:00，`--end` 默认起始日当天结束；也接受 `YYYY-MM-DD`（按当天 23:59:59 结束）与 Unix 秒。
- `--user-id` 只接受 `ou_`，不传时默认**当前登录用户**；未登录（Bot）没有"本人"，必须指定（否则本地报用法错误）。
- `busy`（默认）：按时间排序并合并重叠/相邻区间（区间数 ≠ 日程数）；`raw_busy` 保留原始条目与 `rsvp_status`；
  `free` 每人空闲；`common_free` 多人共同空闲；`--min-duration` 只作用于 `free` / `common_free`。
- JSON：单人 + `busy` 输出数组 `[{start_time,end_time}]`；多人或 `raw_busy` / `free` 输出
  `{"users":[{"user_id":...,"busy"|"raw_busy"|"free":[...]}]}`；`common_free` 输出 `{"user_ids":[...],"common_free":[{start_time,end_time,duration}]}`。

## Token 策略

- **读类**（`calendar list/primary/get/get-event/list-events/freebusy/suggestion/room-find`、`calendar attendee list`）：
  无 `--as`，登录后默认 User Token（自动从 `~/.feishu-cli/token.json` 加载），未登录回落 App Token。
- **身份可选 `--as bot|user|auto`**（`calendar agenda` / `calendar event-search`）：默认 auto（User 优先，未配置回落 Bot；
  已配置 User 但刷新失败 fail-closed，避免 `primary` 查到错误主体的日历）。
- **写类 `--as bot|user|auto`，默认 auto**（`calendar create-event/update-event/delete-event/event-share/event-transfer`、
  `calendar attendee add/remove`）：已登录用 User Token（操作本人日程），未配置 User Token 时回落 App Token，已配置但刷新失败 fail-closed。
  **行为变更**：此前版本这些写命令默认 App Token（Bot）——从 agenda 拿到本人日程 ID 再用 Bot 改删大概率失败。
  需要操作应用（Bot）日历时显式传 `--as bot`。
- **必需 User Token**：`calendar rsvp`、`calendar event-reply`（以本人身份答复邀请）。
