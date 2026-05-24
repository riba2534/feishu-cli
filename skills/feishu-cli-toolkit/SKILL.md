---
name: feishu-cli-toolkit
description: >-
  飞书轻量工具箱与分诊入口。仅在没有更专用 skill 时使用，主要覆盖普通电子表格、
  日历/日程、任务/任务清单、基础文件/素材/评论、知识库、用户和部门查询、审批查询。
  文档读写导入导出、云盘增强、多维表格、画板、消息/群聊、邮箱、搜索、权限、OAuth、
  视频会议/妙记均优先使用对应 feishu-cli-* 专用技能。
argument-hint: <module> <command> [args]
user-invocable: true
allowed-tools: Bash(feishu-cli auth:*), Bash(feishu-cli sheet:*), Bash(feishu-cli calendar:*), Bash(feishu-cli task:*), Bash(feishu-cli tasklist:*), Bash(feishu-cli file:*), Bash(feishu-cli media:*), Bash(feishu-cli comment:*), Bash(feishu-cli wiki:*), Bash(feishu-cli user:*), Bash(feishu-cli dept:*), Bash(feishu-cli approval:*), Read, Write
---

# 飞书轻量工具箱

本技能是 fallback，不和专用技能抢职责。先分诊，再执行对应命令。

## Token 策略（仅本 skill 涵盖范围）

| 类型 | 行为 |
|---|---|
| 读类 + sheet 全家桶（sheet 所有读写命令、calendar list/get/event-search/freebusy/attendee list、task get/list/comment list/subtask list/tasklist get/list/tasks、file meta/stats/list/version list/get、download、wiki nodes/spaces 等） | 已 `auth login` 自动用 User Token，未登录回落 App Token（`resolveOptionalUserTokenWithFallback`） |
| 写类（calendar create-event/update/delete/event-reply/attendee add、task create/update/delete/comment add/reminder add/...、tasklist create/delete/add-task/remove-task、wiki member add/remove） | 默认 App Token（Bot 身份），仅显式 `--user-access-token` 时切到 User（`resolveOptionalUserToken`） |
| 必需 User Token（approval task query/approve/reject/transfer + instance get/cancel/cc、my_tasks、calendar rsvp） | 未登录直接报错（`resolveRequiredUserToken` / `requireUserToken`） |

> 完整跨 skill 规则（含 search / chat / msg flag 等本 skill 范围外的命令）见 `feishu-cli-auth` 技能的"Token 解析策略"章节。

## 分诊

| 用户意图 | 使用技能 |
|---|---|
| 读文档 / 导出 Markdown | `feishu-cli-read` / `feishu-cli-export` |
| 创建、编辑、导入 Markdown | `feishu-cli-write` / `feishu-cli-import` |
| 大文件上传、云盘异步导入导出、富文本评论、drive search | `feishu-cli-drive` |
| 多维表格 base/v3 | `feishu-cli-bitable` |
| 画板、Mermaid/PlantUML 直接落板、SVG 管道 | `feishu-cli-board` |
| 发送消息、卡片、群聊历史 | `feishu-cli-msg` / `feishu-cli-card` / `feishu-cli-chat` |
| 邮件、会议/妙记、搜索、权限、认证 | 对应 `mail` / `vc` / `search` / `perm` / `auth` skill |

## 模块速查

| 模块 | 常用命令 | 详细参考 |
|---|---|---|
| 电子表格 Sheet | `sheet create/get/read/write/append/import-md/export`、V3 富文本 | `references/sheet-commands.md` |
| 日历日程 | `calendar list/get/primary/create-event/list-events/get-event/update-event/delete-event/event-search/freebusy` | `references/calendar-commands.md` |
| 任务 / 清单 | `task create/complete/delete`、`task subtask/member/reminder/comment`、`tasklist` | `references/task-commands.md` |
| 基础群创建 | `chat create/link` | `references/chat-commands.md` |
| 基础文件 | `file list/mkdir/move/copy/delete/download/upload/version/meta/stats` | 本文件 |
| 素材 | `media upload/download` | 本文件 |
| 评论 | `comment list/add/delete/resolve/unresolve`、`comment reply` | 本文件 |
| 知识库 | `wiki get/export/spaces/nodes/space-get/member` | 本文件 |
| 审批 | `approval get`、`approval task query` | 本文件 |
| 用户/部门 | `user info/search/list`、`dept get/children` | 本文件 |

