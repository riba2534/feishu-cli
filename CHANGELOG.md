# Changelog

所有重要的项目变更都会记录在此文件。

版本格式：[MAJOR.MINOR.PATCH](https://semver.org/lang/zh-CN/)

## 未发布

### 新增 — `event` 模块（WebSocket 实时事件订阅 + daemon 进程管理）

新增 `feishu-cli event` 命令族，对齐 lark-cli `event` shortcut（list / schema / consume / status / stop），
通过飞书 WebSocket 长连接接收应用事件并以 NDJSON 输出到 stdout。

**背景**：lark-cli 提供了完整的 `event consume <EventKey>` 实时事件订阅链路（含 daemon + bus.json 状态文件），
feishu-cli 之前完全没有事件订阅能力，AI Agent 想做 bot 实时响应只能切换到 lark-cli 或自写 WebSocket
客户端。本 PR 在 feishu-cli 代码风格下重新实现，让单工具栈即可完成消息接收、群成员变更监听、审批
事件订阅等长连接场景。

**新增子命令**：

- `event list [--json]` — 列出所有支持的 EventKey（按 domain 分组：im / contact / calendar / drive / approval / vc 共 22+ 个）
- `event schema <key> [--json]` — 查看某个 EventKey 的 EventType / scope / payload schema 示例
- `event consume <key>` — 启动 WebSocket 长连接订阅（阻塞，事件流→stdout NDJSON）
- `event status [--json]` — 查看本机所有 consume 进程（PID/EventKey/启动时间/uptime）
- `event stop {--pid N | --event-key K | --all} [--force] [--json]` — 停止 consume 进程

**Consume 关键 flag**：

- `--max-events N` — 接收 N 条事件后退出（0=不限制）
- `--timeout 30s` — 运行时长上限（0=不限制）
- `--jq .event.message` — 极简点路径过滤（不支持完整 jq 语法，用 pipe 接外部 jq）
- `--output-dir ./events` — 每条事件 dump 为 `<event_id>.json` 落盘
- `--quiet` — 抑制 stderr 诊断（AI Agent 慎用，会一起抑制 ready marker）

**Daemon / 进程模型**：

- 每个 `event consume` 进程 = 一个独立 OS 进程 + 一个 WebSocket 长连接（一个 EventKey）
- 状态文件 `~/.feishu-cli/events/<app_id>/bus.json`（每个 AppID 一个目录）：consume 启动写入 PID/EventKey/启动时间，退出时移除
- 跨进程互斥 `~/.feishu-cli/events/<app_id>/bus.lock`（flock 文件锁，fd 关闭自动释放）
- 原子写：tmp + os.Rename 防止半写
- 进程探活：signal(0) 检测 PID 存活，status 命令自动清理僵尸条目
- 重连策略：复用 oapi-sdk-go v3 `ws.Client.WithAutoReconnect(true)`，断线无限重试（间隔 2 分钟 + 首次随机抖动）
- 与 lark-cli 差异：lark-cli 用独立 daemon + Unix socket 做事件 fan-out；feishu-cli 简化为每个 consume 直连 WebSocket，
  不做事件分发——足够覆盖 AI Agent 单 EventKey 订阅的主线场景，省去 IPC 复杂度

**Subprocess 协议**（兼容 AI Agent 子进程调度）：

- 启动后 stderr 立即输出 `[event] ready event_key=<key>`，父进程应阻塞 stderr 等该行出现后再读 stdout
- 非 TTY 模式下 stdin EOF = shutdown 信号（适配 `< /dev/null` / `nohup` 等场景）
- 退出码 0：正常退出（达到 --max-events / --timeout / SIGTERM / Ctrl-C），非 0：startup 失败或 ws 不可恢复错误

**Scope 要求**：默认 App Token；具体 scope 因 EventKey 而异（`event schema <key>` 查看）。已加入
`--domain event --recommend` 推荐列表，覆盖 IM/联系人/日历/云盘/审批/VC 常用 scope 并集。

**代码影响范围**：

- 新增 `cmd/event.go`（顶层命令）+ `cmd/event_{list,schema,consume,status,stop}.go`（5 个子命令）
- 新增 `internal/event/{keys,bus,runtime}.go`（EventKey 注册表 + bus.json 状态管理 + WebSocket runtime）
- 新增 `cmd/event_test.go` + `internal/event/{keys,bus,runtime}_test.go`（mock 单测）
- 新增 `cmd/event_smoke_test.go`（`//go:build smoke` 本地真实 WebSocket 端到端测试）
- 修改 `internal/registry/domain_alias.go`：新增 `event` domain scope 推荐列表
- 修改 `go.sum`：补全 `larksuite/oapi-sdk-go/v3/ws` 子包的传递依赖（gorilla/websocket、gogo/protobuf；均为 indirect，无新顶层依赖）
### 新增 — `attendance` 考勤查询模块

新增 `attendance` 顶层命令组（别名 `att`），覆盖飞书考勤 OpenAPI 两类查询：

- `feishu-cli attendance user-task query` —— 按日期范围查询用户上下班打卡记录
  （`POST /open-apis/attendance/v1/user_tasks/query`，单次最多 50 用户）
- `feishu-cli attendance user-stats query` —— 查询日度 / 月度考勤统计
  （`POST /open-apis/attendance/v1/user_stats_datas/query`，单次最多 200 用户，
  起止跨度 ≤ 31 天）

**特性**：

- 日期参数同时接受 `YYYY-MM-DD` 与 `YYYYMMDD`，自动转换为 API 所需的 `yyyyMMdd` 整数
- 输出双模：默认 `text` 人类可读（打卡时间 / 结果 / 加班标记 / 统计字段标题），
  `-o json` 直出归一化结构体，便于 AI Agent 与脚本消费
- 同时打印 `invalid_user_ids` / `unauthorized_user_ids`，提示无效或无权限用户
- user-task ≤ 50、user-stats ≤ 200 用户数本地预校验，避免无谓远程请求
- user-stats 起止跨度 > 31 天本地预校验，避免触发 OpenAPI 报错
- 全部命令走 tenant_access_token（即应用身份）：larksuite/oapi-sdk-go v3.5.3 中
  `Attendance.UserTask.Query` / `Attendance.UserStatsData.Query` 的
  `SupportedAccessTokenTypes` 仅含 `Tenant`，传入 user token 会被 SDK 拒绝

**权限要求**：应用需在飞书开放平台「应用权限管理」页面获得
`attendance:task:readonly` 权限（tenant 级），无需 `auth login`。
### 新增 — `msg flag`：消息书签（收藏 / 列表 / 取消）

新增 `feishu-cli msg flag {create,list,cancel}` 三个子命令，对应飞书 OpenAPI `/im/v1/flags`，
覆盖消息书签的完整生命周期。

**支持的两层书签模型**：

| item_type  | flag_type | 场景                                |
| ---------- | --------- | ----------------------------------- |
| default    | message   | 消息层书签（最常见，默认值）        |
| thread     | feed      | topic-style 话题群 feed 层（侧边栏）|
| msg_thread | feed      | 普通群消息线程 feed 层              |

其余组合服务端会拒绝，CLI 默认值为 `default + message` 即可覆盖 90% 用例。

**实现说明**：飞书 Open SDK v3 当前未封装 flag 接口，使用通用 HTTP client（`client.Post` /
`client.Get`）直接调用，与 `comment reply add` 同套路。

**权限要求**：User Token，scope `im:flag`
### 新增 — `okr` 模块：OKR 周期和进展记录

新增 `feishu-cli okr` 命令组，覆盖 OKR 最高频的 3 个操作：

- `okr cycle list` — 获取当前租户的所有 OKR 周期（`/open-apis/okr/v1/periods`，租户级全局列表，自动分页）
- `okr progress list --objective-id 7xxx | --key-result-id 7xxx` — 列出某个目标 / 关键结果下的所有进展记录
- `okr progress create --objective-id 7xxx | --key-result-id 7xxx --content "..."` — 创建一条进展记录，
  支持纯文本（`--content`，自动包装为 ContentBlock）或原始富文本（`--content-json`）；
  可附带 `--progress-percent` + `--progress-status` 标记进度；
  `--source-url` 飞书侧必填，CLI 默认填 `https://www.feishu.cn/okr/progress` placeholder，可显式覆盖

**实现要点**：

- `progress create` 走飞书 Open SDK v3.5.3 的 `Okr.ProgressRecord.Create`；
  `cycle list` 走通用 HTTP client 直调 `/open-apis/okr/v1/periods`（v1/periods 是租户级，不按用户过滤）；
  `progress list` 走通用 HTTP client 直调 `/open-apis/okr/v2/...`
- 所有命令默认使用 User Token，会自动读取 `~/.feishu-cli/token.json`；
  也可以通过 `--user-access-token` 或 `FEISHU_USER_ACCESS_TOKEN` 显式覆盖
- 时间戳统一格式化为本地时区 `YYYY-MM-DD HH:MM:SS`，方便人眼阅读

**权限要求（User Token scope）**：

| 命令              | scope                                              |
|-------------------|----------------------------------------------------|
| `cycle list`      | `okr:okr:readonly` 或 `okr:okr.period:readonly`    |
| `progress list`   | `okr:okr:readonly` 或 `okr:okr.progress:readonly`  |
| `progress create` | `okr:okr` 或 `okr:okr.progress:writeonly`          |

**使用示例**：

```bash
feishu-cli auth login --domain event --recommend

# 列出所有 EventKey
feishu-cli event list

# 查看 IM 接收消息事件的字段
feishu-cli event schema im.message.receive_v1

# 订阅（Ctrl-C 退出）
feishu-cli event consume im.message.receive_v1

# 调试：抓 5 条事件后自动退出
feishu-cli event consume im.message.receive_v1 --max-events 5 --timeout 60s

# 并发订阅多个 EventKey（每个进程一个 EventKey）
feishu-cli event consume im.message.receive_v1 > receive.ndjson 2> receive.log &
feishu-cli event consume im.chat.member.user.added_v1 > member.ndjson 2> member.log &
feishu-cli event status                       # 查看活跃进程
feishu-cli event stop --all                   # 一键停止
```

### 新增 — `doctor` 命令（健康检查 / 配置 / 认证 / 网络 / 依赖一把验）

新增 `feishu-cli doctor` 命令，对齐 lark-cli `doctor` 体验，跑一组本地诊断快速验证 CLI 状态。

**6 项检查**：
- `config_file` — app_id / app_secret 是否就位
- `user_token` — token.json 状态（valid / needs_refresh / expired）
- `endpoint_open` — `open.feishu.cn` HTTPS 可达性 + RTT
- `endpoint_larksuite` — `open.larksuite.com` HTTPS 可达性 + RTT
- `proxy` — HTTPS_PROXY 与 NO_PROXY 配置（缺飞书域 warn）
- `dependencies` — Go 版本 + larksuite/oapi-sdk-go 版本

**flag**：`--json` 机器可读输出 / `--offline` 跳过网络检查 / `--only user_token,proxy` 仅运行指定项。

**退出码**：0 = 全 pass / 1 = 任一 fail。

**使用示例**：

```bash
feishu-cli doctor                              # pretty 输出全检查
feishu-cli doctor --json                       # JSON 输出（AI agent 自检友好）
feishu-cli doctor --offline                    # 跳过网络
feishu-cli doctor --only user_token,proxy      # 仅跑指定项
```

**代码影响范围**：新增 `cmd/doctor.go`（6 项检查 + pretty/JSON 输出）和 `cmd/doctor_test.go`（parseOnly / shouldRun / proxy / dependencies 单测）；不引入新依赖。

### 新增 — `slides` 模块：Slides 演示文稿创建与媒体上传

新增 `feishu-cli slides` 顶层命令，提供两个子命令支撑 Slides 演示文稿的最小可用工作流：

- `slides create [--title <name>] [--width <px>] [--height <px>] [--output json]`
  调用 `POST /open-apis/slides_ai/v1/xml_presentations` 创建空白演示文稿，返回 `xml_presentation_id` /
  `revision_id` / `title`。默认尺寸 960x540。
- `slides media-upload --file <path> --presentation-token <xml_presentation_id> [--output json]`
  本地图片走 `/open-apis/drive/v1/medias/upload_all` 上传到指定演示文稿，返回 `file_token`
  可直接作为 slide XML 中 `<img src="...">` 引用。

**关键实现细节**：

- 上传 `parent_type` 固定为 `slide_file`（lark-cli 实测：`slide_image` / `slides_image` /
  `slides_file` 都会被拒）；`parent_node` 必须为目标 `xml_presentation_id`
- 单文件上限 20 MB（多分片 `upload_prepare` 不接受 `parent_type=slide_file`）
- 共用 `internal/client/drive.go::UploadMediaWithExtra` 上传链路

**权限要求**：

- 创建：`slides:presentation:create` 或 `slides:presentation:write_only`
- 上传：`docs:document.media:upload`
### 新增 — `mail` 高级能力：CID 内联图片 + 邮件模板（MVP）

为 `mail` 模块补齐两块进阶能力，对齐 lark-cli `mail +send` / `mail +template-create` 的核心子集。

**1. `mail send --inline-images-auto-scan`（CID 内联图片）**

HTML body 中所有 `<img src="本地相对/绝对路径">` 会被自动扫描：

1. 跳过已经是 `cid:` / `http(s):` / `data:` / `//` scheme 的引用
2. 同一本地路径只上传一次（去重）
3. 每张图独立生成 20-hex CID
4. 走 `drive/v1/medias/upload_all`（`parent_type=email`，`parent_node = 当前登录用户 open_id`）
5. EML 走 `multipart/related`：HTML 段 + 每张图一个 `Content-ID: <cid>`、`Content-Disposition: inline` 的 part
6. 改写 `src` 为 `cid:<cid>` 后回写到 body

依赖 `~/.feishu-cli/user_profile.json` 中缓存的 open_id（`auth login` 后自动写入）。

**2. `mail template create` / `mail template list`（邮件模板 MVP）**

- `mail template create --name xxx --subject xxx --body xxx [--to ... --cc ... --bcc ... --plain-text]`
  调用 `POST /open-apis/mail/v1/user_mailboxes/{id}/templates`
- `mail template list [--mailbox me]`
  调用 `GET /open-apis/mail/v1/user_mailboxes/{id}/templates`（接口不分页，一次返回所有 id+name）

底层 client 也实现了 `GetMailTemplate` / `UpdateMailTemplate` / `DeleteMailTemplate`，但 CLI 层目前只暴露 create/list（MVP）。

**权限要求**：

- User Access Token
- `mail:user_mailbox:readonly` / `mail:user_mailbox.message:modify` / `mail:user_mailbox.message:send`
- 模板相关 scope：`mail:user.email.template`

⚠️ **字节租户 `mail:user.email.template` 暂未开放** —— 命令本身实现完整、参数校验完整、EML/JSON payload
正确，但调用模板 API 时可能返回 scope 校验失败（401/permission denied）；等飞书侧开放该 scope 后立即可用。
CID 内联图片功能不依赖此 scope，已可正常使用。

**示例**：

```bash
# 创建一个标题为 "Q2 OKR" 的演示文稿
feishu-cli slides create --title "Q2 OKR" --output json

# 把封面图上传到该演示文稿
feishu-cli slides media-upload --file ./cover.png \
    --presentation-token <xml_presentation_id>
```
### 新增 — `schema` 命令：本地浏览飞书 OpenAPI 方法（path / 参数 / scope）

新增 `feishu-cli schema [service.resource.method]` 子命令，无需 token、纯本地查询飞书
开放平台 OpenAPI 方法的 HTTP path / 动词 / 参数 / 请求体 / 响应体 / scope / 文档链接。
对齐 `larksuite/cli` 的 `schema` 子命令，便于 AI Agent 和脚本作者快速查找参数。

**用法**：

```bash
feishu-cli schema                                # 列出所有可用 service（12 个）
feishu-cli schema im                             # 列出 im 域下所有 resource.method
feishu-cli schema im.messages                    # 列出 messages 资源下所有 method
feishu-cli schema im.messages.delete             # 查看具体 method 详情
feishu-cli schema im.messages.delete --format json   # JSON 输出（AI Agent 推荐）
feishu-cli schema list --service drive           # 等价于 schema drive，支持 --format json
```

**数据源**：`internal/registry/meta_data.json`（编译期 embed），与认证模块复用同一份元数据。
当前覆盖 12 个 service：approval / attendance / calendar / drive / im / mail / minutes /
sheets / slides / task / vc / wiki。

**输出含**：HTTP verb + 完整 path、parameters（含 path / query / required 标记）、
requestBody（嵌套字段）、responseBody、accessTokens（user / tenant）、scopes、docUrl。

# 查询本人最近一周打卡
feishu-cli attendance user-task query \
    --employee-type open_id \
    --user-ids ou_xxxxxxxxx \
    --start 2026-05-01 --end 2026-05-18

# 查询本月日度统计（JSON 输出）
feishu-cli attendance user-stats query \
    --employee-type open_id \
    --user-ids ou_xxxxxxxxx --current-user-id ou_xxxxxxxxx \
    --stats-type daily --start 2026-05-01 --end 2026-05-31 -o json
# 收藏消息（消息层）
feishu-cli msg flag create om_xxx

# 列出当前用户所有书签
feishu-cli msg flag list --page-size 50

# 取消书签（参数需与 create 一致）
feishu-cli msg flag cancel om_xxx

# feed 层书签（普通群线程）
feishu-cli msg flag create om_xxx --item-type msg_thread --flag-type feed
```
feishu-cli auth login --scope "okr:okr"
feishu-cli okr cycle list
feishu-cli okr progress list --objective-id 7xxx
feishu-cli okr progress create --key-result-id 7xxx --content "本周完成核心模块联调"
```

**MVP 范围说明**：本次只覆盖最常用的 3 个动词，progress update / delete / get 和图片上传暂不暴露
为命令行（client 层已有实现，后续按需补 CLI）。
### 新增 — `approval` 写流程：发起 / 撤回 / 抄送 / 通过 / 拒绝

补齐审批模块的写能力，原本只有 `approval get`（定义查询）和 `approval task query`（任务列表查询）两条只读命令，现在可以完整完成审批生命周期：

- `feishu-cli approval instance create` — 发起一条审批实例，`--form` 或 `--form-file` 传表单 JSON
- `feishu-cli approval instance cancel` — 撤回（取消）已发起的审批实例
- `feishu-cli approval instance cc` — 把审批实例抄送给一个或多个用户（`--cc-user-ids ou_a,ou_b`）
- `feishu-cli approval task approve` — 通过指定审批任务，可附 `--comment`
- `feishu-cli approval task reject` — 拒绝指定审批任务，建议在 `--comment` 中填写原因

**权限要求**：User Token + `approval:approval` scope（实例侧 `approval:instance:write` / 任务侧 `approval:task:write` 已包含其中）。

**底层 API**：

- `POST /open-apis/approval/v4/instances`
- `POST /open-apis/approval/v4/instances/cancel`
- `POST /open-apis/approval/v4/instances/cc`
- `POST /open-apis/approval/v4/tasks/approve`
- `POST /open-apis/approval/v4/tasks/reject`

不在本 MVP 范围（后续按需补）：`tasks/transfer`（转交）、`tasks/rollback`（退回）、`tasks/add_sign`（加签）、`tasks/remind`（催办）。

**代码影响范围**：

- `internal/client/approval.go`：新增 5 个 client 函数（`CreateApprovalInstance` / `CancelApprovalInstance` / `CCApprovalInstance` / `ApproveApprovalTask` / `RejectApprovalTask`）+ 4 个对应 Options 结构 + 共享 POST helper `doApprovalPost`
- `cmd/approval_instance.go`：新增 `approval instance` 父命令
- `cmd/approval_instance_{create,cancel,cc}.go`：3 条实例侧子命令
- `cmd/approval_task_{approve,reject}.go`：2 条任务侧子命令，复用 `readApprovalTaskActionFlags` 校验
### 新增 — `sheet filter-view` + `sheet dropdown`：筛选视图与下拉菜单

补齐 lark-cli 独占的两块电子表格高级能力：

- **筛选视图 CRUD（V3 API）**：用 SDK `SpreadsheetSheetFilterView` 实现
  - `feishu-cli sheet filter-view create --token <t> --sheet-id <s> --range "<sheetId>!A1:H14" [--name 视图名 --filter-view-id 自定义ID]`
  - `feishu-cli sheet filter-view list --token <t> --sheet-id <s>`
  - `feishu-cli sheet filter-view delete --token <t> --sheet-id <s> --filter-view-id <fv>`
  - `--range` 不带 sheetId 前缀时自动补全为 `<sheet-id>!<range>`
- **下拉菜单（V2 dataValidation API）**：list 类型数据验证
  - `feishu-cli sheet dropdown set --token <t> --range "<sheetId>!A1:A100" --options "待办,处理中,已完成" [--multiple --colors "#FF4D4F,#FAAD14,#52C41A"]`
  - `--options-json '["a, b","c"]'`：选项内含逗号时绕过 CSV 解析
  - 传 `--colors` 自动开启 `highlightValidData`，颜色数量需与选项一致

**权限**：`sheets:spreadsheet`（User Token 或 App Token 均可），命令默认 `resolveOptionalUserTokenWithFallback` 自动读取登录态。

**代码影响范围**：

- `internal/client/sheets.go`：新增 `CreateFilterView` / `ListFilterViews` / `DeleteFilterView` / `SetDropdown`
- `cmd/sheet_filter_view.go`、`cmd/sheet_dropdown.go`：CLI 入口
### 新增 — `markdown {create,fetch,overwrite}`：Drive 原生 .md 文件 CRUD

新增 `feishu-cli markdown` 顶层命令，把 Drive 上的 `.md` 当作普通文件整体读写，
保留原始 Markdown 格式（**不做** Markdown ↔ 飞书 docx 块的转换）。

**与 `doc import` / `doc export` 的区别**：

| 命令 | 行为 | 创建出的文档类型 |
|------|------|------------------|
| `doc import/export` | Markdown ↔ 飞书 docx 块（标题/列表/表格/Callout…） | docx |
| `markdown create/...` | 把 `.md` 整体上传/下载，不做转换 | file（普通 Drive 文件） |

适合 AI agent 把生成的 Markdown 直接落盘到飞书 Drive、下次读回时仍是原汁原味
Markdown 源码的场景。

**子命令**：

- `markdown create --name xxx.md --content "..." | --content-file path.md [--folder-token fldxxx]`
  从字符串或本地文件创建 `.md`；强制 `.md` 后缀；空内容报错；底层走
  `client.UploadFileWithToken`（≤ 20MB 单次上传，> 20MB 复用现成分片管线）。

- `markdown fetch --file-token boxcnxxx [--output path | -]`
  缺省 `--output` 时直接打印到 stdout（行为与 lark-cli `markdown +fetch` 一致）；
  指定路径则落盘，目录会拼 `fileToken.md`，`--overwrite` 防误覆。

- `markdown overwrite --file-token boxcnxxx --content "..." | --content-file path.md [--name renamed.md]`
  覆盖现有 `.md` 的内容，`file_token` 保持不变；`--name` 可选改名。
  **实现细节**：飞书 Go SDK v3.5.3 的 `UploadAllFileReqBody` 没有暴露 `file_token`
  字段，因此本命令用 `client.Post` + `*larkcore.Formdata` 自己拼 multipart，
  endpoint 仍是官方的 `POST /open-apis/drive/v1/files/upload_all`，参考 lark-cli
  `shortcuts/markdown/helpers.go` 的写法。

**权限**：User Access Token + `drive:file:upload` / `drive:file:download`
（或 `drive:drive`）。

# 内联图片
feishu-cli mail send --to user@example.com --subject "周报" \
    --body '<p>看附图</p><img src="./screenshot.png">' \
    --inline-images-auto-scan --confirm-send

# 模板创建+列表
feishu-cli mail template create --name "周报" --subject "本周进度" --body "<p>模板</p>"
feishu-cli mail template list
### 新增 — `calendar` 智能化三件套（suggestion / room-find / rsvp）

针对 AI Agent 自动排会场景，补齐三条飞书日历开放能力，使整条「选时段 → 选会议室 → 答复邀请」
流水线全部可在 CLI 完成。

- **`calendar suggestion`**：智能时段建议。直调 `POST /open-apis/calendar/v4/freebusy/suggestion`，
  按 `--attendee-ids ou_xxx,oc_yyy` + `--duration 30m/1h30m/90` 推荐可用时段；支持
  `--start`/`--end` 搜索窗口（默认当天）、`--exclude start~end,...` 排除午休/已占用时段、
  `--event-rrule` 周期性规则、`--timezone`。返回带「推荐理由」+「AI 行动指引」。
- **`calendar room-find`**：会议室查找。直调 `POST /open-apis/calendar/v4/freebusy/room_find`，
  支持多个 `--slot start~end` 并发查询（worker=10），可按 `--city`/`--building`/`--floor`/
  `--room-name`（逗号分隔多个）/`--min-capacity`/`--max-capacity` 多维度过滤；可选
  `--attendee-ids` 让服务端结合参与者位置筛选。
- **`calendar rsvp`**：答复日程邀请。走 SDK Reply 接口，`--calendar-id`（可省略，默认主日历）+
  `--event-id` + `--action accept|decline|tentative`。与既有的 `calendar event-reply`
  位置参数风格互为补充——rsvp 全 flag 风格、calendar-id 可省，更适合 AI Agent 调度。

**SDK 现状**：v3.5.3 暴露 `Reply` 但未暴露 `freebusy/suggestion` 和 `freebusy/room_find`，
故 suggestion / room-find 走 `client.Post` 通用 HTTP 直调 OpenAPI；新增 client 函数集中在
`internal/client/calendar_smart.go`，包括 `SuggestFreebusy`、`FindMeetingRoom`、
`FindMeetingRoomBatch`（并发+排序）、`SplitAttendeeIDs`（按 `ou_`/`oc_` 前缀分流）。

**权限要求**：
- suggestion / room-find：`calendar:calendar.free_busy:read`（User Token 或 App Token 均可）
- rsvp：`calendar:calendar.event:reply`（推荐 User Token，以本人身份答复）

**典型用法**：

```bash
# 1. 先让飞书推荐可用时段
feishu-cli calendar suggestion --attendee-ids ou_aaa,ou_bbb --duration 30m

# 2. 锁定时段后查会议室
feishu-cli calendar room-find \
  --slot 2024-01-22T09:00:00+08:00~2024-01-22T09:30:00+08:00 \
  --building "飞书大厦" --min-capacity 6

# 3. 收到邀请后答复
feishu-cli calendar rsvp --event-id EVENT_xxx --action accept
```

### 新增 — `comment reply add`：为已有评论添加回复

新增命令 `feishu-cli comment reply add <file_token> <comment_id> --text "..."`，补齐评论回复
生命周期的最后一块拼图（此前只有 list / delete）。

**背景**：飞书 Open SDK v3.5.3 的 `fileCommentReply` 只暴露 `List`/`Delete`/`Update`，没有
`Create` 方法，而 Open API 本身是支持的（`POST /drive/v1/files/:token/comments/:comment_id/replies`）。
此 PR 不依赖 SDK 升级，用通用 HTTP client（`client.Post`）直接调用 API 实现。

**同时改进**：

- `comment reply add` / `delete` / `list` 全部加上 `--user-access-token` 参数支持，并走
  `resolveOptionalUserTokenWithFallback` 自动读取登录态，和 msg/chat/doc export 等模块保持一致
- **重要修复**：`comment reply delete` 在 App Token（Bot 身份）下调用飞书侧会返回 `1069303
  forbidden`——飞书只允许回复作者本人删除。现在命令默认优先使用 User Token（如果已登录），
  行为才符合用户预期。命令帮助中也显式说明了这个权限模型
- `comment reply add` 默认也走 User Token fallback，回复会以用户身份发布（而非显示为 Bot），
  且该回复能被后续 `reply delete` 正常删除

**权限要求**：`docs:document.comment:create`（User Token）

**使用示例**：

```bash
feishu-cli auth login                       # 确保有 User Token
feishu-cli comment reply add <file_token> <comment_id> --text "已处理"
feishu-cli comment reply delete <file_token> <comment_id> <reply_id>  # 自动用 User Token
```

**代码影响范围**：

- `internal/client/comment.go`：新增 `CreateCommentReply`（HTTP client 直调），
  `ListCommentReplies` / `DeleteCommentReply` 签名增加 `userAccessToken` 参数
- `cmd/comment_reply.go`：新增 `addReplyCmd`，三个子命令统一加 `--user-access-token` flag
- `cmd/comment.go`：Long help 中补充 reply add 示例

### Features — 新增 `wiki move-docs` 命令（移动云空间文档至知识空间）

新增 `feishu-cli wiki move-docs <obj_token> --space-id <id>` 命令，对应飞书 OpenAPI `POST /open-apis/wiki/v2/spaces/{space_id}/nodes/move_docs_to_wiki`。

**解决的问题**：之前要把"我的空间 / 共享空间"里已存在的 docx / sheet / mindnote / bitable / file 挂到知识库下，只有两条路——(1) 飞书客户端手动点"添加到知识库"；(2) 走 `wiki create` 新建空文档再重写内容。前者不能自动化，后者丢原文档权限和历史。新命令一步到位。

**用法**：

```bash
# 把 drive docx 移入知识空间根目录
feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567

# 移入指定父节点
feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567 --parent-node wikcnYYYYYY

# 移动电子表格
feishu-cli wiki move-docs shtcnXXXXXX --space-id 7012345678901234567 --obj-type sheet

# 无 move 权限时提交迁入申请
feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567 --apply

# 用用户身份调用（企业版 wiki 空间不接受 app 成员，必须 user token）
feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567 --user-access-token u-xxx
```

**返回三种情况**：`wiki_token`（立即完成）/ `task_id`（异步任务）/ `applied=true`（权限不足已提交申请）。

**Scope 要求**：`wiki:node:move` 或 `wiki:wiki`，已加入 `--domain wiki --recommend` 推荐列表。

**代码影响范围**：
- 新增 `cmd/move_docs_to_wiki.go`（命令）和 `internal/client/wiki.go` 的 `MoveDocsToWiki` 函数
- `internal/registry/domain_alias.go` 的 `wiki` domain 补上 `wiki:node:move`
- README 知识库操作段落补一行命令

---

### Breaking Changes — 移除 `config add-scopes` 命令

`feishu-cli config add-scopes` 子命令及其 `--domain` / `--scopes` / `--print-only` flag 全部删除。

**删除理由**：

1. **命令几乎不可用** — 硬编码的 `scopeDomains` 字典里多数 scope 名已过时（`docx:document` / `sheets:spreadsheet` / `bitable:app` / `im:chat:readonly` / `drive:export:readonly` / `vc:room:readonly` 等都是飞书不支持的粗粒度名称），生成的申请链接里多数 scope 会被后台拒绝
2. **权限开通不适合自动化** — 飞书开放平台的权限申请通常需要 tenant 管理员审批，scope 选择也是业务决策而非技术"默认值"。CLI 自动化只会造成"看起来装好了但后台还没批"的幻觉
3. **有更简单的替代** — 飞书开放平台的应用权限管理页面支持"导入权限 JSON"入口，复制 [README 权限要求](../README.md#权限要求) 章节里的完整权限清单一次性粘贴即可开通 400+ 个 scope

**迁移指引**：

旧：
```bash
feishu-cli config add-scopes --domain all
```

新：
1. 打开飞书开放平台 → 你的应用 → 权限管理页面
2. 复制 README 的完整权限 JSON（tenant + user 两套 400+ scope）
3. 粘贴到"导入权限"入口，一键开通全部
4. 等待 tenant 管理员审批（如果需要）

**代码影响范围**：
- 删除 `cmd/config_add_scopes.go` 整个文件
- `cmd/auth_check.go` 的 `suggestion` 文案改为引导用户去开放平台开通（不再推荐 `config add-scopes`）
- README / CLAUDE.md / AGENTS.md / 6 个 skill 的 `config add-scopes` 引用全部更新为"去开放平台开通"
- 保留 `config create-app --save`（Device Flow 创建应用）不变

---

### Breaking Changes — 多维表格（bitable）切换到 `base/v3` API

**旧实现**：`bitable` 模块全部调用 `/open-apis/bitable/v1/apps/{app_token}/...` 老 API，覆盖 ~30 个基础 CRUD 命令。
**新实现**：全面切换到 `/open-apis/base/v3/bases/{base_token}/...` 新 API，覆盖 48 个命令，支持深度能力（视图完整配置读写、记录 upsert、修改历史、角色 CRUD、高级权限、数据聚合、工作流查询）。

#### 命令名迁移表

| 旧命令 | 新命令 |
|---|---|
| `bitable tables <app>` | `bitable table list --base-token <t>` |
| `bitable create-table <app>` | `bitable table create --base-token <t> --name x` |
| `bitable rename-table <app> <tbl>` | `bitable table update --base-token <t> --table-id <tbl> --name x` |
| `bitable delete-table <app> <tbl>` | `bitable table delete --base-token <t> --table-id <tbl>` |
| `bitable fields <app> <tbl>` | `bitable field list --base-token <t> --table-id <tbl>` |
| `bitable create-field` | `bitable field create` |
| `bitable update-field` | `bitable field update`（method 改为 `PUT`） |
| `bitable delete-field` | `bitable field delete` |
| `bitable records <app> <tbl>` | `bitable record list --base-token <t> --table-id <tbl>` |
| `bitable get-record` | `bitable record get` |
| `bitable add-record` | `bitable record upsert --base-token <t> --table-id <tbl> --config '...'` |
| `bitable add-records --data-file` | `bitable record batch-create --config-file ...` |
| `bitable update-record` | `bitable record upsert --record-id ...`（根据是否传 id 自动 PATCH/POST） |
| `bitable delete-records` | `bitable record delete --record-id ...` |
| `bitable views` | `bitable view list` |
| `bitable create-view` | `bitable view create` |
| `bitable delete-view` | `bitable view delete` |
| `bitable view-filter get/set` | `bitable view view-filter-get / view-filter-set` |
| `bitable dashboard list`（v1） | **暂不支持**（v3 dashboard CRUD 留待下次迭代） |
| `bitable form list` | **暂不支持** |
| `bitable role list` | `bitable role list`（新增 get/create/update/delete） |
| `bitable workflow list/enable` | `bitable workflow list`（改为 POST /workflows/list） |
| `bitable advperm enable/disable` | 同名但底层改为 `PUT .../advperm/enable?enable=true/false` |
| `bitable data-query` | 同名但路径从 table 级改为 base 级：`POST .../bases/{t}/data/query` |

#### 新增能力

- **视图配置完整写入**：`view-sort-set` / `view-group-set` / `view-visible-fields-set` / `view-timebar-set` / `view-card-set`（老 v1 只能写 filter）
- **记录修改历史**：`bitable record history-list --record-id xxx`
- **角色 CRUD**：`bitable role create/update/delete`（老 v1 只有 list）
- **字段选项搜索**：`bitable field search-options`
- **Base create 支持时区**：`--time-zone Asia/Shanghai`

#### Flag 变化
- **删除 `--app-token` 别名**：只保留 `--base-token`（与 base/v3 API 命名一致，不再做兼容别名）
- `bitable create` 的 `--description` 被删除（base/v3 不支持），新增 `--time-zone`
- `bitable data-query` 的 `--table-id` 被删除（v3 端点挂在 base 下）

#### 删除的文件
- `internal/client/bitable.go` / `bitable_test.go`（v1 实现）
- `cmd/bitable_create.go` / `bitable_get.go` / `bitable_copy.go` / `bitable_advperm.go` / `bitable_dashboard.go` / `bitable_data_query.go` / `bitable_form.go` / `bitable_record_upload_attachment.go` / `bitable_role.go` / `bitable_view_config.go` / `bitable_workflow.go`

#### 新增的文件
- `internal/client/base.go`（`BaseV3Call` + `BaseV3Path` helper + `X-App-Id` header 自动注入）
- `cmd/bitable_base.go` / `bitable_misc.go`（所有 base/v3 命令的注册）
- `cmd/bitable_table.go` / `bitable_field.go` / `bitable_record.go` / `bitable_view.go` 全部重写

---

### Breaking Changes — VC（视频会议）改造升级

- **`vc search`**：底层 API 从 `GET /meeting_list` 切换到 `POST /meetings/search`。
  - 新增 flag：`--query` / `--organizer-ids` / `--participant-ids` / `--room-ids`
  - 删除 flag：`--meeting-no` / `--meeting-status`
  - 必须指定至少一个过滤条件
- **`vc notes`**：
  - flag 从 `--meeting-id` / `--minute-token`（单数）改为 `--meeting-ids` / `--minute-tokens`（复数，支持 CSV 批量最多 50）
  - 新增第三路径 `--calendar-event-ids`：从日历事件自动反查会议 / 妙记
  - 新增开关 `--with-artifacts`（获取 AI 产物）/ `--download-transcript --output-dir`（下载逐字稿）
- **所有 vc / minutes 命令默认 User Access Token**，未登录时统一报错提示 `feishu-cli auth login`

#### 新增命令
- `vc recording --meeting-ids/-calendar-event-ids`：查询会议录制并自动提取 `minute_token`
- `minutes download --minute-tokens x,y,z --output ./dir`：批量下载妙记音视频媒体（SSRF 防护 / 重定向校验 / Content-Disposition 解析 / 文件名去重 / 5 req/s 速率限制 / `--url-only` 预览链接）
- `minutes get <token> --with-artifacts`：新增 AI 产物合并输出

---

### Added — `drive` 云盘命令组（8 个命令）

新增独立的 `drive` 子命令组，与现有 `file` / `media` / `doc media-*` 命令并存，提供增强能力：

| 命令 | 相比老命令的增强 |
|---|---|
| `drive upload` | 大文件自动分块（>20MB 走 `upload_prepare/part/finish` 三步式，每片独立重试 3 次；支持 User Token） |
| `drive download` | 流式下载 + 路径校验 + `--overwrite` / `--timeout` |
| `drive export` | 新增 **markdown 快捷路径**：docx → markdown 走 `/docs/v1/content` 直接拉取，不跑异步 export task；支持 sheet / bitable 按 `--sub-id` 导出 CSV；有界轮询（10×5s）+ 超时返回 resume 命令 |
| `drive export-download` | 通过 `file_token` 直接下载已完成的导出任务产物，配合 `drive export` 超时后接力完成 |
| `drive import` | **切换到 `/medias/upload_*` 端点 + `parent_type=ccm_import_open` + `extra` 字段**（不再在用户云盘留下中间文件）；格式特定大小限制（docx 20MB / sheet 20MB / bitable 100MB）；有界轮询 + resume |
| `drive move` | 文件夹移动自动轮询 `task_check`（30×2s），文件移动同步返回 |
| `drive add-comment` | 支持**富文本 `reply_elements`**（text / mention_user / link）+ `--block-id` 局部评论（docx）+ **wiki URL 自动解析**成 docx token |
| `drive task-result` | 通用异步任务查询（`--scenario import/export/task_check`），配合 drive export / import / move 的超时 resume |

**保留不动**：`file list / delete / mkdir / copy / shortcut / quota / meta / stats / version` + `media upload / download` + `doc media-download / media-insert` + `comment list / resolve / delete / reply`

---

### Added — `mail` 飞书邮箱模块（10 个命令，从零新建）

**全新命令组**。首期不支持附件和 CID 内联图片，仅支持纯文本和 HTML body。所有命令默认 User Access Token。

| 命令 | 功能 |
|---|---|
| `mail message --message-id x` | 获取单封邮件（`--format full/plain_text_full/raw`） |
| `mail messages --message-ids a,b,c` | 批量获取多封邮件 |
| `mail thread --thread-id x` | 获取邮件线程 |
| `mail triage` | 列出 / 搜索邮件（`--folder INBOX --label x --query xxx --unread-only --list-folders --list-labels`），`--query` 走专用 `POST /search` 端点 |
| `mail send` | 发送邮件（**默认保存为草稿**，加 `--confirm-send` 立即发送，安全兜底） |
| `mail draft-create` | 仅创建草稿 |
| `mail draft-edit --draft-id x` | 编辑已有草稿（全量覆盖） |
| `mail reply --message-id x --body "..."` | 回复邮件（自动 `Re: ` 前缀 + 引用块 + `In-Reply-To` / `References` header 继承） |
| `mail reply-all` | 全部回复（包含 To 和 CC，自动排除自己） |
| `mail forward --message-id x --to y` | 转发（自动 `Fwd: ` 前缀 + 原文正文引用） |

**关键技术点**：
- RFC 5322 EML 构建 + base64 URL-safe 编码，`POST /drafts` body `{"raw":"..."}`
- HTML 自动检测（`<html>/<div>/<b>/<br>` 等标签），可用 `--plain-text` / `--html` 强制
- 发件人地址默认从 `/user_mailboxes/{mailbox}/profile` 读取
- 地址格式支持 `"Name <email>"` 和 `"email"`
- Subject 去重：`reply` 自动避免 `Re: Re:`，`forward` 自动避免 `Fwd: Fwd:`

---

### Fixed

- **`mail reply` 引用块缺日期占位符**：之前的 quote header 模板第一个 `%s` 传空字符串，会输出 `"在 ，xxx 写道:"`，已修正为 `"{email} 写道:"`
- **分片上传 fd 泄漏**：`uploadFileMultipart` 之前每片每次重试都 `os.Open + Seek`，现改为外层打开一次 + `io.NewSectionReader`，大文件不稳定网络下重试时节省 N×syscall
- **`mail reply` 重复 `GetMailboxProfile` 调用**：之前在 `runMailReply` 里调用 2 次（一次取 selfEmail 一次取 from/fromName），现合并为 1 次，省 1 个 API RTT
- **`drive import` 上传端点错误**：之前走 `/files/upload_all` 会在用户云盘留下中间文件，现改为官方的 `/medias/upload_all` + `parent_type=ccm_import_open` + `extra`
- **`mail triage --query` 静默失效**：之前把 query 当 list 端点的查询参数，飞书会忽略；现改走专用的 `POST /search` 端点

### Refactor（内部代码清理，用户感知较小）

- 新增 `requireUserToken(cmd, cmdName)` helper，统一所有新命令的 "需要 User Access Token" 错误信息格式
- 删除重复的 `GetWikiNodeByToken`（58 行），改用已有的 `GetWikiNode`
- 删除 `internal/client/mail.go` 的 `joinPath`，用 `strings.Join`
- `dedupStrings` 从 `vc_recording.go` 移到 `vc_common.go`
- `runBaseV3WithJSON` 重构，抽出 `runBaseV3WithBody` 让命令层直接传已构造的 body
- `bitable view create/rename` 去掉 `cmd.Flags().Set("config", ...)` + `MarkHidden` 的 hack 模式
- 删除 `runBaseV3Simple` / `addBaseTokenFlag` / `exactlyOneNonEmpty` 三处死参数/死变量
- 所有文件统一 `gofmt`

---

## [v1.18.0] - 未发布

### Breaking Changes — OAuth 认证全面切换到 Device Flow

彻底删除 Authorization Code Flow，只保留 Device Flow（RFC 8628）。本地桌面、SSH 远程、容器、CI 全环境统一使用同一条命令，**无需任何重定向 URL 白名单配置**。

#### 删除的 flag

`auth login` 命令删除以下 flag：

- `--manual` — SSH 远程手动粘贴回调模式（Device Flow 下 SSH 和本地一视同仁）
- `--no-manual` — 强制本地回调模式（本地回调 HTTP server 已移除）
- `--port` — 本地回调端口（不再需要）
- `--print-url` — 非交互两步式第一步（改用 `--no-wait` + `--device-code`）
- `--method` — 授权方式选择（Device Flow 是唯一方式）
- `--scopes` — 请求 OAuth scope（飞书 token v2 端点实际忽略此参数，返回应用预配置的全部 scope）

#### 删除的子命令

- `auth callback <url> --state <state>` — Authorization Code Flow 换 token 专用，整体删除

#### 删除的代码

- `internal/auth/oauth.go`：`Login` / `loginLocal` / `loginManual` / `buildAuthURL` / `GenerateAuthURL` / `ParseCallbackURL` / `ExchangeToken` 等函数
- `internal/auth/browser.go`：`isLocalEnvironment()` 函数（曾在 macOS 上无条件返回 true 导致 SSH 远程 Darwin bug）
- `cmd/auth_callback.go`：整个文件

#### 修复的 bug

- **Issue #95**：飞书错误码 20029（重定向 URL 有误）。根因是 Authorization Code Flow 需要用户在飞书开放平台配置 `http://127.0.0.1:9768/callback` 白名单，Device Flow 直接绕过此要求
- **Darwin SSH bug**：`isLocalEnvironment()` 在 macOS 上无条件返回 `true`，SSH 到 Mac 服务器时错误走本地回调模式会 2 分钟超时失败。已通过删除该函数消除

### 新增 — `auth login` 的 JSON 事件流模式

- **`auth login --json`**：阻塞轮询 + JSON 事件流输出到 stdout。AI Agent 推荐配合 Claude Code 的 `run_in_background=true` 使用
  - 首次输出：`{"event":"device_authorization","verification_uri":"...","verification_uri_complete":"...","user_code":"...","device_code":"...","expires_in":240,"interval":5}`
  - 成功输出：`{"event":"authorization_success","expires_at":"...","refresh_expires_at":"...","scope":"..."}`

- **`auth login --no-wait --json`**：两步模式第一步。只请求 `device_code` 并立即输出 JSON，不启动轮询。适合 AI Agent 希望把"请求"和"轮询"拆到两次独立 Bash 调用的场景

- **`auth login --device-code <code> --json`**：两步模式第二步。用已有的 `device_code` 继续轮询直到授权完成

### 新增 — `auth check` 子命令

预检当前 Token 是否包含指定 scope，专为 AI Agent 在执行业务命令前做前置判断而设计：

```bash
feishu-cli auth check --scope "search:docs:read"
feishu-cli auth check --scope "search:docs:read im:message:readonly"
```

输出 JSON：

```json
{
  "ok": true,
  "granted": ["search:docs:read"],
  "missing": null
}
```

或失败情况：

```json
{
  "ok": false,
  "error": "not_logged_in",
  "missing": ["search:docs:read"],
  "suggestion": "feishu-cli auth login"
}
```

退出码 0 = 满足，非 0 = 缺少或未登录，AI Agent 可直接分支。

### 不变

- **Token 存储格式**：`~/.feishu-cli/token.json` 仍是明文 JSON，数据结构完全兼容。升级后**不需要重新登录**
- **Token 自动刷新**：`ResolveUserAccessToken()` 路径和 `RefreshAccessToken()` 逻辑保持不动，access_token 过期时用 refresh_token 自动刷新
- **`config create-app`** 命令完全不变（它本来就用 Device Flow）
- **`auth status`** / **`auth logout`** 行为不变
- **所有业务命令**（doc/msg/search/wiki/task/calendar/...）行为不变

### 迁移指引

#### 人类用户

无需任何迁移。一条命令通吃所有场景：

```bash
feishu-cli auth login
```

本地桌面会自动开浏览器，SSH 远程需要手动复制 stderr 里的链接在本机浏览器打开，一模一样的命令。

#### AI Agent / 脚本用户

旧的两步式：
```bash
feishu-cli auth login --print-url --scopes "..."
feishu-cli auth callback "<回调URL>" --state "<state>"
```

迁移为以下**任一**方案：

**方案 A（推荐）**：阻塞 + 后台运行：
```bash
# run_in_background=true
feishu-cli auth login --json
# 读 stdout 第一行拿 verification_uri_complete，展示给用户
# 等后台进程退出，读第二行 stdout 拿 authorization_success
```

**方案 B**：两步模式：
```bash
# 第一步
feishu-cli auth login --no-wait --json  # → device_code JSON
# 把链接展示给用户等待授权
# 第二步
feishu-cli auth login --device-code <code> --json  # → authorization_success
```

#### CI / 无头脚本

**Authorization Code Flow 本来就无法无头完成**（需要浏览器授权），Device Flow 同样需要人类介入一次。如果 CI 需要 User Token，应该预先在本地通过 `auth login` 拿到 token.json 然后把它作为 secret 部署到 CI 环境，**不需要任何迁移**。

### 详细对比

| 方面 | v1.17.0 及以前 | v1.18.0 |
|---|---|---|
| OAuth Flow | Authorization Code Flow（默认）+ Device Flow（`--method device`） | 仅 Device Flow |
| 子命令 | `login` / `callback` / `status` / `logout` | `login` / `check` / `status` / `logout` |
| `auth login` 的 flag | `--port` / `--manual` / `--no-manual` / `--print-url` / `--scopes` / `--method` | `--json` / `--no-wait` / `--device-code` |
| 重定向 URL 白名单 | 必须（Authorization Code Flow 前置条件） | 不需要 |
| SSH 远程支持 | 要么手动粘贴（`--manual`）要么非交互两步（`--print-url`） | 一条命令通吃 |
| AI Agent 非交互方案 | `--print-url` + `auth callback` | `--json` + `run_in_background` 或 `--no-wait` / `--device-code` |
| `offline_access` 注入 | 用户手动通过 `--scopes` 传 | CLI 强制注入，用户无需操心 |
| scope 预检 | 手动解析 `auth status` JSON 的 scope 字段 | `auth check --scope "..."` |

---

更早的版本请参考 [GitHub Releases](https://github.com/riba2534/feishu-cli/releases)。
