---
name: feishu-cli-event
description: >-
  飞书实时事件订阅（WebSocket）。event list 看支持的 EventKey；event schema 看事件 payload/scope；
  event consume 启动长连接订阅，事件流以 NDJSON 写到 stdout（阻塞，一个进程订阅一个 EventKey）；
  event status 看本机活跃 consume 进程；event stop 按 PID / EventKey / --all 停止 consume。
  支持 22+ EventKey（im 消息接收/已读/撤回/reaction、群成员变动、contact 员工变更、
  日历变更、云盘标题/协作者、审批实例与任务、VC 会议起止）。
  状态文件 ~/.feishu-cli/events/<app_id>/bus.json + flock 文件锁 + WebSocket auto-reconnect。
  当用户请求"监听飞书事件"、"实时接收消息事件"、"订阅审批回调"、"event 流"、
  "WebSocket 长连接监听"、"event consume"、"event list / schema / status / stop"、
  "AI Agent bot 实时响应"时使用。
  注意：本技能只负责订阅；处理事件 webhook 业务逻辑（push 到飞书消息/写多维表格）
  请配合 feishu-cli-msg / feishu-cli-bitable。
argument-hint: list | schema <key> | consume <key> | status | stop
user-invocable: true
allowed-tools: Bash, Read, Write
---

# 飞书实时事件订阅技能（WebSocket）

通过 `feishu-cli event` 子命令族订阅飞书开放平台事件，使用 WebSocket 长连接接收事件并以 NDJSON 输出到 stdout，适合 AI Agent 做 bot 实时响应、群消息监听、审批回调消费等场景。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。
>
> **发消息？** 请使用 **feishu-cli-msg** 技能。本技能专注于事件订阅（**接收**应用事件），不负责发送。

## 核心概念

### 进程模型 = 1 个 EventKey 1 个 consume 进程

```
event consume <EventKey>
   │
   ├─ 启动 WebSocket 长连接（飞书 SDK ws.Client + AutoReconnect）
   ├─ 注册到 bus.json（PID / EventKey / 启动时间 / max-events / timeout）
   ├─ stderr 输出 [event] ready event_key=<key>
   ├─ 接收事件 → 写 stdout（NDJSON，每条一行 JSON）
   ├─ 可选：dump 每条事件为 <event_id>.json 文件
   ├─ 退出条件：--max-events / --timeout / SIGTERM / Ctrl-C / stdin EOF / pipe broken
   └─ 退出时自动 unregister bus.json
```

**与 lark-cli event 的差异**：lark-cli 用 Unix domain socket 跑独立 bus 守护进程做事件 fan-out；feishu-cli 简化为「每个 consume 直接连一条 WebSocket」，不做事件分发——足够覆盖 AI Agent 单 EventKey 订阅的主线场景。

### 状态文件与跨进程互斥

| 路径 | 作用 |
|---|---|
| `~/.feishu-cli/events/<app_id>/bus.json` | 活跃 consumer 列表（PID/EventKey/启动时间/参数） |
| `~/.feishu-cli/events/<app_id>/bus.lock` | flock 文件锁；bus.json 读写串行化，fd 关闭自动释放 |

**每个 AppID 一个子目录**，不同应用互不干扰。`event status` 查询会主动剔除已不存活的 PID 条目（kill -9 / 崩溃残留）。bus.json 用 tmp + rename 原子写，防半写。

### 输出协议（NDJSON + ready marker）

| 流 | 内容 |
|---|---|
| **stdout** | 每条事件一行 JSON（NDJSON），适合 jq / 脚本管道 |
| **stderr** | 诊断日志；启动时一行 `[event] ready event_key=<key> (init complete; WS handshake in progress)` |

> **AI Agent 推荐**：父进程把 consume 跑后台（`run_in_background=true`），先阻塞 stderr 等到 `[event] ready` 那一行再开始读 stdout。**注意**：ready marker 只表示进程初始化完成，WS 握手在后台异步执行；父进程见到 marker 后**还需额外等 1-3s 让 WS 握手真正完成**，生产环境建议父进程发自检事件 + 等 echo 回环来确认链路通。

