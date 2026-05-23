---
name: feishu-cli-auth
description: >-
  飞书 OAuth 认证和 User Access Token 管理（Device Flow，RFC 8628）。
  支持一键创建飞书应用、按域申请推荐权限、auth check 预检 scope、auth login 登录、Token 自动刷新。
  当用户请求登录飞书、获取 Token、OAuth 授权、权限缺失、Token 过期、create-app、99991672、99991679，
  或其他飞书技能遇到 User Access Token 问题时使用。
  本技能同时承载两个相关子命令：doctor 做配置/网络/代理体检（错误信息不指向 scope/token 时用），
  profile 管理多 App / 多账号独立配置（多租户切换）。
  注意：profile 指 CLI 配置切换，与邮箱 mailbox profile（feishu-cli-mail）无关。
argument-hint: login | status | check | logout | doctor | profile <subcmd> | config create-app
user-invocable: true
allowed-tools: Bash(feishu-cli auth:*), Bash(feishu-cli config:*), Bash(feishu-cli doctor:*), Bash(feishu-cli profile:*), Read
---

# 飞书认证

本项目默认使用 App Token。只有搜索、消息历史/互动、审批任务、会议/妙记、邮箱等用户身份场景需要 User Access Token。

## 推荐流程

```bash
# 预检 scope
feishu-cli auth check --scope "search:docs:read search:message"

# 按业务域登录，自动带推荐 scope
feishu-cli auth login --domain search --recommend

# AI Agent 后台模式：输出 JSON 事件流，展示 verification_uri_complete 给用户
feishu-cli auth login --scope "search:docs:read search:message" --json
```

如果 `auth check` 返回 `missing`，先到飞书开放平台开通缺失 scope，再重新 `auth login`。

## 常用命令

```bash
feishu-cli auth status
feishu-cli auth status -o json --verify
feishu-cli auth check --scope "REQ_SCOPES"
feishu-cli auth login --domain <domain> --recommend
feishu-cli auth logout
feishu-cli config create-app --save
feishu-cli doctor --json
feishu-cli profile current
```

`auth status -o json` 未登录时会包含：

```json
{"logged_in": false, "identity": "bot", "note": "未登录，当前将使用 App Token"}
```

AI Agent 判断是否满足某个任务，优先用 `auth check`，不要只看 `auth status`。

## Token 解析策略

CLI 把命令分成三类，对应 `cmd/utils.go` 里三个 helper：

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
| `msg search-chats` | `im:chat:readonly` |
| `msg flag create/cancel/list` | `im:feed.flag:read/write` |
| `chat get/update/delete`、`chat member list/add/remove` | `im:chat:*`、`im:chat.members:*` |
| `approval task query/approve/reject/transfer` + `instance get/cancel/cc` | `approval:task` / `approval:instance:*` |
| `task my` (`my_tasks`) | `task:task:read` |
| `vc search/notes/recording`、`minutes *` | `vc:*`、`minutes:*` 相关 scope |
| `mail *` | `mail:user_mailbox:*` |
| `drive upload/download/export/import/move/add-comment/task-result/search` | `drive:drive`、`drive:file:*`、`search:docs:read` |
| `calendar rsvp` | `calendar:calendar.event:reply` |
| `markdown create/fetch/overwrite` | `docx:document` |

`chat create` 和 `chat link` **不在此表**：默认走 App Token，仅显式 `--user-access-token` 时切到 User 身份创建群/获取链接。

登录时 CLI 会自动注入 `offline_access` 和 `auth:user.id:read`。如果 `refresh_token_present=false`，通常是应用未开通 `offline_access`，需要开通后重新登录。

## 业务域登录

```bash
feishu-cli auth login --domain search --recommend
feishu-cli auth login --domain chat --recommend
feishu-cli auth login --domain vc --recommend
feishu-cli auth login --domain mail --recommend
```

多域可以显式传 scope；最终以 `auth check --scope` 结果为准。

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
```

`doctor` 共 6 项检查；`--only` 仅接受其中之一（typo 会被服务端拒收）：

| 检查名 | 含义 |
|---|---|
| `config_file` | 配置文件存在性、字段完整性 |
| `user_token` | `~/.feishu-cli/token.json` 是否存在 / 过期 / refresh 可用 |
| `endpoint_open` | `open.feishu.cn` 可达性 |
| `endpoint_larksuite` | `open.larksuite.com` 可达性（海外站） |
| `proxy` | `HTTPS_PROXY` / `NO_PROXY` 是否会拦截 OpenAPI 域名 |
| `dependencies` | 二进制依赖（如 jq、git）状态 |

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
2. 登录用 `--json` 事件流，把 `verification_uri_complete` 给用户。
3. 不把 `user_access_token` 写入文档、代码或日志。
4. 错误信息明确指向 scope/token 时直接 `auth check`；只有错误不明确（"突然不工作"/网络异常）才用 `doctor` 缩小问题面，不要混用。
