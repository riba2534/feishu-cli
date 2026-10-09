# 飞书实时事件订阅技能（WebSocket）

通过 `feishu-cli event` 子命令族订阅飞书开放平台事件，使用 WebSocket 长连接接收事件并以 NDJSON 输出到 stdout，适合 AI Agent 做 bot 实时响应、群消息监听、审批回调消费等场景。

> **发消息？** 走 [`msg` 工作流](../msg/workflow.md)。本工作流专注于事件订阅（**接收**应用事件），不负责发送。

## 目录

1. [核心概念](#核心概念)
2. [命令速查](#命令速查)
3. [EventKey 速查（按 domain 分组）](#eventkey-速查按-domain-分组)
4. [权限与开放平台配置](#权限与开放平台配置)
5. [AI Agent 后台订阅推荐用法](#ai-agent-后台订阅推荐用法)
6. [踩坑与注意事项](#踩坑与注意事项)
7. [何时转其他 skill](#何时转其他-skill)
8. [参考](#参考)

## 核心概念

### 进程模型 = 同一 App 一个 consume 进程、一条连接、可订阅多个 EventKey

```
event consume <EventKey> [EventKey...]
   │
   ├─ 获取 App 级单实例锁（同一 App 本机只允许一个 consume 进程；--force 可绕过，不安全）
   ├─ 查询飞书侧已有长连接数（>0 时 stderr 告警：事件会被其他连接分走）
   ├─ 启动 1 条 WebSocket 长连接（飞书 SDK ws.Client + AutoReconnect），注册全部订阅的事件类型
   ├─ 注册到 bus.json（每个 EventKey 一条：PID / EventKey / 启动时间 / max-events / timeout）
   ├─ stderr 输出 [event] ready event_key=<k1,k2>（pre-consume + WS 握手都完成后才发）
   ├─ 接收事件 → 按 event_id 去重 → 写 stdout（NDJSON，每条一行 JSON，用 .header.event_type 区分）
   ├─ 可选：dump 每条事件为 <event_id>.json 文件
   ├─ 退出条件：--max-events / --timeout / SIGTERM / Ctrl-C / stdin EOF（仅无界运行） / pipe broken
   └─ 退出时自动 unregister bus.json
```

**为什么只能一个进程**：飞书长连接按 App 维度把事件**随机**投递给该 App 的任一条连接，不看连接"想要"哪些类型。
同一 App 开多个 consume（哪怕订阅不同 EventKey）= 多条连接互相抢事件，每个进程都只能收到一部分。
因此需要多个 EventKey 时**在一个进程里一起订阅**：`event consume k1 k2`（或 `k1,k2`）。第二个 consume 会直接报错退出，
错误里列出正在运行的 PID 与 EventKey。

**未订阅的事件类型**：启动前远端预检确认本进程是该 App 唯一的长连接时，收到未订阅的已知事件类型会 ACK 后本地丢弃（stderr 每种类型提示一次），
避免服务端反复重投；存在其他连接（其他机器/服务）时不代答，让服务端可以重投给真正处理它的连接。卡片回调从不代答。

**架构取舍**：不跑官方那种独立 bus 守护进程做事件 fan-out；feishu-cli 简化为「单进程单连接多 EventKey」。

### 状态文件与跨进程互斥

| 路径 | 作用 |
|---|---|
| 默认：`~/.feishu-cli/events/<app_id>/bus.json`；启用 profile：`~/.feishu-cli/profiles/<active>/events/<app_id>/bus.json` | 活跃 consumer 列表（PID/EventKey/启动时间/参数） |
| 同目录 `bus.lock` | flock 文件锁；bus.json 读写串行化，fd 关闭自动释放 |
| 同目录 `consume.lock` | App 级单实例锁（非阻塞 flock / Windows 独占打开），进程退出由系统自动释放；文件内容为持有者 PID |

**每个 AppID 一个子目录**，不同应用互不干扰。`event status` 查询会主动剔除已不存活的 PID 条目（kill -9 / 崩溃残留）。bus.json 用 tmp + rename 原子写，防半写。

### 输出协议（NDJSON + ready marker）

| 流 | 内容 |
|---|---|
| **stdout** | 每条事件一行 JSON（NDJSON），适合 jq / 脚本管道 |
| **stderr** | 诊断日志；pre-consume 与 WebSocket 握手都完成后一行 `[event] ready event_key=<key>`（多个 key 时 `event_key=k1,k2`，顺序同命令行） |

> **AI Agent 推荐**：父进程把 consume 跑后台（`run_in_background=true`），先阻塞 stderr 等到 `[event] ready` 那一行再开始读 stdout。ready 发出前握手未完成，不要靠额外 sleep 猜。VC EventKey 还需 User Token 做服务端订阅；同 key 每个 consume 都幂等 POST subscribe，确认订阅成功后才发 ready，最后一个人退出才注销。

### 退出码与退出 reason

| 退出码 | 含义 |
|---|---|
| 0 | 正常退出（达到 `--max-events` / `--timeout` / SIGTERM / Ctrl-C / stdin EOF / 下游管道关闭） |
| 1 | 同一 App 已有 consume 进程（单实例锁冲突），或其他启动 / 运行错误 |
| 2 | 参数错误（未知 EventKey、非法 `--jq` / `--output-dir`） |
| 3 / 4 | 按全局分类：鉴权或凭证问题（如审批/VC 缺 User Token）/ 网络错误 |

stderr 末尾会输出 `[event] exited — elapsed=<d> reason=<r>`，reason 有 4 个：

- `limit` — 达到 `--max-events`
- `timeout` — 达到 `--timeout`
- `signal` — 上下文取消（Ctrl-C / SIGTERM / stdin EOF / 下游 pipe broken）
- `error` — 启动失败或 WebSocket 连接失败

## 命令速查

```bash
feishu-cli event list [--json]                       # 1. 列所有支持的 EventKey
feishu-cli event schema <event_key> [--json]         # 2. 看某 key 的 EventType / scope / payload schema
feishu-cli event consume <event_key> [event_key...] [flags]  # 3. 启动订阅（阻塞，同一进程一条连接）
feishu-cli event status [--json]                     # 4. 看本机活跃 consume 进程
feishu-cli event stop {--pid N | --event-key K | --all} [--force] [--json]  # 5. 停 consume
```

### 1. `event list`：列出支持的 EventKey

按 domain 分组展示当前支持的全部 EventKey；完整清单以 `feishu-cli event list` 输出为准。

```bash
# 表格视图（默认）
feishu-cli event list

# JSON 输出，jq 提取 IM 域所有 EventKey
feishu-cli event list --json | jq -r '.[] | select(.domain=="im") | .key'
```

**输出字段**（JSON 模式）：`key` / `event_type` / `description` / `domain` / `scopes[]` / `auth_types[]`（`bot` 或 `user`，
`user` 表示需要 User Token 做服务端订阅）/ `required_console_events[]`（需在开放平台勾选的事件）/ `payload_schema`。

### 2. `event schema`：看 payload schema 与 scope

```bash
feishu-cli event schema im.message.receive_v1
feishu-cli event schema im.message.receive_v1 --json
```

输出 `Key` / `Event Type` / `Domain` / `Description` / `Scopes` / `Auth Types` / `Console`（需在开放平台勾选的事件），
部分 key 附 `Payload Schema (示例)`。Payload schema 为手工整理的示例，实际 payload 以飞书开放平台文档为准。

### 3. `event consume`：启动 WebSocket 订阅（阻塞）

```bash
# 基础订阅，Ctrl-C 退出
feishu-cli event consume im.message.receive_v1

# 调试：抓 5 条消息，最多跑 60s
feishu-cli event consume im.message.receive_v1 --max-events 5 --timeout 60s

# 落盘 + 静默
feishu-cli event consume im.message.receive_v1 --output-dir ./events --quiet

# 配合 jq 实时过滤群消息
feishu-cli event consume im.message.receive_v1 | jq 'select(.event.message.chat_type=="group")'

# 后台订阅多个 EventKey：一个进程一起订阅（共用一条连接），按 header.event_type 分流
feishu-cli event consume im.message.receive_v1 im.message.reaction.created_v1 > events.ndjson 2> events.log &
feishu-cli event status
jq -c 'select(.header.event_type=="im.message.receive_v1")' events.ndjson > receive.ndjson

# ❌ 不要为每个 EventKey 各起一个进程：同 App 多连接会随机拆分事件，第二个进程也会被单实例锁拒绝
```

**关键 flag**：

| Flag | 默认 | 说明 |
|---|---|---|
| `--max-events N` | 0（不限制） | 接收 N 条事件后退出，reason=`limit`；有界运行忽略 stdin EOF |
| `--timeout <duration>`（示例 `60s`） | 0（不限制） | 运行 D 时长后退出，reason=`timeout`；有界运行忽略 stdin EOF |
| `--force` | false | 跳过同一 App 单实例检查（**不安全**：多进程随机拆分事件） |
| `--jq .event.xxx` | "" | 极简**点路径**过滤，不支持完整 jq 语法（用 pipe 接外部 jq） |
| `--output-dir ./events` | "" | 每条事件额外 dump 为 `<event_id>.json` 落盘（不影响 stdout） |
| `--quiet` | false | 抑制 stderr 诊断；**AI Agent 慎用**——会一起抑制大部分 stderr，但 ready marker 仍走真实 os.Stderr 不受影响 |

**`--jq` 限制**：只识别 `.a.b.c` 形式的 map 取值（如 `.event.message`），不支持 `select` / 数组下标 / 管道。复杂过滤请用 `feishu-cli event consume ... | jq '<expr>'`。

**`--output-dir` 限制**：必须是安全相对路径；不做 `~` 展开，不接受绝对路径或 `..` 路径段。

### 4. `event status`：看本机活跃 consume 进程

```bash
feishu-cli event status
feishu-cli event status --json | jq '(.consumers // [])[] | .pid'   # 无活跃进程时 consumers 为 null
```

输出：`App ID` / `State file` 路径 / `PID` / `EVENT_KEY` / `UPTIME` / `EXTRA`（max-events / timeout / output-dir / jq）。

查询时会主动剔除已不存活的 PID 条目（清理 kill -9 / 崩溃残留的僵尸记录）。

### 5. `event stop`：停止 consume 进程

```bash
feishu-cli event stop --pid 12345                          # 按 PID
feishu-cli event stop --event-key im.message.receive_v1    # 按 EventKey（订阅了该 key 的进程整体退出，含它订阅的其他 key）
feishu-cli event stop --all                                # 当前 AppID 下全部 consume
feishu-cli event stop --all --force                        # SIGKILL（紧急情况）
```

默认 SIGTERM 优雅退出（consume 进程会自动 unregister bus.json），等最多 3s 验证进程已退出；`--force` 升级为 SIGKILL，会留下 bus.json 僵尸条目，下次 `event status` 会自动清理。

## EventKey 速查（按 domain 分组）

完整列表用 `feishu-cli event list`。常用：

| Domain | EventKey | 描述 |
|---|---|---|
| im | `im.message.receive_v1` | 接收消息（用户/群聊发给 Bot） |
| im | `im.message.message_read_v1` | 消息已读回执 |
| im | `im.message.recalled_v1` | 消息被撤回 |
| im | `im.message.reaction.created_v1` / `deleted_v1` | 消息表情回复添加/删除 |
| im | `im.chat.updated_v1` | 群聊信息更新 |
| im | `im.chat.member.user.added_v1` / `deleted_v1` | 用户进群/离群 |
| im | `im.chat.member.bot.added_v1` / `deleted_v1` | Bot 被拉入/移出群 |
| im | `im.chat.disbanded_v1` | 群聊被解散 |
| im | `card.action.trigger` ⭐ | **卡片交互回调**（按钮点击/表单提交/下拉选择），交互式 Bot 核心事件 |
| application | `application.bot.menu_v6` | 用户点击 Bot 自定义菜单 |
| contact | `contact.user.created_v3` / `updated_v3` / `deleted_v3` | 员工入职/变更/离职 |
| calendar | `calendar.calendar.event.changed_v4` | 日程变更（创建/更新/删除） |
| calendar | `calendar.calendar.acl.created_v4` | 日历权限变更 |
| drive | `drive.file.title_updated_v1` | 文档标题修改 |
| drive | `drive.file.permission_member_added_v1` | 文档协作者添加 |
| approval | `approval.instance.status_changed_v4` | 审批实例状态变更（需服务端订阅注册，见下） |
| approval | `approval.task.status_changed_v4` | 审批任务状态变更（需服务端订阅注册，见下） |
| vc | `vc.meeting.participant_meeting_started_v1` / `joined_v1` / `ended_v1` | 当前用户参与的会议开始/加入/结束（User pre-consume） |
| vc | `vc.note.generated_v1` | 智能纪要已生成 |
| vc | `vc.recording.recording_started_v1` / `recording_transcript_generated_v1` / `recording_ended_v1` | 录制开始/逐字稿/结束 |

> **EventKey 与 EventType 通常一致**；接收到的 payload 里 `header.event_type` 等于 `event_type` 字段。

### 卡片交互回调 `card.action.trigger`（交互式 Bot 闭环）

卡片按钮/表单回调走独立的回调帧通道，consume 已自动处理，用法与普通事件一致：

```bash
feishu-cli event consume card.action.trigger   # 每次卡片交互输出一行 NDJSON
```

payload 里的关键字段：`event.operator.open_id`（谁点的）、`event.action.value`（按钮自定义值）、
`event.action.form_value`（表单数据）、`event.token`（卡片更新凭证，可拿去调卡片更新 OpenAPI 回写卡片）、
`event.context.open_message_id / open_chat_id`（消息与会话定位）。
开放平台需在「事件与回调 - 回调订阅」勾选 `card.action.trigger` 并发布版本。

### 审批事件的服务端订阅注册（v4，自动完成）

审批 v4 事件**除了后台勾选事件，还必须以 User 身份注册服务端订阅关系**，否则连上 WS 也收不到。
consume 启动时自动完成注册（对 INVOLVED_APPROVAL 与 MANAGED_APPROVAL 各注册一次）：

```bash
feishu-cli auth login   # 需 approval:instance:read / approval:task:read scope
feishu-cli event consume approval.instance.status_changed_v4
# stderr: [event] 已注册服务端订阅: ... subscription_type=INVOLVED_APPROVAL / MANAGED_APPROVAL
```

订阅是**持久的用户级关系**，进程退出不注销；重复注册服务端幂等处理。未登录时报错并提示 auth login。

### VC 事件的服务端订阅（User pre-consume，last-consumer 注销）

`vc.meeting.participant_meeting_*` / `vc.note.generated_v1` / `vc.recording.*` 必须用 User Token 在 consume 启动前 POST 对应 `.../subscription`（body `{"event_type": "<key>"}`）。同一 EventKey 多个 consume 并存时（仅 `--force` 绕过单实例锁时可能出现），**每个 consumer 都幂等 POST subscribe**，自己的订阅成功后才发 ready（避免 first 的 subscribe 阻塞/失败时 second 跳过订阅并提前 ready）。最后一个人退出才 POST `.../unsubscription`（5s timeout），避免先退出者打断后者。

**注销后复检补订阅**：unsubscription 是文件锁之外的网络调用，注销在途期间可能有新 consumer 完成注册并订阅。因此注销后会复检存活 consumer 数，若 > 0 则幂等重新订阅并在 stderr 提示「注销后检测到 N 个新 consumer，正在恢复服务端订阅」——否则新 consumer 虽已 ready 却会静默收不到任何事件。看到该提示属正常自愈，无需干预。

## 权限与开放平台配置

### 默认 App Token，无需 `auth login`

WebSocket 连接本身走 App 身份（app_id + app_secret）。普通事件不强制 User Token；
审批 v4 和 VC 事件还需用户身份完成前置订阅注册，详见上文相应章节。配好 `~/.feishu-cli/config.yaml` 或 `FEISHU_APP_ID` / `FEISHU_APP_SECRET` 环境变量即可。

### 飞书开放平台配置

在 [open.feishu.cn](https://open.feishu.cn) 你的应用控制台：

1. **「事件订阅 - 长连接接收事件」** 开启长连接模式（feishu-cli 走 WebSocket，**不是** webhook URL 模式）
2. **「事件与回调 - 事件订阅」** 选中目标 EventType（与 `event schema <key>` 输出的 Event Type 一致）并**发布版本**
3. **scope 开通**：每个 EventKey 需要的 scope 见 `event schema <key>` 的 `Scopes` 字段；在「权限管理」页面开通。`event` 域已加入 `--domain event --recommend` 推荐列表，可一次性申请 IM/contact/calendar/drive/approval/vc 常用 scope 并集

### 常见错误

| 现象 | 原因 | 解决 |
|---|---|---|
| WS 连接失败，stderr 报 ws error | 长连接模式未开启 | 飞书开放平台开启「事件订阅 - 长连接接收事件」 |
| 启动后看到 ready，但收不到事件 | 目标 EventType 未在「事件订阅」勾选 / 未发版本 | 重新勾选 + 发版 |
| 收到事件但 payload 字段缺失 | App 缺对应 scope（如 `im:message.p2p_msg:readonly`） | `event schema <key>` 看 Scopes，去权限管理页开通后重新订阅 |
| `event consume` 立即退出 reason=error | App ID/Secret 错 / 网络不通 / 域名走 lark 但 BaseURL 用了 feishu | 检查 App 凭证与网络；Lark 国际版需在 `config.yaml` / profile 中把 `base_url` 设为 `https://open.larksuite.com` |
| 报"本机已有 event consume 进程在运行" | 同一 App 只允许一个 consume 进程 | 把 EventKey 合并到一个进程：`event consume k1 k2`；或 `event stop --all` 后重启 |
| stderr 警告"飞书侧已有 N 条事件长连接" | 其他机器/服务用同一 App 连着长连接 | 事件会被随机分走；停掉其他连接，或换独立 App / profile |
| 偶发收不到事件、事件"丢了" | 同 App 有其他长连接在抢事件 | 看启动时的远端连接告警；保证同 App 只有一个消费端 |

## AI Agent 后台订阅推荐用法

### 后台订阅（`run_in_background=true`）

```python
# 1. 后台启动 consume，stderr/stdout 各 redirect
task = Bash(
    command='feishu-cli event consume im.message.receive_v1 --output-dir ./events 2> consume.log',
    run_in_background=True,
)

# 2. tail consume.log 阻塞等 "[event] ready event_key=im.message.receive_v1"

# 3. 业务逻辑：tail stdout / 读 ./events/*.json 处理新事件

# 4. 需要多个 EventKey？在同一个命令里一起列出（一条连接），不要再起第二个 consume：
#    feishu-cli event consume im.message.receive_v1 card.action.trigger ...
# 5. 退出：feishu-cli event stop --event-key im.message.receive_v1
#         或父进程 kill 后台 Bash task（SIGTERM 触发 graceful shutdown + unregister）
```

### 子进程 stdin EOF 协议（非 TTY）

非 TTY 且**未设 `--max-events` / `--timeout`** 时，**关闭 stdin 即触发优雅退出**（reason=signal）。Python `subprocess.Popen` 用 `stdin=subprocess.PIPE`，处理完后 `p.stdin.close()` 比 SIGTERM 更稳——consume 会跑完当前事件再退出。

设置了 `--max-events` 或 `--timeout` 的**有界运行会忽略 stdin EOF**：`true | feishu-cli event consume ... --timeout 30s`、
后台任务 stdin 为 `/dev/null` 等场景会跑到上限才退出，不会再 2 秒内被 EOF 提前结束。

### 限制单跑时长 / 事件数

调试场景永远先用 `--max-events N --timeout Ds`，避免忘了 stop 留下后台进程吃 API quota：

```bash
feishu-cli event consume im.message.receive_v1 --max-events 1 --timeout 30s
# 抓 1 条事件 demo / 30 秒超时双保险
```

## 踩坑与注意事项

- **daemon 进程持久**：`event consume` 阻塞运行直到信号/超时/EOF；**不会自己退出**。AI Agent 后台跑必须配 `--max-events` / `--timeout` 或显式 `event stop`，否则会留下长跑进程
- **同一 App 只跑一个 consume**：多进程 = 多条长连接 = 事件被随机拆分。多个 EventKey 放在同一个命令里；`--force` 只用于明确接受拆分的调试场景
- **事件去重**：同一 event_id 5 分钟内的重复投递只输出一次（stderr 会提示跳过），不计入 `--max-events`
- **flock 跨进程互斥**：bus.json 读写都走 flock；但**不要手动编辑** bus.json
- **pipe broken 自动退出**：下游 jq / tee 关闭 stdout（典型场景：`event consume ... | head -1`）会触发 SIGPIPE，consume 主动 cancel 退出 reason=signal，不会卡死等 Ctrl-C
- **`--quiet` 不影响 ready marker**：ready marker 走真实 `os.Stderr` 绕过 `--quiet` 重定向，所以 AI Agent 即使开 `--quiet` 父进程仍能等到 ready 行；但其他诊断（包括 `[event] exited` reason）会被静默
- **断线自动重连、无限重试**：断线后约每 2 分钟重试一次（首次带抖动），进程不会因断线自行退出。长时间断线场景建议用 `--timeout` 主动退出，由外层守护进程拉起，比内层无限重试更可控
- **PID 复用风险**：`event status` 仅用 `signal(0)` 探活，没有核对进程启动时间或可执行文件。
  若旧 PID 已被系统复用，status 可能把无关进程误判为 consumer，`event stop --pid N` 也可能向无关进程发信号。
  stop 前先检查 `bus.json` 的启动时间和系统进程信息；状态明显陈旧时不要直接 `--force`。
- **每条事件独立文件**：`--output-dir` 模式下每条事件落盘 `<event_id>.json`，**短时间高频事件可能创建大量小文件**；落盘只为留痕，业务消费仍推荐用 stdout NDJSON
- **`--jq` 只支持点路径**：`--jq .event.message` 把每条事件投影到子树后再输出；不命中的事件**会被 skip**（不输出空行）。复杂过滤永远走 pipe 外部 jq
- **`--output-dir` 只支持安全相对路径**：传 `~/events`、`/tmp/events`、`../events` 都会报错；用 `./events` 或 `events/today`

## 何时转其他 skill

| 任务 | 路由 |
|---|---|
| **发**消息 / 回复 / 卡片 / 通知 | [`msg` 工作流](../msg/workflow.md) |
| 构造 interactive 卡片 JSON | [`card` 工作流](../card/workflow.md) |
| 处理收到的消息事件 → 写多维表格 | **feishu-cli-data**（解析 payload 后调 record 命令） |
| 处理收到的审批事件 → 查审批详情 | **feishu-cli-work**（approval 子命令） |
| 收到群消息后查群信息/成员 | [`chat` 工作流](../chat/workflow.md) |
| Webhook URL 模式（HTTP 回调，非长连接） | 不在本技能范围；走飞书开放平台的「请求网址配置」+ 自建 HTTP server |
| 历史消息批量拉取（非实时） | [`chat` 工作流](../chat/workflow.md) 的 `msg history` / `msg list` |

## 参考

- 飞书开放平台事件订阅文档：https://open.feishu.cn/document/server-docs/event-subscription-guide/event-list
- `--output-dir` 落盘文件名由 `header.event_id` 净化而来：只保留 `[A-Za-z0-9_-]`、截断到 128 字符，净化后为空则跳过落盘，
  恶意构造的 `event_id`（如 `../etc/passwd`）不会写出目录之外。
