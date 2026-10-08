---
name: feishu-cli-platform
description: >-
  仅用于飞书 CLI 的平台基础能力，不是所有飞书请求的通用兜底。覆盖配置初始化、OAuth 登录与
  Token/Profile 管理、doctor 诊断、
  OpenAPI schema 查询、api 通用透传、全局搜索以及用户和部门查询。用户提到登录飞书、
  Device Flow、scope、User/Tenant Token、Token 过期、profile、doctor、99991672/99991679、
  查询 API path/参数/scope、raw api、调用未封装 OpenAPI、搜索文档/消息/应用或查询用户、邮箱、
  部门时必须使用本 Skill。明确禁止用于文档正文、云盘文件、消息/群聊、Sheet/Bitable、
  画板/展示、日历/任务/审批/考勤/OKR、邮箱或会议/妙记；这些业务操作交给对应领域 Skill。
  这里的“全局搜索”仅指 `search docs/messages/apps`，不包括在审批、会议、邮箱等业务域内查询。
  业务审批定义/实例/待办使用 feishu-cli-work，会议/妙记业务使用 feishu-cli-meetings。
  但明确查询 schema 或调用未封装 raw OpenAPI 时仍使用本 Skill，即使端点属于 approval/vc。
compatibility: Requires feishu-cli v1.41.0+ and network access for Feishu API calls.
allowed-tools: Bash(feishu-cli:*) Bash(./feishu-cli:*) Bash(./bin/feishu-cli:*) Bash(jq:*) Bash(curl:*) Bash(python3:*) Read Write
---

# 飞书平台能力

先判断意图，再读取对应工作流。不要一次加载全部 reference。
加载后，将工作流中的 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 路由

| 意图 | 读取文件 |
|---|---|
| 登录、登出、scope 预检、Token、profile、config、doctor | `references/workflows/auth/workflow.md` |
| 调任意 OpenAPI、`--as`、分页和 dry-run | `references/workflows/api/workflow.md` |
| 查询本地 OpenAPI path、参数和 scope | `references/workflows/schema/workflow.md` |
| 搜索文档、消息或应用 | `references/workflows/search/workflow.md` |
| 查询用户、邮箱、手机号、部门 | `references/workflows/directory/workflow.md` |

涉及身份选择时读取 `references/workflows/auth/references/identity.md`；`auth check` 不是 Bot 权限检查。

Schema 只负责发现接口；API 负责执行请求。通常先查 schema，再调用 api。

## 执行规则

1. 仓库开发优先使用刚编译的 `bin/feishu-cli`（`make build`）或 `./feishu-cli`；安装环境使用 PATH 中的 `feishu-cli`。
2. 先确定执行身份。`auth check --scope` 只检查当前 profile 的本地 User Token，不能验证 Bot 权限或显式 Token；Bot 的权限看应用配置和实际接口结果。不要回显真实 Token。
3. 多 Bot / 不确定当前应用时先 `profile list --json`；单次用 `--profile <name>`，不改默认指针。环境变量覆盖了目标 App 时，沿用已有授权在本次进程移除对应覆盖，或用 `--bot-app-id/--bot-app-secret` 指定正确凭证，不修改用户全局环境。
4. `api` 的所有写请求均可先用 CLI 本地 `--dry-run` 预览；预览不代表服务端接受。其他命令按各自帮助选择验证方式。
5. 搜索与通讯录结果默认只读取；用户要求后续写操作时再切换到对应领域 Skill。
6. 按退出码判断失败类型（所有命令通用）：`0` 成功、`1` 业务错误、`2` 用法错误（未知命令/flag、参数或本地路径校验失败）、`3` 鉴权/权限、`4` 网络（可重试）、`10` 危险操作需确认（获得用户同意后追加全局 `--yes` 重跑）、`130` 被中断。错误与诊断（log_id、所需 scope、修复建议）只写 stderr。

## 领域边界

- 文档读写与导入导出：`feishu-cli-docs`
- 云盘、知识库、评论和权限：`feishu-cli-storage`
- 消息、群聊、卡片和事件：`feishu-cli-messaging`
- Sheet/Bitable：`feishu-cli-data`
