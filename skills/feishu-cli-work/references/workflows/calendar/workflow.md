# 飞书日历工作流

覆盖日程查询（agenda / event-search / get-event）、日程增删改与重复日程、参与人与会议室、忙闲与智能排会
（freebusy / suggestion / room-find）、答复邀请（rsvp / event-reply）、分享与转让组织者。
各命令的完整参数与输出字段见 `references/basic-commands.md`，以 `feishu-cli calendar <cmd> --help` 为最终依据。

## 目录

- [决策规则](#决策规则)
- [身份选择](#身份选择)
- [查询日程](#查询日程)
- [智能排会：suggestion / room-find](#智能排会suggestion--room-find)
- [答复邀请：rsvp / event-reply](#答复邀请rsvp--event-reply)
- [典型工作流](#典型工作流)
- [重复日程（RRULE）操作指引](#重复日程rrule操作指引)
- [坑点](#坑点)
- [权限速查](#权限速查)
- [何时转其他技能](#何时转其他技能)

## 决策规则

| 用户意图 | 用什么 | 说明 |
|---|---|---|
| 看某天/某段时间的安排 | `calendar agenda` | 展开重复日程为实例并过滤已取消日程；`list-events` 返回原始日程（含多年前的重复日程主体和 `status=cancelled` 记录），不适合按天看安排 |
| 按关键词/参与人/会议室找日程 | `calendar event-search` | 返回实例 ID |
| 查单个日程详情 | `calendar get-event` | 不含参与人列表，参与人用 `calendar attendee list` |
| 没有明确时间的约会/排会 | 先 `suggestion` 推荐时段 | 再对选定时段 `room-find`；不要在没有明确时段时直接 `room-find` |
| 只看忙闲或多人共同空闲 | `calendar freebusy --type busy\|free\|common_free` | 需要"推荐"时段时用 `suggestion`（会综合工作时间等因素） |
| 创建并邀请 | `create-event --attendee-ids`（先 `--dry-run`） | 添加参与人会立即发邀请；参与人添加失败自动删除刚建的日程（回滚） |
| 已有日程补人/会议室 | `attendee add` | 立即通知被添加者，无 `--dry-run`，执行前先向用户确认名单 |
| 分享日程给他人/群/文档 | `get-event --share-link` 或 `event-share` | 不要用 `app_link`（带查看者本人的 calendarId，只能本人打开） |
| 转让组织者 | `event-transfer`（先 `--dry-run`） | 不可撤销，需 `--yes`；重复日程还需 `--transfer-series` |
| 修改/删除重复日程 | `update-event` / `delete-event --apply-to` | 范围必须由用户确认，见[重复日程](#重复日程rrule操作指引) |

## 身份选择

| 命令 | 身份 |
|------|------|
| `list` / `primary` / `get` / `get-event` / `list-events` / `attendee list` / `freebusy` / `suggestion` / `room-find` | 无 `--as`：User Token 优先，未登录回落 Bot（Bot 查公开忙闲与公司可订会议室） |
| `agenda` / `event-search` | `--as bot\|user\|auto`，默认 auto；`primary` 跟随当前身份，已配置 User 但刷新失败时报错（不静默切 Bot） |
| `create-event` / `update-event` / `delete-event` / `attendee add\|remove` / `event-share` / `event-transfer` | `--as bot\|user\|auto`，**默认 auto**（行为变更：此前默认 Bot）。已登录即以本人身份操作本人日程；操作应用（Bot）日历显式 `--as bot` |
| `rsvp` / `event-reply` | **必需 User Token**（答复是本人动作），未登录直接报错 |

- `freebusy` 不传 `--user-id` 时查当前登录用户；未登录（Bot）没有"本人"，必须显式 `--user-id`。
- 带 `[calendar_id] <event_id>` 的命令（get-event / update-event / delete-event / event-share / event-transfer）只传
  event_id 时使用 `primary`（当前身份的主日历）；必须传 calendar_id 的位置参数也可以直接写 `primary`。
- 创建日程时组织者不会自动出现在参与人列表：实测 User 身份不带 `--attendee-ids` 创建后 `attendee list` 为空；
  带 `--attendee-ids` 时 CLI 会把本人一并加入。Bot 需要出现在参与人列表（例如后续以 Bot 身份收会议事件）时，先取 Bot open_id 再加入：

```bash
BOT_ID=$(feishu-cli api GET /open-apis/bot/v3/info --as bot --jq '.bot.open_id' | tr -d '"')
feishu-cli calendar attendee add <calendar_id> <event_id> --user-ids "$BOT_ID"
```

## 查询日程

```bash
# 今天 / 明天（--end-date 为包含端；只查明天时两个都传明天）
feishu-cli calendar agenda
feishu-cli calendar agenda --start-date 2026-03-28 --end-date 2026-03-28 -o json

# 关键词 + 时间 + 参与人
feishu-cli calendar event-search --query "周会" --start 2026-04-20 --end 2026-04-27 --attendee-ids ou_xxx,omm_xxx -o json
```

- `agenda`：缺省 `--end-date` 与起始日同一天；起始日晚于结束日在发网前报错。超过 40 天的区间自动切分；
  `--page-size` / `--page-token` 被忽略（服务端无分页）。JSON 字段 `events[].{event_id,summary,start_time,end_time,status,self_rsvp_status,free_busy_status}`。
- 全天日程输出 `is_all_day=true`，起止为 `YYYY-MM-DD`，结束日已换算为**包含端**（服务端 `end.date` 是排他的次日）。
- `event-search`：`--query` 可空（纯过滤）；`--start/--end` 接受 RFC3339 或 `YYYY-MM-DD`，只给一边时补同一天边界；
  `--page-size` 1-30（越界报错）；`--attendee-ids` 按前缀识别 `ou_` / `oc_` / `omm_`，官方说明同类型多个 ID 为"或"关系。
  JSON 输出 `{events, next_page_token, has_more}`，以 `has_more` 判断是否还有下一页。

## 智能排会：suggestion / room-find

```bash
# 推荐共同空闲时段（--start/--end 只收 RFC3339）
feishu-cli calendar suggestion \
  --attendee-ids ou_aaa,ou_bbb,oc_xxx \
  --duration 30m \
  --start 2026-01-22T09:00:00+08:00 --end 2026-01-22T18:00:00+08:00 \
  --exclude 2026-01-22T12:00:00+08:00~2026-01-22T13:00:00+08:00 \
  -o json

# 在明确时段找会议室（可重复 --slot，或逗号分隔多段）
feishu-cli calendar room-find \
  --slot 2026-01-22T09:30:00+08:00~2026-01-22T10:00:00+08:00 \
  --floor F2 --min-capacity 6 --max-capacity 12 \
  -o json
```

- `suggestion` 必填 `--attendee-ids` 与 `--duration`。`--duration` 支持 `30m` / `1h30m` / 纯数字分钟（`90`），范围 1-1440；
  不足 1 分钟（如 `0.5m`）报错。`--start` 默认当前时间，`--end` 默认 `start` 当天 23:59:59（跨天必须显式传 `--end`）。
- `suggestion` JSON：`suggestions[].{event_start_time,event_end_time,recommend_reason}`，可能附带 `ai_action_guidance`。
- `room-find` 必填 `--slot`；JSON：`time_slots[].{start,end,meeting_rooms[].{room_id,room_name,capacity,reserve_until_time}}`。
  多时段并发查询，任一时段失败即返回错误；已内置 429 退避重试，不要在外层再叠加重试。
  `suggestion` 没有内置重试，脚本里循环调用时自行间隔 1-2 秒。
- 两者的 `--attendee-ids` 只接受 `ou_`（用户）/ `oc_`（群），其他前缀（`omm_` 会议室、`on_` union_id、纯 user_id、邮箱）
  会在 stderr 告警并**跳过**；重复 ID 自动去重。需要先用 `feishu-cli user search --email ...` 换成 `ou_`（见 feishu-cli-platform 的 directory 工作流）。
- 时段写法 `start~end`（`--slot` 与 `--exclude` 都用 `~`，不是 `--`）；`end` 必须严格晚于 `start`，两端用同一时区。

## 答复邀请：rsvp / event-reply

```bash
feishu-cli calendar rsvp --event-id <event_id> --action accept|decline|tentative [--calendar-id <cal_id>]
feishu-cli calendar event-reply <calendar_id> <event_id> --status accept|decline|tentative
```

- 两者等价，都必需 User Token。`rsvp` 省略 `--calendar-id` 时自动查主日历（多一次请求，批量时可先
  `calendar primary -o json` 缓存）；`--rsvp-status` 是 `--action` 的兼容别名，同时传不同值报错。
- 取值只有 `accept` / `decline` / `tentative`，其他值在本地报错（`accpet` / `declined` / `maybe` 是常见拼错）。
- `rsvp` 只有文本输出。

## 典型工作流

### 工作流 A：排会议（端到端）

```bash
# 1. 找共同空闲
feishu-cli calendar suggestion --attendee-ids ou_alice,ou_bob,ou_carol --duration 30m \
  --start 2026-01-22T09:00:00+08:00 --end 2026-01-22T12:00:00+08:00 -o json | jq '.suggestions[0]'

# 2. 在选定时段找会议室
feishu-cli calendar room-find --slot 2026-01-22T09:30:00+08:00~2026-01-22T10:00:00+08:00 \
  --floor F2 --min-capacity 6 --max-capacity 12 -o json | jq '.time_slots[0].meeting_rooms[0]'

# 3. 先预览，再创建日程并邀请参与人与会议室（omm_ 为会议室；添加失败自动删除日程回滚）
feishu-cli calendar create-event --summary "三方对齐" \
  --start 2026-01-22T09:30:00+08:00 --end 2026-01-22T10:00:00+08:00 \
  --attendee-ids ou_alice,ou_bob,ou_carol,omm_xxx --vchat --dry-run
# 确认后去掉 --dry-run 执行
```

### 工作流 B：批量答复邀请

```bash
# agenda JSON 用 self_rsvp_status 过滤待答复
feishu-cli calendar agenda --start-date 2026-01-22 --end-date 2026-01-23 -o json \
  | jq -r '.events[] | select(.self_rsvp_status=="needs_action") | .event_id' \
  | while read -r eid; do
      feishu-cli calendar rsvp --event-id "$eid" --action accept
    done
```

## 重复日程（RRULE）操作指引

`create-event` / `update-event` 用 `--rrule` 传 RFC5545 RRULE；`create-event`、`update-event`、`get-event` 的输出带
`recurrence`（文本为「重复规则」）。

```bash
# 每周一 10:00-11:00 的周会（--start/--end 决定首个实例）
feishu-cli calendar create-event --summary "周会" \
  --start 2026-07-27T10:00:00+08:00 --end 2026-07-27T11:00:00+08:00 \
  --rrule "FREQ=WEEKLY;BYDAY=MO"
```

### 先分清四种日程（event_id 形如 `{uid}_{原始时间戳}`）

| 类型 | event_id | 判别 |
|------|----------|------|
| 普通日程 | `{uid}_0` | 无 `recurrence` |
| 重复日程主体 | `{uid}_0` | 有 `recurrence` |
| 实例 | `{uid}_{ts>0}` | `is_exception=false`（`agenda` / `event-search` 返回的就是实例 ID） |
| 例外 | `{uid}_{ts>0}` | `is_exception=true`（单独改过的那一次） |

`update-event` / `delete-event` 会先读取日程判型；不传 `--apply-to` 或重复日程时在 stderr 说明影响范围。

### 不传 `--apply-to`：服务端原生语义（兼容旧行为）

| 传入 ID | delete-event | update-event |
|---------|--------------|--------------|
| 主体 `{uid}_0` | 删整条序列，但**不级联**已单独修改过的例外（例外仍留在日历上） | 改整条序列，已存在的例外不受影响 |
| 实例/例外 | 只删这一次 | 只改这一次（实例被物化为例外） |

### `--apply-to` 显式范围

| 值 | 适用 | 语义 |
|----|------|------|
| `single` | 实例 / 例外 / 普通日程 | 只操作这一次；传主体 ID 报用法错误（exit 2） |
| `all` | 主体 / 实例 / 例外 | 整条序列含全部例外：delete 先删例外再删主体；update 改了时间先删例外再改主体，未改时间把本次字段同步到每个例外再改主体 |
| `this-and-following` | 仅实例（例外/主体报错） | 删除该次及之后的例外，主体 RRULE 截断到前一天（`UNTIL`）；update 还会以该次时间新建一条继承原设置的新序列，原规则带 `COUNT` 时换算为 `UNTIL`，总次数不变 |

`all` / `this-and-following` 需要确认：非交互环境不带 `--yes` 以 exit 10 退出且不执行。批量清理例外时不通知参与人，
主体的删除/截断按 `--notify`（默认 true）通知。**用户没说清范围时先问，不要替用户选。**

```bash
# 只删某一次（agenda 拿到的实例 ID）
feishu-cli calendar delete-event <instance_event_id> --apply-to single

# 删除整条重复序列（含例外）
feishu-cli calendar delete-event <任意实例或主体 ID> --apply-to all --yes

# 从某次起不再重复
feishu-cli calendar delete-event <instance_event_id> --apply-to this-and-following --yes

# 整条序列改名（例外一并改）
feishu-cli calendar update-event <instance_event_id> --summary "新周会" --apply-to all --yes

# 从某次起改时间（截断 + 新序列）
feishu-cli calendar update-event <instance_event_id> --apply-to this-and-following \
  --start 2026-08-03T15:00:00+08:00 --end 2026-08-03T16:00:00+08:00 --yes

# 把日程改成每个工作日重复（主体 ID 作用于序列，例外不受影响）
feishu-cli calendar update-event <calendar_id> <master_event_id> --rrule "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"
```

`--dry-run` 可预览请求（不联网）。删除后服务端保留 `status=cancelled` 的记录：`get-event` 仍能查到，`agenda` 不再返回
（instance_view 有数秒缓存延迟）。

### 常用 RRULE 速查

| 需求 | RRULE |
|------|-------|
| 每天 | `FREQ=DAILY` |
| 每天，共 10 次 | `FREQ=DAILY;COUNT=10` |
| 每周一 | `FREQ=WEEKLY;BYDAY=MO` |
| 每个工作日 | `FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR` |
| 每两周一次 | `FREQ=WEEKLY;INTERVAL=2` |
| 每月 1 号 | `FREQ=MONTHLY;BYMONTHDAY=1` |
| 每月 1 号，到指定日期结束 | `FREQ=MONTHLY;BYMONTHDAY=1;UNTIL=20261231T000000Z` |

`COUNT` 与 `UNTIL` 不能同时出现；预定会议室的重复日程长度不得超过两年。

## 坑点

1. **不能用 CLI 建全天日程**：`create-event` / `update-event` 的 `--start/--end` 传 `YYYY-MM-DD` 会被当作当天 00:00 的具体时段
   （实测 `--start 2026-10-25 --end 2026-10-26` 生成一个 24 小时的时段日程）。时间点优先带时区的 RFC3339；
   也接受 `2026-01-21 14:00`（本地时区）与 Unix 秒/毫秒。
2. **`update-event` 的 `--start` 与 `--end` 必须成对**：只改一端服务端返回成功但不生效，CLI 在本地报用法错误（exit 2）。
3. **`suggestion` / `room-find` / `--exclude` 只收 RFC3339**：传 `YYYY-MM-DD` 直接报错；`freebusy`、`event-search`
   的 `--start/--end` 才接受纯日期。
4. **添加参与人即通知**：`create-event --attendee-ids` 与 `attendee add` 都会立即发邀请；`attendee remove` 可用 `--notify=false`。
   被移除的参与人仍会出现在 `attendee list` 中，`rsvp_status=removed`。
5. **分享链接**：`get-event --share-link` / `event-share` 返回 `share_link`；`get-event -o json` 仍带 `app_link` 字段，
   但它只能本人打开，不要拿来分享或自行拼接。
6. **`calendar list --page-size` 取值 50–1000**（服务端最小 50），默认 50；越界在本地报用法错误（退出码 2）。

## 权限速查

| 命令 | scope（任一即可，以报错提示为准） |
|------|------|
| `agenda` / `event-search` / `get-event` / `event-share` | `calendar:calendar:readonly`、`calendar:calendar`、`calendar:calendar.event:read` |
| `create-event` | `calendar:calendar`、`calendar:calendar.event:create` |
| `update-event` / `attendee add` | `calendar:calendar`、`calendar:calendar.event:update` |
| `delete-event` | `calendar:calendar`、`calendar:calendar.event:writeonly`、`calendar:calendar.event:delete` |
| `attendee remove` | `calendar:calendar`、`calendar:calendar.event:writeonly`、`calendar:calendar.event:update` |
| `freebusy` | `calendar:calendar:readonly`、`calendar:calendar`、`calendar:calendar.free_busy:read` |
| `suggestion` / `room-find` | `calendar:calendar.free_busy:read` |
| `rsvp` / `event-reply` | `calendar:calendar`、`calendar:calendar.event:writeonly`、`calendar:calendar.event:reply` |
| `event-transfer` | `calendar:calendar.event:transfer` |

完整 scope 可用 `feishu-cli schema calendar.<resource>.<method> --format json` 查看。

预检（User 身份）：

```bash
feishu-cli auth check --scope "calendar:calendar.free_busy:read calendar:calendar.event:reply"
```

Bot 身份缺 scope 报 99991672，需在开放平台为应用开通对应 tenant scope；User 身份报 99991679 时增量 `auth login --scope`。

## 何时转其他技能

| 需求 | 转到 |
|------|------|
| 给参会人发会议提醒消息 | `feishu-cli-messaging`（msg + card 工作流） |
| 邮箱 / 手机号 / 姓名换 `ou_` open_id | `feishu-cli-platform` 的 directory 工作流（`user search`） |
| 查历史会议、会议纪要、录制 | `feishu-cli-meetings` |
