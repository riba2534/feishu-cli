# 飞书智能日历技能

通过 feishu-cli 智能安排会议：自动找共同空闲、按容量/楼层筛会议室、接受/拒绝邀请。AI Agent 排会议主用本技能。

本工作流覆盖基础日历 CRUD、agenda、参与人、忙闲查询，以及智能时段、会议室和 RSVP。

`calendar agenda --end-date` 是包含端：结束时刻为**该日次日当地午夜减 1 秒**（不要用 `Add(24h)`，DST 日会错）。
缺省 `--end-date` 与起始日同一天。只查“明天”时 start-date 和 end-date 都传明天日期；
起始日晚于结束日会在发网前报错。

`calendar agenda` 走 `GET /open-apis/calendar/v4/calendars/{id}/events/instance_view`：单次窗口上限 40 天，超过会客户端对半切分；命中 193104（单窗口超过 1000 个实例）同样切分后去重。instance_view **没有服务端分页**，`--page-size` / `--page-token` 会被忽略。全天日程的结束日按飞书排他日期转为含当日（例如 API 的 `end.date=2025-03-22` 输出为 `2025-03-21`）。

`calendar event-search` 走 `POST /open-apis/calendar/v4/calendars/{id}/events/search_event`，时间过滤写入 `filter.time_range.{start_time,end_time}`（RFC3339），不要再用旧 `/events/search` 的顶层 `start_time`/`end_time`。`--attendee-ids` 按前缀拆到 `attendee_user_ids`（`ou_`）/ `attendee_chat_ids`（`oc_`）/ `meeting_room_ids`（`omm_`）。`--page-size` 范围 1-30。

## 目录

