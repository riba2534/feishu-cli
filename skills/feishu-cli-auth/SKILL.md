---
name: feishu-cli-auth
description: >-
  飞书 OAuth 认证和 User Access Token 管理（Device Flow，RFC 8628）。
  支持一键创建飞书应用、按域申请推荐权限、auth check 预检 scope、auth login 登录、
  Token 自动刷新。当用户请求登录飞书、获取 Token、OAuth 授权、权限缺失、Token 过期、
  create-app、99991672、99991679，或其他飞书技能遇到 User Access Token 问题时使用。
user-invocable: true
allowed-tools: Bash(feishu-cli auth:*), Bash(feishu-cli config:*), Read
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
```

`auth status -o json` 未登录时会包含：

```json
{"logged_in": false, "identity": "bot", "note": "未登录，当前将使用 App Token"}
```

AI Agent 判断是否满足某个任务，优先用 `auth check`，不要只看 `auth status`。

## Token 解析策略

| 场景 | 策略 |
|---|---|
| 默认命令 | App Token，不自动加载用户 token |
| 可选 User Token 命令 | 有 `--user-access-token` 或 `FEISHU_USER_ACCESS_TOKEN` 时用用户身份，否则 App Token |
| 必须 User Token 命令 | 参数 → 环境变量 → `~/.feishu-cli/token.json`（可自动刷新）→ config |

登录时 CLI 会自动注入 `offline_access` 和 `auth:user.id:read`。如果 `refresh_token_present=false`，通常是应用未开通 `offline_access`，需要开通后重新登录。

## 必须 User Token 的典型命令

| 命令 | 典型 scope |
|---|---|
| `search docs` | `search:docs:read` |
| `search messages` | `search:message` |
| `msg get/list/history/mget/thread-messages` | `im:message:readonly` + 用户消息读取 scope |
| `msg pin/unpin/pins` | `im:message.pins` |
| `msg reaction add/remove/list` | `im:message.reactions` |
| `msg search-chats`、`chat *` | `im:chat:*`、`im:chat.members:*` |
| `approval task query` | `approval:task:read` |
| `vc search/notes/recording`、`minutes *` | `vc:*`、`minutes:*` 相关 scope |
| `mail *` | `mail:user_mailbox:*` 相关 scope |
| `drive search` | `search:docs:read` |

`msg delete` 不是必须 User Token：默认 App Token 用于 Bot 自撤回；显式 User Token 可用于管理员撤回场景。

## 可选 User Token 的典型命令

```text
wiki get/export/nodes
calendar list/get/event-search/freebusy
task create/complete/list、tasklist create/list/delete
user info/search
drive pull/push/status
```

这些命令默认 App Token；只有显式传 User Token 时才切换用户身份。

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

## Agent 约定

1. 执行业务前先 `auth check --scope`，缺什么报什么。
2. 登录用 `--json` 事件流，把 `verification_uri_complete` 给用户。
3. 不把 `user_access_token` 写入文档、代码或日志。
4. `auth logout` 会清理 token 和用户 profile 缓存。