## Sheet

普通电子表格读写和 Markdown 表格导入/导出：

```bash
feishu-cli sheet create --title "数据表"
feishu-cli sheet read <token> "A1:C10" --sheet-id <sheet_id>
feishu-cli sheet write <token> "A1:B2" --sheet-id <sheet_id> --data '[["姓名","分数"],["张三",95]]'
feishu-cli sheet import-md report.md --title "报表"
feishu-cli sheet export <token_or_url> --format markdown -o report.md
```

富文本、样式、图片、保护范围等细节见 `references/sheet-commands.md`。注意 `sheet export` 支持 `/sheets/<token>` URL。

## Calendar

```bash
feishu-cli calendar list
feishu-cli calendar primary
feishu-cli calendar create-event --calendar-id <id> --summary "会议" \
  --start "2024-01-21T14:00:00+08:00" --end "2024-01-21T15:00:00+08:00"
feishu-cli calendar list-events <calendar_id> --start-time "2024-01-01T00:00:00+08:00"
feishu-cli calendar get-event <calendar_id> <event_id>
feishu-cli calendar update-event <calendar_id> <event_id> --summary "新标题"
feishu-cli calendar delete-event <calendar_id> <event_id>
feishu-cli calendar event-search --calendar-id <id> --query "周会"
```

更多参数见 `references/calendar-commands.md`。

## Task / Tasklist

```bash
feishu-cli task create --summary "任务标题"
feishu-cli task complete <task_guid>
feishu-cli task subtask create <task_guid> --summary "子任务"
feishu-cli task member add <task_guid> --members ou_xxx --role assignee
feishu-cli tasklist create --name "项目清单"
feishu-cli tasklist tasks <tasklist_guid>
```

完整任务、成员、提醒、评论命令见 `references/task-commands.md`。

## File / Media

基础文件和素材命令适合简单 App Token 场景；大文件、resume、云盘 diff 用 `feishu-cli-drive`。

```bash
feishu-cli file list <folder_token>
feishu-cli file upload ./report.pdf --parent fldxxx
feishu-cli file download <file_token> -o ./report.pdf
feishu-cli file mkdir "新文件夹" --parent fldxxx
feishu-cli media upload image.png --parent-type docx_image --parent-node <document_id>
feishu-cli media download <file_token> -o ./image.png
```

## Comment / Wiki

```bash
feishu-cli comment list <file_token> --type docx
feishu-cli comment add <file_token> --type docx --text "评论内容"
feishu-cli comment resolve <file_token> <comment_id> --type docx

feishu-cli wiki spaces
feishu-cli wiki nodes <space_id>
feishu-cli wiki get <node_token>
feishu-cli wiki member list <space_id>

# v1.29+ 新增 ⭐
feishu-cli wiki space-create --name "新知识库" [--description "..."]      # 创建知识库
feishu-cli wiki space-list [--page-all] [--page-limit N] [-o json]         # 列出所有可见 space（默认仅第一页）
feishu-cli wiki node-copy --space-id <src> --node-token <node> \
  --target-space-id <dst> [--title "副本标题"]                              # 复制节点（也可用 --target-parent-node-token）
```

需要富文本评论、wiki URL 自动解析、局部评论时使用 `feishu-cli-drive`。

## Approval / User / Dept

```bash
feishu-cli approval get <approval_code>
feishu-cli approval task query --topic todo -o json

feishu-cli user info ou_xxx
feishu-cli user search --email user@example.com
feishu-cli user list --department-id od_xxx
feishu-cli dept get <department_id>
feishu-cli dept children <department_id>
```

`approval task query` 需要 User Token；执行前可用 `feishu-cli-auth` 做 scope 预检。

## 执行前检查

1. 不确定模块时先看上方分诊表。
2. 涉及 User Token 的命令先运行 `feishu-cli auth check --scope "<scope>"`。
3. 需要复杂参数时读取对应 `references/*.md`，不要把二级 reference 当成默认上下文。
