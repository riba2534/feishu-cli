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
  请求出现 approval、approval_code、审批定义/实例/待办时绝对不要使用本 Skill，应使用
  feishu-cli-work；出现视频会议、妙记、minute、录制或逐字稿时应使用 feishu-cli-meetings。
argument-hint: <auth|config|api|schema|search|user|dept> [args]
user-invocable: true
allowed-tools: Bash(feishu-cli:*), Bash(jq:*), Bash(curl:*), Bash(python3:*), Read, Write
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

Schema 只负责发现接口；API 负责执行请求。通常先查 schema，再调用 api。

## 执行规则

1. 优先运行仓库当前编译产物 `./feishu-cli`；安装环境才使用 PATH 中的 `feishu-cli`。
2. User Token 操作先执行 `auth check --scope`。不要在输出、文件或命令历史中暴露真实 Token。
3. 写请求先用 `--dry-run`；API 不支持 dry-run 的端点需明确告知用户副作用。
4. 搜索与通讯录结果默认只读取；用户要求后续写操作时再切换到对应领域 Skill。

## 领域边界

- 文档读写与导入导出：`feishu-cli-docs`
- 云盘、知识库、评论和权限：`feishu-cli-storage`
- 消息、群聊、卡片和事件：`feishu-cli-messaging`
- Sheet/Bitable：`feishu-cli-data`
