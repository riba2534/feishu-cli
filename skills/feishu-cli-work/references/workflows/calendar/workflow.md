# 飞书智能日历技能

通过 feishu-cli 智能安排会议：自动找共同空闲、按容量/楼层筛会议室、接受/拒绝邀请。AI Agent 排会议主用本技能。

本工作流覆盖基础日历 CRUD、agenda、参与人、忙闲查询，以及智能时段、会议室和 RSVP。

`calendar agenda --end-date` 按当天 `23:59:59` 处理，是包含端。只查“明天”时 start-date 和
end-date 都传明天日期；传“明天到后天”会同时包含后天。

## 核心概念

### 三件套定位

| 子命令 | 解决什么 | 何时用 |
|--------|---------|--------|
| `suggestion` | 给一组参与者推荐共同空闲时段 | 排会议第一步：定时间 |
| `room-find` | 给定时段找可用会议室 | 排会议第二步：定地点 |
| `rsvp` | 接受/拒绝/待定 已收到的邀请 | 被邀方处理邀请 |

典型组合：先 `suggestion` 拿到推荐时段 → `room-find` 在该时段筛会议室 → `calendar create-event` 创建日程并邀请参与人和会议室。

### 底层实现 & 重试

- 直调 OpenAPI `/open-apis/calendar/v4/freebusy/suggestion` 与 `.../freebusy/room_find`，SDK v3.5.3 未暴露这两个方法。
- `room-find` 内置 `DoWithRetry`（`MaxRetries=3 / MaxTotalAttempts=8 / RetryOnRateLimit=true`）——429 限流不计失败次数，full-jitter 退避，上限 30s。
- `room-find` 多时段批量是并发调用（默认 10 worker），429 由每个 goroutine 各自重试，无需用户层退避。
- `suggestion` 当前**未挂 DoWithRetry**（单次调用），如果手工脚本里高频跑请自行 sleep 1-2s。

### 身份选择

| 命令 | Token 行为 |
|------|-----------|
| `suggestion` / `room-find` | User Token 优先 + App Token 兜底：已 `auth login` 时用 User Token（查私人忙闲），未登录回落 App Token（查公开忙闲、公司可订会议室）。`--user-access-token` 可显式指定。 |
| `rsvp` | **必需 User Token**（以本人身份答复邀请），未登录直接报错。 |

权限：`calendar:calendar.free_busy:read`（suggestion / room-find）、`calendar:calendar.event:reply`（rsvp）。

`calendar rsvp` 支持 `--action`，也支持官方 lark-cli 兼容别名 `--rsvp-status`，二者等价且不能同时指定不同值。

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
| `--start` | 搜索起点（RFC3339） | 当前时间 |
| `--end` | 搜索终点（RFC3339） | `start` 当天 23:59:59 |
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

# 3. 创建日程并邀请参与人 + 会议室（走 toolkit）
feishu-cli calendar create-event \
  --calendar-id <主日历> \
  --summary "三方对齐" \
  --start 2024-01-22T09:30:00+08:00 \
  --end   2024-01-22T10:00:00+08:00
# 后续 attendee add 把人和 room 加进去
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
- `--duration 1.5h` = **报错**（time.ParseDuration 不接受小数小时，要写 `1h30m`）
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
| 加/删 attendee、查 attendee 列表 | 本工作流的 `references/basic-commands.md` |
| event-reply 老接口（位置参数版） | 本工作流的 `references/basic-commands.md` |
| 给参会人发会议提醒消息 | `feishu-cli-messaging` + `feishu-cli-messaging` |
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
