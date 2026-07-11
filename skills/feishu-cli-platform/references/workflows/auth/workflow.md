# 飞书认证

本项目默认使用 App Token。只有搜索、消息历史/互动、审批任务、会议/妙记、邮箱等用户身份场景需要 User Access Token。

## 目录

- [推荐流程](#推荐流程人工--交互终端)
- [AI Agent 两步授权](#ai-agent-授权两步模式-)
- [授权结果判读](#授权结果判读)
- [常用命令](#常用命令)
- [Token 解析策略](#token-解析策略)
- [业务域登录](#业务域登录)
- [排错与 doctor](#排错)
- [多 App Profile](#多-app-profileprofile)

## 推荐流程（人工 / 交互终端）

```bash
# 1. 预检：缺什么 scope 一目了然（auth check 返回 missing 时先到开放平台开通）
feishu-cli auth check --scope "search:docs:read search:message"

# 2a. 按业务域登录，自动带该域推荐 scope
feishu-cli auth login --domain search --recommend

# 2b. 一次授全：对全部业务域申请推荐 scope（单独用 --recommend，不带 --domain）
feishu-cli auth login --recommend

# 2c. 精确控制：显式指定 scope（与 --domain/--recommend 互斥）
feishu-cli auth login --scope "search:docs:read search:message"
```

`--recommend` 三种用法：单独用 = 全部 20 个业务域的推荐 scope；配 `--domain X` = 仅该域推荐 scope；都不传且在交互终端下 = 弹出选择提示。**非交互环境（无 tty）必须显式指定范围**，否则报错。

> 💡 `auth login` 是**增量授权**：多次登录申请的 scope 在飞书服务端累积，补授新 scope 不会丢掉之前已授的。（本地 `token.json` 虽被新 token 覆盖，但其 `scope` 是服务端返回的累积值。）

## AI Agent 授权（两步模式 ⭐）

AI Agent 的 harness 通常**只把最终回复发给用户、且单轮有 timeout**，不适合在同一轮里阻塞等授权。用两步模式：第一步拿链接发给用户并结束本轮，用户授权后下一轮再续轮询。

```bash
# 第一步：只取 device_code + 授权链接，立即返回不轮询
feishu-cli auth login --recommend --no-wait --json
# → 输出一行 device_authorization 事件，把 verification_uri_complete 发给用户后结束本轮

# 第二步（用户回复已授权后）：用 device_code 续上轮询
feishu-cli auth login --device-code <device_code> --json
```

要点：
- **scope 自动恢复**：第二步从 device_code 缓存读回第一步申请的 scope，**不用也不能**再传 `--scope/--domain/--recommend`（重传会报错）。
- **device_code 有效期约 10 分钟**，超时需从第一步重来；每次重新跑第一步都会作废上一个链接。
- **不要用短 timeout 反复重试**第一步——每次重启都会让上一个授权链接失效。

若 harness 支持后台任务，也可一步阻塞 + 后台运行：

```bash
# run_in_background 跑：阻塞轮询最长约 10 分钟，stdout 逐行出 JSON 事件
feishu-cli auth login --recommend --json
```

阻塞模式务必 `run_in_background`，或把单命令 timeout 设到 ≥ 600s。

### JSON 事件 schema

`--json` 模式按 JSONL 逐行输出事件，Agent 解析这两个即可：

**`device_authorization`**（第一步 / 阻塞模式开头）：

```json
{"event":"device_authorization","verification_uri_complete":"https://accounts.feishu.cn/oauth/v1/device/verify?flow_id=...&user_code=XXXX-XXXX","user_code":"XXXX-XXXX","device_code":"...","expires_in":600,"interval":5,"requested_scopes":["..."]}
```

→ 把 `verification_uri_complete`（已含 user_code，可直接点开）**原样**发给用户，不要做任何 URL 编码/改写。

**`authorization_complete`**（授权成功，token 已落盘）：

```json
{"event":"authorization_complete","expires_at":"...","scope":"...","refresh_token_present":true,"granted_scopes":["..."],"missing_scopes":["..."],"requested_scopes":["..."]}
```

→ 上述 6 个字段常驻（`scope` 即落盘 `token.json` 的累积 scope 值）；`refresh_expires_at`（拿到 refresh token 时）、`warnings`/`hints`（refresh 缺失或 scope 未授予时）为条件字段，无对应情况时 key 不出现——解析勿假设必存。

## 授权结果判读

收到 `authorization_complete` 即代表**登录成功、token 已写入 `token.json`**——但还要看这几个字段：

| 字段 | 含义 | 处理 |
|---|---|---|
| `refresh_token_present: false` | 没拿到 refresh token | 应用未开通 `offline_access`，开通后 `auth logout && auth login` |
| `missing_scopes` 非空 | 部分申请的 scope 未授予（**warning，不是失败**） | 这些 scope 没在开放平台开通；其余 `granted_scopes` 照常可用 |
| `granted_scopes` | 实际拿到的 scope | 以此为准，可 `auth check` 复核 |

**补授权（增量）**：`missing_scopes` 非空时，先到飞书开放平台开通这些 scope，再照 CLI 的 hint 执行 `auth login --scope "<缺失的>"` 即可。多次 login 的 scope 在服务端累积，补授只需带缺失的那几个，**不会丢掉**之前已授的，无需重跑全量。

## 常用命令

```bash
feishu-cli auth status
feishu-cli auth status -o json --verify
feishu-cli auth check --scope "REQ_SCOPES"
feishu-cli auth login --domain <domain> --recommend
feishu-cli auth refresh
feishu-cli auth logout
feishu-cli auth token --as user|bot|auto    # v1.29+ 导出 token 给 curl/Python 用
feishu-cli config init
feishu-cli config get app_id
feishu-cli config create-app --save
feishu-cli doctor --json
feishu-cli profile current
```

### `auth token` 导出 Token 给外部工具用（v1.29+）

让 curl / Python requests / 任何 HTTP 工具复用本 CLI 的 Token 全生命周期管理
（Device Flow 登录、2 小时自动刷新、多 profile），不再各自实现 OAuth 流程。

| `--as` | 输出 | 适用场景 |
|---|---|---|
| `user` | User Access Token（`eyJhbGc...`，自动刷新） | 真人身份调 API，含 `auth login` 授权过的 scope |
| `bot` | Tenant Access Token（`t-g10...`，2h 有效） | App 身份，调 tenant scope API |
| `auto`（默认） | 优先 user，没有再回退 bot | 兼容兜底 |

```bash
# 给 curl 用
TOKEN=$(feishu-cli auth token --as user)
curl -H "Authorization: Bearer $TOKEN" \
  https://open.feishu.cn/open-apis/authen/v1/user_info

# 给 Python 用
TOKEN=$(feishu-cli auth token --as bot)
python3 -c "import requests; print(requests.get('https://open.feishu.cn/open-apis/im/v1/chats', headers={'Authorization': f'Bearer $TOKEN'}).json())"
```

> 想直接调任意 OpenAPI 而不写 curl？用 `feishu-cli api <method> <path>`（详见 `feishu-cli-platform` skill）。

### Agent 判读：auth check / auth status 的 JSON 契约

**`auth check --scope "..."`** —— 执行业务前预检某组 scope 够不够。退出码 `0`=满足、非 `0`=缺失或未登录；stdout 出 JSON：

| 字段 | 说明 |
|---|---|
| `ok` | `true` 表示所有 required scope 都已授权 |
| `granted` / `missing` | 已有 / 缺失的 scope 列表 |
| `error` | 仅失败时出现：`not_logged_in`（没登录）/ `token_expired`（access + refresh 都失效） |
| `suggestion` | `ok=false` 时给出的修复命令（已拼好 `auth login --scope`） |

```bash
# 满足才往下执行业务命令
feishu-cli auth check --scope "search:docs:read" && feishu-cli search docs --query "..."
```

**`auth status -o json`** —— 看本地 token 现状（默认不连服务端，加 `--verify` 才在线核验）。JSON 字段按字母序输出，已登录时关键字段（按实际输出顺序）：

| 字段 | 说明 |
|---|---|
| `access_token` | 脱敏后的 access token（前 6 + 末 6） |
| `access_token_valid` | access token 是否还在有效期内 |
| `expires_at` | access token 过期时间（RFC3339） |
| `health` | `healthy` / `missing_refresh_token`（没拿到 refresh，对应未开 `offline_access`）/ `needs_relogin`（refresh 也过期） |
| `identity` | `user`（User Token 可用）/ `bot`（仅剩 App Token 可用） |
| `logged_in` | 是否登录 |
| `refresh_expires_at` | refresh token 过期时间（有 refresh 时出现） |
| `refresh_token_present` | 是否拿到 refresh token |
| `refresh_token_valid` | refresh token 是否还在有效期内 |
| `scope` | 当前 token 已授权 scope 列表（空格分隔） |
| `token_status` | `valid` / `needs_refresh`（access 过期但 refresh 可用，下次调用自动刷新）/ `expired` |
| `cached_user.open_id` / `.name` | **当前登录的是谁**——需要本人 open_id（发消息给自己、查自己任务）时从这里取（仅当本地有 user cache 时出现） |
| `note` | 健康度提示文案（如 `missing_refresh_token` / `expired` 场景出现） |
| `verified` / `verify_error` | 仅 `--verify`：在线调 `user_info` 核验 token 是否仍被服务端接受 |

未登录时返回 `{"logged_in": false, "identity": "bot", "note": "..."}`。

> 判断"任务能不能干"优先用 `auth check`（按 scope 精确判定）；`auth status` 看整体健康度和当前身份。下面「排错」表的状态值即来自这两个命令（`auth check` 的 `error` / `auth status` 的 `health`）。

## Token 解析策略

CLI 把命令分成四类；另有少量只接受显式 flag 的严格命令：

### 1. 读类 · User 优先 + Tenant 兜底（`resolveOptionalUserTokenWithFallback`）

登录后自动用 token.json 里的 User Token；未登录则回落 App Token（要求 Bot 在群/有相应权限）。

优先级链：`--user-access-token` → `FEISHU_USER_ACCESS_TOKEN` → `~/.feishu-cli/token.json`（access 过期会自动刷新）→ `config.yaml` 的 `user_access_token` → App Token 兜底。

涉及命令：
- 消息读：`msg history`（container 路径）、`msg list`、`msg get`、`msg mget`、`msg thread-messages`、`msg resource-download`
- 任务读：`task get`、`task list`、`task subtask list`、`task comment list`、`tasklist get/list/tasks`
- 日历读：`calendar get/list/primary/agenda/freebusy/suggestion/room-find`、`calendar event get/list/search`、`calendar attendee list`
- 文件/文档读：`file meta/stats/list/version list/version get`、`file download`、`doc blocks`（读）、`board image/nodes/export-code/lint`
- **sheet 全家桶**（项目历史就是这种行为）：`sheet read/export/get/meta/list-sheets/find/replace/write/append/insert/delete/clear/import-md/dropdown/filter/filter-view/protect/style/merge/...`，所有 sheet 子命令都走 fallback，登录后默认 User、未登录 Tenant 兜底
- wiki 读：`wiki get/nodes/spaces/space-get/export/export-tree`、`wiki member list`
- drive：`drive pull/push/status`
- 其他：`user read`

### 2. 写类 / 默认 Bot 身份（`resolveOptionalUserToken`）

默认 App Token（Bot 身份），仅当显式传 `--user-access-token` 或 `FEISHU_USER_ACCESS_TOKEN` 时才切到 User Token，**不会自动加载 token.json**。

涉及命令：所有 `add/create/update/delete/move/copy/import/upload/send/reply/forward/merge-forward` 类、`comment reply`、`doc content-update / table 写`、`msg delete`（Bot 自撤回，传显式 User Token 给管理员撤回）等。

### 3. 必须 User Token（`resolveRequiredUserToken` / `requireUserToken`）

强制 User Token，没登录直接报错。

| 命令 | 典型 scope |
|---|---|
| `search docs / messages / apps` | `search:docs:read` / `search:message` |
| `msg pin/unpin/pins` | `im:message.pins` |
| `msg reaction add/remove/list` | `im:message.reactions` |
| `msg search-chats` | `im:chat:read` |
| `msg flag create/cancel/list` | `im:feed.flag:read/write` |
| `chat get/update/delete` | `im:chat:*` |
| `approval task query/approve/reject/transfer` + `instance get/cancel/cc` | `approval:task` / `approval:instance:*` |
| `task my` (`my_tasks`) | `task:task:read` |
| `vc search/notes/recording`、`minutes *` | `vc:*`、`minutes:*` 相关 scope |
| `mail *` | `mail:user_mailbox:*` |
| `drive upload/download/export/import/move/add-comment/task-result/search` | `drive:drive`、`drive:file:*`、`search:docs:read` |
| `calendar rsvp` | `calendar:calendar.event:reply` |
| `markdown create/fetch/overwrite/diff` | `drive:file:upload/download/content:read` 或 `drive:drive` |

`markdown create/fetch/overwrite/diff` 使用 Drive scope：创建/覆盖需要 `drive:file:upload`（或
`drive:drive`），读取/diff 需要 `drive:file:download` 或 `drive:file.content:read`（或 `drive:drive`）。

`chat create` 和 `chat link` **不在此表**：当前命令没有 User Token flag，始终使用 App Token。

### 4. 身份可选 · `--as bot|user|auto`（`resolveIdentityToken`）

`bitable` 全家桶使用这一模式。`auto` 默认优先 User Token、缺失时回落 Tenant Token；`bot` 强制
Tenant Token；`user` 强制 User Token，缺失即报错。`chat member` 和 `msg history` 也提供同名
身份选择，但使用各自的解析函数，边界以命令帮助为准。

### 严格 flag-only

`vc bot meeting-join` / `meeting-leave` 只认命令行显式 `--user-access-token`；不会读取
`FEISHU_USER_ACCESS_TOKEN`，也不会自动加载 `token.json`。不传 flag 时固定使用 Bot 身份。

登录时 CLI 会自动注入 `offline_access` 和 `auth:user.id:read`。如果 `refresh_token_present=false`，通常是应用未开通 `offline_access`，需要开通后重新登录。

## 业务域登录

`--domain` 可重复或逗号分隔。可选域：`approval attendance bitable calendar chat contact doc_access docs drive event im mail minutes search sheets slides task vc whiteboard wiki`，或 `all`。

```bash
feishu-cli auth login --domain search --recommend                # 单域
feishu-cli auth login --domain vc --domain minutes --recommend   # 多域
feishu-cli auth login --recommend                                # 全部域（等价 --domain all --recommend）
```

`--scope` 与 `--domain/--recommend` 互斥；最终以 `auth check --scope` 结果为准。

## 排错

| 现象 | 处理 |
|---|---|
| `not_logged_in` | 执行 `auth login --scope "..." --json` |
| `token_expired` 且 refresh 可用 | 业务命令会自动刷新；也可重新登录 |
| `needs_relogin` | refresh token 已过期，重新登录 |
| `missing_refresh_token` | 开通 `offline_access` 后 `auth logout && auth login` |
| `99991672` | access token 无效或过期，重新登录 |
| `99991679` | 应用未开通 scope，先在开放平台开通权限 |

## 环境诊断（doctor）

用户报“feishu-cli 不工作”“突然连不上”“配置有问题”时，先用 doctor 缩小问题面：

```bash
feishu-cli doctor
feishu-cli doctor --json
feishu-cli doctor --offline
feishu-cli doctor --only user_token
feishu-cli doctor --only proxy
feishu-cli doctor --only user_token,endpoint_open    # 多值，逗号分隔
```

`doctor` 共 6 项检查；`--only` 支持单个值或逗号分隔多个值（typo 会被本地校验拒收）：

| 检查名 | 含义 |
|---|---|
| `config_file` | 配置文件存在性、字段完整性 |
| `user_token` | `~/.feishu-cli/token.json` 是否存在 / 过期 / refresh 可用 |
| `endpoint_open` | `open.feishu.cn` 可达性 |
| `endpoint_larksuite` | `open.larksuite.com` 可达性（海外站） |
| `proxy` | `HTTPS_PROXY` / `NO_PROXY` 是否会拦截 OpenAPI 域名 |
| `dependencies` | 当前 Go 版本与编译依赖中的 Lark SDK 版本 |

每项状态为 `pass` / `warn` / `fail` / `skip`。整体退出码：全部 `pass`/`skip` 或仅 `warn` → `0`；任一 `fail` → `1`。

`--json` 输出 schema：

```json
{
  "ok": true,
  "checks": [
    {"name": "user_token", "status": "pass", "message": "...", "hint": "..."}
  ]
}
```

CI 用法：`feishu-cli doctor --offline --json | jq -e '.ok == true'`。

常见命中：`proxy` 检查发现 `HTTPS_PROXY` 拦截 → 把 `.feishu.cn,.larkoffice.com,.larksuite.com` 加入 `NO_PROXY`；`HTTPS_PROXY` 中的 userinfo（`user:pass@host`）会被 redact 后再上报，无需担心日志泄漏。

已经明确是 scope 缺失时，不需要 doctor，直接 `auth check --scope "..."`。

> **v1.27.1 新增**：使用 `--config <path>` 显式覆盖配置时，CLI 会在 stderr 打印 warning，提示 profile 系统被绕过（`token.json` 自动加载等行为失效）。AI Agent 调试时若看到该 warning，意味着当前命令未走 profile 体系，token 解析仅依赖该 `--config` 文件中的字段。

## 多 App Profile（profile）

用户需要在多个飞书租户、多个 App ID 或工作/个人账号之间切换时，用 profile 管理独立的 `config.yaml` 和 `token.json`：

```bash
feishu-cli profile add work --app-id cli_xxx --app-secret secret_xxx --use
feishu-cli profile list --json
feishu-cli profile current
feishu-cli profile use work
feishu-cli profile use -                    # toggle 到上一个 profile（previous-profile 指针）
feishu-cli profile rename old-name new-name
feishu-cli profile remove old-name
feishu-cli profile migrate --name work
```

解析优先级：

1. `FEISHU_PROFILE=<name>` 环境变量强制覆盖。
2. `~/.feishu-cli/active-profile` 指针。
3. 指针缺失或失效时，回退到 `profiles/` 下字典序第一个（可能不是预期的那个）。
4. 未启用 profile 系统时，继续使用旧布局 `~/.feishu-cli/config.yaml` 和 `~/.feishu-cli/token.json`。

踩坑（违反直觉、可能丢数据，逐条留意）：

1. **`profile add` 不会自动迁移**旧配置；已有配置接入 profile 系统必须显式运行 `profile migrate`，避免静默改变当前环境。
2. **`profile migrate` 不可逆且不删旧文件**：`~/.feishu-cli/config.yaml` / `token.json` 仍留在原位，需要时手动清理；不要把 migrate 当成"备份+迁移"用。
3. **`profile use <name>` 不会自动登录**：切到一个新建的 profile 后，必须再 `auth login` 才有 User Token。
4. **进程内锁仅 `sync.Mutex`，不跨进程**：并发 `profile use` / `profile rename` 在不同 shell 里同时跑会有 race。
5. **`profile use -` 在没有上一个 profile 时直接报错**：CLI 不会自动回退到字典序首位；先 `profile list` 确认目标，再 `profile use <name>` 显式切换。

profile 名校验规则 `[A-Za-z0-9_-]{1,64}`（禁止 `.` / `..` / `profiles` / `cache` 等保留名），违反时 CLI 自身会报错。`profile rename` 会自动同步 active 与 previous 指针，不需要手动改文件。

`auth logout` 会清理当前 profile 的 token 和用户 profile 缓存。

## Agent 约定

1. 执行业务前先 `auth check --scope`，缺什么报什么。
2. 登录优先用**两步模式**（`--no-wait --json` 拿链接 → 用户授权后 `--device-code --json` 续轮询）；能开后台任务时也可 `--json` 阻塞 + `run_in_background`。把 `verification_uri_complete` 原样发给用户，不改写 URL。
3. 授权成功后读 `authorization_complete` 的 `missing_scopes`：非空只是 warning，开通后按需补授（增量授权，补缺失的几个即可，不会丢已授 scope）。
4. 不把 `user_access_token` / `device_code` 写入文档、代码或日志。
5. 错误信息明确指向 scope/token 时直接 `auth check`；只有错误不明确（"突然不工作"/网络异常）才用 `doctor` 缩小问题面，不要混用。