### 退出码与退出 reason

| 退出码 | 含义 |
|---|---|
| 0 | 正常退出（达到 `--max-events` / `--timeout` / SIGTERM / Ctrl-C / stdin EOF） |
| 非 0 | startup 失败 / WebSocket 不可恢复错误 / 参数错误 |

stderr 末尾会输出 `[event] exited — elapsed=<d> reason=<r>`，reason 有 4 个：

- `limit` — 达到 `--max-events`
- `timeout` — 达到 `--timeout`
- `signal` — 上下文取消（Ctrl-C / SIGTERM / stdin EOF / 下游 pipe broken）
- `error` — WebSocket 连接持续失败

## 命令速查

```bash
feishu-cli event list [--json]                       # 1. 列所有支持的 EventKey
feishu-cli event schema <event_key> [--json]         # 2. 看某 key 的 EventType / scope / payload schema
feishu-cli event consume <event_key> [flags]         # 3. 启动订阅（阻塞）
feishu-cli event status [--json]                     # 4. 看本机活跃 consume 进程
feishu-cli event stop {--pid N | --event-key K | --all} [--force] [--json]  # 5. 停 consume
```

### 1. `event list`：列出支持的 EventKey

按 domain 分组展示当前支持的 22+ EventKey（im / contact / calendar / drive / approval / vc）。

```bash
# 表格视图（默认）
feishu-cli event list

# JSON 输出，jq 提取 IM 域所有 EventKey
feishu-cli event list --json | jq -r '.[] | select(.domain=="im") | .key'
```

**输出字段**（JSON 模式）：`key` / `event_type` / `description` / `domain` / `scopes[]` / `payload_schema`。

### 2. `event schema`：看 payload schema 与 scope

```bash
feishu-cli event schema im.message.receive_v1
feishu-cli event schema im.message.receive_v1 --json
```

输出 4 部分：`Key` / `Event Type` / `Domain` / `Description` / `Scopes` + 可选 `Payload Schema (示例)`。Payload schema 为手工 curated，订阅后实际 payload 以飞书开放平台文档为准。

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

# 后台并发订阅多个 EventKey（每个 EventKey 一个进程）
feishu-cli event consume im.message.receive_v1     > receive.ndjson  2> receive.log  &
feishu-cli event consume im.message.reaction.created_v1 > reaction.ndjson 2> reaction.log &
feishu-cli event status
```

**关键 flag**：

| Flag | 默认 | 说明 |
|---|---|---|
| `--max-events N` | 0（不限制） | 接收 N 条事件后退出，reason=`limit` |
| `--timeout 30s` | 0（不限制） | 运行 D 时长后退出，reason=`timeout` |
| `--jq .event.xxx` | "" | 极简**点路径**过滤，不支持完整 jq 语法（用 pipe 接外部 jq） |
| `--output-dir ./events` | "" | 每条事件额外 dump 为 `<event_id>.json` 落盘（不影响 stdout） |
| `--quiet` | false | 抑制 stderr 诊断；**AI Agent 慎用**——会一起抑制大部分 stderr，但 ready marker 仍走真实 os.Stderr 不受影响 |

**`--jq` 限制**：只识别 `.a.b.c` 形式的 map 取值（如 `.event.message`），不支持 `select` / 数组下标 / 管道。复杂过滤请用 `feishu-cli event consume ... | jq '<expr>'`。

**`--output-dir` 限制**：必须是相对路径或已存在的绝对路径，**不做 `~` 展开**。

### 4. `event status`：看本机活跃 consume 进程

```bash
feishu-cli event status
feishu-cli event status --json | jq '.consumers[] | .pid'
```

输出：`App ID` / `State file` 路径 / `PID` / `EVENT_KEY` / `UPTIME` / `EXTRA`（max-events / timeout / output-dir / jq）。

查询时会主动剔除已不存活的 PID 条目（清理 kill -9 / 崩溃残留的僵尸记录）。

### 5. `event stop`：停止 consume 进程

```bash
feishu-cli event stop --pid 12345                          # 按 PID
feishu-cli event stop --event-key im.message.receive_v1    # 按 EventKey（所有订阅该 key 的进程）
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
| contact | `contact.user.created_v3` / `updated_v3` / `deleted_v3` | 员工入职/变更/离职 |
| calendar | `calendar.calendar.event.changed_v4` | 日程变更（创建/更新/删除） |
| calendar | `calendar.calendar.acl.created_v4` | 日历权限变更 |
| drive | `drive.file.title_updated_v1` | 文档标题修改 |
| drive | `drive.file.permission_member_added_v1` | 文档协作者添加 |
| approval | `approval_instance` | 审批实例状态变更 |
| approval | `approval_task` | 审批任务变更 |
| vc | `vc.meeting.meeting_started_v1` / `meeting_ended_v1` | VC 会议开始/结束 |