- [核心概念](#核心概念)
- [子命令速查](#子命令速查)
- [典型工作流](#典型工作流)
- [重复日程（RRULE）操作指引](#重复日程rrule操作指引)
- [关键 flag 速记](#关键-flag-速记)
- [踩坑（必读）](#踩坑必读)
- [何时转其他技能](#何时转其他技能)
- [权限速查](#权限速查)

## 核心概念

### 三件套定位

| 子命令 | 解决什么 | 何时用 |
|--------|---------|--------|
| `suggestion` | 给一组参与者推荐共同空闲时段 | 排会议第一步：定时间 |
| `room-find` | 给定时段找可用会议室 | 排会议第二步：定地点 |
| `rsvp` | 接受/拒绝/待定 已收到的邀请 | 被邀方处理邀请 |

典型组合：先 `suggestion` 拿到推荐时段 → `room-find` 在该时段筛会议室 → `calendar create-event --attendee-ids ou_...,omm_...` 创建日程并邀请参与人和会议室（参与人添加失败自动回滚删除日程）。

### 底层实现 & 重试

- 直调 OpenAPI `/open-apis/calendar/v4/freebusy/suggestion` 与 `.../freebusy/room_find`，SDK v3.5.3 未暴露这两个方法。
- `room-find` 内置 `DoWithRetry`（`MaxRetries=3 / MaxTotalAttempts=8 / RetryOnRateLimit=true`）——429 限流不计失败次数，full-jitter 退避，上限 30s。
- `room-find` 多时段批量是并发调用（默认 10 worker），429 由每个 goroutine 各自重试，无需用户层退避。
- `suggestion` 当前**未挂 DoWithRetry**（单次调用），如果手工脚本里高频跑请自行 sleep 1-2s。

### 身份选择

| 命令 | Token 行为 |
|------|-----------|
| `suggestion` / `room-find` | User Token 优先 + App Token 兜底：已 `auth login` 时用 User Token（查私人忙闲），未登录回落 App Token（查公开忙闲、公司可订会议室）。`--user-access-token` 可显式指定。 |
| `agenda` / `event-search` | `--as bot\|user\|auto`（默认 auto）。`primary` 跟当前身份；已配置 User 但刷新失败 fail-closed，禁止静默切 Bot。 |
| `create-event` / `update-event` / `delete-event` / `attendee add\|remove` / `event-share` / `event-transfer` | `--as bot\|user\|auto`，**默认 auto**（行为变更：此前默认 Bot）。已登录即以本人身份操作本人日程；操作应用日历时显式 `--as bot`。 |
| `freebusy` | User Token 优先 + App Token 兜底；不传 `--user-id` 时默认当前登录用户，Bot 身份必须显式指定。 |
| `rsvp` / `event-reply` | **必需 User Token**（以本人身份答复邀请），未登录直接报错。 |

权限：`calendar:calendar.free_busy:read`（suggestion / room-find）、`calendar:calendar.event:reply`（rsvp）。

`calendar rsvp` 支持 `--action`，也支持兼容别名 `--rsvp-status`，二者等价且不能同时指定不同值。

## 子命令速查

### 1. calendar suggestion（找共同空闲）

```bash
feishu-cli calendar suggestion \
  --attendee-ids ou_aaa,ou_bbb,oc_groupid \
  --duration 30m \
  [--start 2024-01-22T09:00:00+08:00] \
  [--end   2024-01-22T18:00:00+08:00] \
  [--timezone Asia/Shanghai] \
  [--event-rrule "FREQ=WEEKLY;COUNT=4"] \
  [--exclude 2024-01-22T12:00:00+08:00~2024-01-22T13:00:00+08:00] \
  [-o json]
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--attendee-ids`（必填） | 参与者 ID 列表，逗号分隔，`ou_xxx`（用户）+ `oc_xxx`（群聊）混合 | — |
| `--duration`（必填） | 会议时长，`30m` / `1h30m` / `90`（纯数字按分钟），范围 1-1440 | — |
| `--start` | 搜索起点（RFC3339 / `YYYY-MM-DD` / Unix 秒或毫秒，13 位自动识别为毫秒） | 当前时间 |
| `--end` | 搜索终点（同 `--start` 的格式） | `start` 当天 23:59:59 |
| `--timezone` | 时区，如 `Asia/Shanghai` | — |
| `--event-rrule` | 周期性规则 rrule 字符串（找系列会议共同空闲） | — |
| `--exclude` | 排除时段，多段逗号分隔，单段 `start~end` RFC3339 | — |
| `-o` | 输出格式：`json` 给 AI 解析 / 空给人看 | 空 |

**返回**：推荐时段列表 + `ai_action_guidance`（服务端给的人话建议）。

#### 输出示例（文本）

```
推荐时段（共 3 个）:

[1] 2024-01-22T09:00:00+08:00 ~ 2024-01-22T09:30:00+08:00
    理由: 全员有空
[2] 2024-01-22T10:00:00+08:00 ~ 2024-01-22T10:30:00+08:00
    理由: 全员有空
[3] 2024-01-22T14:00:00+08:00 ~ 2024-01-22T14:30:00+08:00
    理由: 全员有空

建议: 推荐选择上午 09:00，所有人精力较好。
```

### 2. calendar room-find（找会议室）

```bash
feishu-cli calendar room-find \
  --slot 2024-01-22T09:00:00+08:00~2024-01-22T10:00:00+08:00 \
  [--slot 2024-01-22T14:00:00+08:00~2024-01-22T15:00:00+08:00] \
  [--attendee-ids ou_aaa,ou_bbb] \
  [--city "北京" --building "飞书大厦" --floor F2] \
  [--room-name "01,02,03"] \
  [--min-capacity 6 --max-capacity 20] \
  [--timezone Asia/Shanghai] \
  [-o json]
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--slot`（必填） | 待查时段 `start~end`（RFC3339）；可重复传入或逗号分隔多段 | — |
| `--attendee-ids` | 参与者 ID（`ou_xxx`/`oc_xxx`），用于推荐离参与人近的会议室 | — |
| `--city` | 城市约束 | — |
| `--building` | 建筑约束 | — |
| `--floor` | 楼层约束（如 `F2`） | — |
| `--room-name` | 会议室名称约束，逗号分隔多个 | — |
| `--min-capacity` / `--max-capacity` | 容量范围（≥0，min ≤ max） | 0（不限） |
| `--timezone` | 时区 | — |
| `--event-rrule` | 周期性规则 rrule | — |

**返回**：按时段聚合的可用会议室列表，含 `room_id`/`room_name`/`capacity`/`reserve_until_time`。多 slot 时并发查询（10 worker），任一时段失败立即返回首个错误。

#### 输出示例（文本）

```
2024-01-22T09:00:00+08:00 ~ 2024-01-22T10:00:00+08:00
  [1] 飞书大厦-F2-01 (id=omm_xxx, capacity=8)
      可预订至: 2024-01-22T10:00:00+08:00
  [2] 飞书大厦-F2-02 (id=omm_yyy, capacity=12)

2024-01-22T14:00:00+08:00 ~ 2024-01-22T15:00:00+08:00
  （无可用会议室）
```

### 3. calendar rsvp（答复邀请）

```bash
feishu-cli calendar rsvp \
  --event-id <EVENT_ID> \
  --action accept | decline | tentative \
  [--calendar-id <CAL_ID>] \
  [--user-access-token <TOKEN>]
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--event-id`（必填） | 日程 ID | — |
| `--action`（必填） | `accept` / `decline` / `tentative` | — |
| `--calendar-id` | 日历 ID | 主日历（自动调 `calendar primary`） |
| `--user-access-token` | User Token，以本人身份答复 | 必需（`requireUserToken`），未登录直接报错 |

**与 `calendar event-reply` 的区别**：

| 维度 | `rsvp`（新） | `event-reply`（旧） |
|------|-------------|--------------------|
| 参数风格 | 全 flag（`--event-id`/`--action`） | 位置参数 `<calendar_id> <event_id>` + `--status` |
| calendar-id | 可省略（默认主日历） | 必填 |
| 适用 | AI Agent 调度 | 人类直接敲命令 |

## 典型工作流

### 工作流 A：AI Agent 排会议（端到端）

```bash
# 1. 找共同空闲（30 分钟，明早 9-12 点）
feishu-cli calendar suggestion \
  --attendee-ids ou_alice,ou_bob,ou_carol \
  --duration 30m \
  --start 2024-01-22T09:00:00+08:00 \
  --end   2024-01-22T12:00:00+08:00 \
  -o json | jq '.suggestions[0]'
# 假设拿到 09:30-10:00

# 2. 在该时段找会议室（6-12 人，F2 楼）
feishu-cli calendar room-find \
  --slot 2024-01-22T09:30:00+08:00~2024-01-22T10:00:00+08:00 \
  --floor F2 --min-capacity 6 --max-capacity 12 \
  -o json | jq '.time_slots[0].meeting_rooms[0]'
# 假设拿到 room_id=omm_xxx

# 3. 创建日程并邀请参与人与会议室（omm_ 前缀为会议室；添加失败会自动删除日程回滚）
feishu-cli calendar create-event \
  --summary "三方对齐" \
  --start 2024-01-22T09:30:00+08:00 \
  --end   2024-01-22T10:00:00+08:00 \
  --attendee-ids ou_alice,ou_bob,ou_carol,omm_xxx --vchat
# 已有日程补人/补会议室：attendee add <cal_id> <event_id> --user-ids ... --room-ids omm_xxx
```

### 工作流 B：批量答复邀请

```bash
# 列出待答复的邀请（agenda JSON 使用 self_rsvp_status）
feishu-cli calendar agenda --start-date 2024-01-22 --end-date 2024-01-23 -o json \
  | jq -r '.events[] | select(.self_rsvp_status=="needs_action") | "\(.event_id)"' \
  | while read eid; do
      feishu-cli calendar rsvp --event-id "$eid" --action accept --user-access-token <TOKEN>
    done
```

## 重复日程（RRULE）操作指引

`create-event` / `update-event` 支持 `--rrule`，传 RFC5545 RRULE 字符串把日程变成重复日程。
`recurrence` 字段就是这个 RRULE 串，`create-event` / `update-event` / `get-event` 的 `-o json`
输出和文本输出都会带「重复规则」。

### 创建重复日程

```bash
# 每周一 10:00-11:00 的周会（--start/--end 决定首个实例）
feishu-cli calendar create-event \
  --calendar-id <id> \
  --summary "周会" \
  --start "2026-07-27T10:00:00+08:00" \
  --end   "2026-07-27T11:00:00+08:00" \
  --rrule "FREQ=WEEKLY;BYDAY=MO"
```

### 先分清四种日程（event_id 形如 `{uid}_{原始时间戳}`）

| 类型 | event_id | 判别 |
|------|----------|------|
| 普通日程 | `{uid}_0` | 无 `recurrence` |
| 重复日程主体 | `{uid}_0` | 有 `recurrence` |
| 实例 | `{uid}_{ts>0}` | `is_exception=false`（`agenda` / `event-search` 返回的就是实例 ID） |
| 例外 | `{uid}_{ts>0}` | `is_exception=true`（单独改过/删过的那一次） |

`update-event` / `delete-event` 会先 `GET` 日程判型，并在 stderr 说明影响范围（`-o json` 有 `kind` / `scope`）。

### 不传 `--apply-to`：服务端原生语义（实测，兼容旧行为）

| 传入 ID | delete-event | update-event |
|---------|--------------|--------------|
| 主体 `{uid}_0` | 删整条序列，但**不级联**已单独修改过的例外（例外仍留在日历上） | 改整条序列，已存在的例外不受影响 |
| 实例/例外 | 只删这一次 | 只改这一次（实例被物化为例外） |

### `--apply-to` 显式范围（对齐官方 lark-calendar-recurring）

| 值 | 适用 | 语义 |
|----|------|------|
| `single` | 实例 / 例外 / 普通日程 | 只操作这一次；主体 ID 传 single 报错（应传实例 ID） |
| `all` | 主体 / 实例 / 例外 | 整条序列含全部例外：delete 先销毁例外再删主体；update 改了时间先删例外再改主体，未改时间把本次字段同步到每个例外再改主体 |
| `this-and-following` | 仅实例（例外/主体报错） | 从该次起：删除该次及之后的例外，主体 RRULE 截断到前一天（UNTIL）；update 还会以该次时间新建一条继承原设置（标题/描述/地点/提醒/视频会议/参与人）的新序列，原规则带 `COUNT` 时换算成原序列最后一次的 `UNTIL`，总次数不变 |

`all` / `this-and-following` 需要确认（非交互环境加 `--yes`，否则 exit 10 且不执行）；批量清理例外时不通知参与人。
用户没说清范围时先问，不要替用户选。

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

# 把日程改成每个工作日重复（主体 ID，作用于序列；例外不受影响）
feishu-cli calendar update-event <calendar_id> <master_event_id> \
  --rrule "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"
```

先 `--dry-run` 预览请求（不联网）。删除后服务端保留 `status=cancelled` 的记录：`get-event` 仍能查到，但
`agenda` / `list-events` 不再作为有效实例出现（instance_view 有数秒缓存延迟）。

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

注意：`COUNT` 与 `UNTIL` 不能同时出现；预定会议室的重复日程长度不得超过两年。

### 身份提示

`create-event` / `update-event` / `delete-event` 等写命令支持 `--as`，**默认 auto**：已 `auth login` 时以本人身份
操作本人日历（**行为变更**：此前默认 Bot），未配置 User Token 时回落 Bot。要在应用日历上操作显式传 `--as bot`；
若 App 未开通 tenant 级 `calendar:calendar.event:*`，Bot 身份会报 99991672。

Bot 身份建的日程 **Bot 自己不在参会人列表里**（用户身份建则自动入会）。如需 Bot 出现在
参会人列表（例如后续要以 Bot 身份收会议事件），先取 Bot 自身 open_id 再随 attendee add 加入：

```bash
BOT_ID=$(feishu-cli api GET /open-apis/bot/v3/info --as bot --jq '.bot.open_id' | tr -d '"')
feishu-cli calendar attendee add <cal_id> <event_id> --user-ids "$BOT_ID,ou_其他人"
```

另注：搜索用户接口（`user read --query`）不支持 Bot 身份，需 User Token。

## 关键 flag 速记

| 场景 | 关键 flag | 备注 |
|------|----------|------|
| 时段格式 | `start~end`（RFC3339） | `--slot` 和 `--exclude` 都用 `~` 分隔，**不是** `--` |
| 多时段 | `--slot a~b --slot c~d` 或 `--slot a~b,c~d` | StringSlice 两种语法都支持 |
| 时长两种写法 | `--duration 30m` 或 `--duration 30` | 纯数字 = 分钟；范围 1-1440 |
| AI 解析 | `-o json` | suggestion / room-find 都支持；rsvp 仅有文本输出 |
| 不知道 calendar-id | rsvp 省略 `--calendar-id` | 自动走主日历 |

## 踩坑（必读）

### 1. ID 前缀必须是 `ou_` / `oc_`

`--attendee-ids` 内部走 `SplitAttendeeIDs(raw)` 按前缀切：

- `ou_xxx` → `attendee_user_ids`
- `oc_xxx` → `attendee_chat_ids`
- 其他前缀（`omm_`/`room_`/`app_` 会议室或资源 token）→ stderr 打 warn 然后**跳过**，不阻塞批量
- 重复 ID 会自动去重

**所以**：从飞书拷贝 `user_id`（纯字符串无前缀）/`union_id`（`on_xxx`）/`email` 直接喂会被全部跳过，要先通过 `feishu-cli user get` 或 `contact:user.base:readonly` 接口换成 `ou_xxx`。

### 2. 429 多发是常态，依赖 DoWithRetry

`freebusy/room_find` 在批量并发场景 429 命中率较高（实测 10 worker 跑 6 个 slot 经常触发 2-3 次）。CLI 已内置 `RetryOnRateLimit=true`——**用户层不要再叠加 retry**，会撞 `MaxTotalAttempts=8` 上限提前失败。如果跑大批量需要更激进的并发，自己调 `roomFindWorkers` 常量重编一版。

`suggestion` 暂未挂重试，循环调用前自己 `sleep 1`。

### 3. duration 单位坑

- `--duration 30` = 30 **分钟**（不是秒、不是小时）
- `--duration 30m` = 30 分钟
- `--duration 1h30m` = 90 分钟
- `--duration 1.5h` = 合法（time.ParseDuration 接受小数，= 90 分钟）；过小值如 `0.5m` 会被截断为 0 分钟，需避免
- 范围 1-1440，超过 24 小时直接报错

### 4. start/end 默认值容易踩

- `--start` 默认 = 当前时间（不是当天 00:00）
- `--end` 默认 = `start` 当天 23:59:59（注意是同一天，跨天必须显式传 `--end`）
- 想找"明天全天" → 必须两个都传

### 5. rsvp 没 calendar-id 时会多一次 API

省略 `--calendar-id` 会先调 `calendar primary` 拿主日历再调 reply，多一次 RTT。批量答复脚本里建议先 `feishu-cli calendar primary -o json` 缓存 ID 再传给每次 `rsvp`。

### 6. rsvp action 仅三个枚举

`accept` / `decline` / `tentative`，其它字符串直接 400。错别字常见：`accpet` / `declined` / `maybe`。

### 7. exclude / slot 时间方向

`start~end` 中 `end` 必须**严格晚于** `start`，相等也会报错。跨日要带正确时区，不要直接传 `T00:00:00Z` 后跟 `+08:00` 混用，时区不一致解析后比较会乱。

## 何时转其他技能

| 需求 | 转到 |
|------|------|
| 创建/修改/删除日程、agenda、event-search | 本工作流的 `references/basic-commands.md` |
| 朴素 freebusy 查询单人/单时段 | 本工作流的 `references/basic-commands.md` |
| 加/删 attendee（含会议室）、查 attendee 列表 | 本工作流的 `references/basic-commands.md` |
| 日程分享链接、转让组织者 | 本工作流的 `references/basic-commands.md`（`event-share` / `event-transfer`） |
| event-reply 老接口（位置参数版，同样必需 User Token） | 本工作流的 `references/basic-commands.md` |
| 给参会人发会议提醒消息 | `feishu-cli-messaging`（msg + card 工作流） |
| 拿 `ou_xxx` open_id（email/user_id → open_id 转换） | `feishu-cli-platform` 的 directory 工作流 |

## 权限速查

| 命令 | scope | Token 推荐 |
|------|------|-----------|
| `suggestion` | `calendar:calendar.free_busy:read` | User Token 优先 + App Token 兜底 |
| `room-find` | `calendar:calendar.free_busy:read` | User Token 优先 + App Token 兜底 |
| `rsvp` | `calendar:calendar.event:reply` | **必需 User Token**（以本人身份答复） |

预检：

```bash
feishu-cli auth check --scope "calendar:calendar.free_busy:read calendar:calendar.event:reply"
```