> **EventKey 与 EventType 通常一致**；接收到的 payload 里 `header.event_type` 等于 `event_type` 字段。

## 权限与开放平台配置

### 默认 App Token，无需 `auth login`

事件订阅走 App 身份（app_id + app_secret），**不强制 user token**。配好 `~/.feishu-cli/config.yaml` 或 `FEISHU_APP_ID` / `FEISHU_APP_SECRET` 环境变量即可。

### 飞书开放平台两步配置

在 [open.feishu.cn](https://open.feishu.cn) 你的应用控制台：

1. **「事件订阅 - 长连接接收事件」** 开启长连接模式（feishu-cli 走 WebSocket，**不是** webhook URL 模式）
2. **「事件与回调 - 事件订阅」** 选中目标 EventType（与 `event schema <key>` 输出的 Event Type 一致）并**发布版本**
3. **scope 开通**：每个 EventKey 需要的 scope 见 `event schema <key>` 的 `Scopes` 字段；在「权限管理」页面开通。`event` 域已加入 `--domain event --recommend` 推荐列表，可一次性申请 IM/contact/calendar/drive/approval/vc 常用 scope 并集

### 常见错误

| 现象 | 原因 | 解决 |
|---|---|---|
| WS 连接失败，stderr 报 ws error | 长连接模式未开启 | 飞书开放平台开启「事件订阅 - 长连接接收事件」 |
| 启动后看到 ready，但收不到事件 | 目标 EventType 未在「事件订阅」勾选 / 未发版本 | 重新勾选 + 发版 |
| 收到事件但 payload 字段缺失 | App 缺对应 scope（如 `im:message.group_msg`） | `event schema <key>` 看 Scopes，去权限管理页开通后重新订阅 |
| `event consume` 立即退出 reason=error | App ID/Secret 错 / 网络不通 / 域名走 lark 但 BaseURL 用了 feishu | 检查 `config.yaml`；`lark` 国际版需 `--base-url https://open.larksuite.com` 或对应配置 |

## AI Agent 后台订阅推荐用法

### 单 EventKey 后台订阅（`run_in_background=true`）

```python
# 1. 后台启动 consume，stderr/stdout 各 redirect
task = Bash(
    command='feishu-cli event consume im.message.receive_v1 --output-dir ./events 2> consume.log',
    run_in_background=True,
)

# 2. tail consume.log 阻塞等 "[event] ready event_key=im.message.receive_v1"

# 3. 额外 sleep 1-3s 让 WS 握手完成

# 4. 业务逻辑：tail stdout / 读 ./events/*.json 处理新事件

# 5. 退出：feishu-cli event stop --event-key im.message.receive_v1
#         或父进程 kill 后台 Bash task（SIGTERM 触发 graceful shutdown + unregister）
```

### 子进程 stdin EOF 协议（非 TTY）

非 TTY 模式下，**关闭 stdin 即触发优雅退出**（reason=signal）。Python `subprocess.Popen` 用 `stdin=subprocess.PIPE`，处理完后 `p.stdin.close()` 比 SIGTERM 更稳——consume 会跑完当前事件再退出。

### 限制单跑时长 / 事件数

调试场景永远先用 `--max-events N --timeout Ds`，避免忘了 stop 留下后台进程吃 API quota：

```bash
feishu-cli event consume im.message.receive_v1 --max-events 1 --timeout 30s
# 抓 1 条事件 demo / 30 秒超时双保险
```

## 踩坑与注意事项

- **daemon 进程持久**：`event consume` 阻塞运行直到信号/超时/EOF；**不会自己退出**。AI Agent 后台跑必须配 `--max-events` / `--timeout` 或显式 `event stop`，否则会留下长跑进程
- **flock 跨进程互斥**：bus.json 读写都走 flock，多个 `event consume` 同时启动注册是安全的；但**不要手动编辑** bus.json
- **pipe broken 自动退出**：下游 jq / tee 关闭 stdout（典型场景：`event consume ... | head -1`）会触发 SIGPIPE，consume 主动 cancel 退出 reason=signal，不会卡死等 Ctrl-C
- **`--quiet` 不影响 ready marker**：ready marker 走真实 `os.Stderr` 绕过 `--quiet` 重定向，所以 AI Agent 即使开 `--quiet` 父进程仍能等到 ready 行；但其他诊断（包括 `[event] exited` reason）会被静默
- **AutoReconnect 无限重试**：oapi-sdk-go v3 ws.Client 默认 `WithAutoReconnect(true)`，断线后无限重试（间隔 2 分钟 + 首次抖动）。长时间断线场景建议用 `--timeout` 主动退出，由外层守护进程拉起，比内层无限 retry 更可控
- **status 不主动 ping 进程**：`event status` 用 `signal(0)` 探活，对 PID 复用场景理论可能误判（极小概率）。`event stop --pid N` 也是 syscall.Kill，命中错 PID 会 ESRCH 失败，不会误杀
- **每条事件独立文件**：`--output-dir` 模式下每条事件落盘 `<event_id>.json`，**短时间高频事件可能创建大量小文件**；落盘只为留痕，业务消费仍推荐用 stdout NDJSON
- **`--jq` 只支持点路径**：`--jq .event.message` 把每条事件投影到子树后再输出；不命中的事件**会被 skip**（不输出空行）。复杂过滤永远走 pipe 外部 jq
- **`--output-dir` 不支持 `~`**：传 `~/events` 会报错；用相对路径 `./events` 或绝对路径 `/Users/xxx/events`

## 何时转其他 skill

| 任务 | 路由 |
|---|---|
| **发**消息 / 回复 / 卡片 / 通知 | **feishu-cli-msg** |
| 构造 interactive 卡片 JSON | **feishu-cli-card** |
| 处理收到的消息事件 → 写多维表格 | **feishu-cli-bitable**（解析 payload 后调 record 命令） |
| 处理收到的审批事件 → 查审批详情 | **feishu-cli-toolkit**（approval 子命令） |
| 收到群消息后查群信息/成员 | **feishu-cli-chat** |
| Webhook URL 模式（HTTP 回调，非长连接） | 不在本技能范围；走飞书开放平台的「请求网址配置」+ 自建 HTTP server |
| 历史消息批量拉取（非实时） | **feishu-cli-chat** 的 `msg history` / `msg list` |

## 参考

- 飞书开放平台事件订阅文档：https://open.feishu.cn/document/server-docs/event-subscription-guide/event-list
- 项目 CHANGELOG：本模块新增详情见仓库 `CHANGELOG.md` `event 模块` 段落
- 源码：`cmd/event*.go` + `internal/event/{bus,keys,runtime}.go`

## 安全 — event_id 文件名净化

`--output-dir` 启用时每条事件 dump 为 `<event_id>.json`。v1 PR 加 `sanitizeEventID` 防御：
- 只保留 `[A-Za-z0-9_-]` 字符，长度截到 128
- `..`、`/`、空格、特殊符号都被丢弃
- 净化后空串 → 跳过 dump（不写空文件名文件）

防御场景：服务端 payload 异常或恶意构造 `header.event_id = "../etc/passwd"` 类 payload 时，writeFile 不会逃出 `--output-dir`。
