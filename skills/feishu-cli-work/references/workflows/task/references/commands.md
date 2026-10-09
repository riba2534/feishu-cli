# 任务管理命令参考

飞书任务 V2 API。任务 ID 为 GUID（如 `d300a75f-c56a-4be9-80d6-e47653f6xxxx`）；只列非显然的参数与约束，
完整参数以 `feishu-cli task <cmd> --help` / `feishu-cli tasklist <cmd> --help` 为准。

## 任务 CRUD

### 创建任务

```bash
feishu-cli task create \
  --summary "任务标题" \
  [--description "详细描述"] \
  [--due "2026-02-01 18:00:00"] \
  [--origin-href "https://example.com"] \
  [--origin-platform "feishu-cli"] \
  [-o json]
```

`--due` 格式 `YYYY-MM-DD HH:mm:ss` 或 `YYYY-MM-DD`（纯日期 = 当天 00:00，不是全天截止）。

### 列出 / 查看

```bash
feishu-cli task list [--completed | --uncompleted] [--page-size 50] [--page-token <token>]   # 过滤 flag 互斥
feishu-cli task get <task_guid_or_applink> [-o json]
feishu-cli task my [--completed | --uncompleted] [--page-size 50] [-o json]                 # 必需 User Token
```

`task get` 输出成员、父任务与提醒（`reminders[].id` / `relative_fire_minute`）；JSON 的 `due_time` 普通截止为
`YYYY-MM-DD HH:mm:ss`，全天截止为 `YYYY-MM-DD` 并带 `due_is_all_day=true`。列表 JSON 结构为 `{tasks, has_more, page_token}`。

### 搜索任务

```bash
feishu-cli task search \
  [--keyword "评审"] \
  [--creator ou_xxx] [--assignee ou_xxx] [--follower ou_xxx] \
  [--completed | --uncompleted] \
  [--due-after "2026-01-01"] [--due-before "2026-12-31"] \
  [--page-size 20] [--page-token <token>] [--page-all] \
  [--enrich=false] \
  [-o json]
```

- 底层 `POST /open-apis/task/v2/tasks/search`，**需 User Token**（`task:task:read`）。
- 创建人 / 负责人 / 关注人 / 完成状态 / 截止时间在**服务端**过滤；`--keyword` 是服务端关键词检索（匹配标题等）。
  以上条件至少提供一个。`--creator/--assignee/--follower` 传 `open_id`，多个用逗号分隔。
- `--due-*` 接受 RFC3339 / `2026-01-02 15:04:05` / `2026-01-02`；`--due-before` 传纯日期时自动对齐到当天 23:59:59。
- 搜索接口只返回任务 GUID，默认逐条补全详情（5 并发）；脚本只要 GUID/链接时加 `--enrich=false`。
- 服务端限制：`--page-size` 最大 30（超出自动截断）；翻页 offset 上限 150，`--page-all` 越过后停止并提示缩小范围。

### 与我相关的任务

```bash
feishu-cli task related [--include-completed=false] [--page-all] [--page-limit 20] [--page-token <微秒时间戳>] [-o json]
```

必需 User Token。稀疏分页（首页可能 0 条但 `has_more=true`），以 `has_more` 为准；`--page-limit` 范围 1-40。

### 更新 / 完成 / 重开 / 删除

```bash
feishu-cli task update <task_guid> [--summary "新标题"] [--description "新描述"] [--due "2026-03-01 18:00:00"] [--completed]
feishu-cli task complete <task_guid> [-o json]   # 幂等，已完成时 already_completed=true
feishu-cli task reopen <task_guid>
feishu-cli task delete <task_guid>
```

## 子任务与父任务

```bash
feishu-cli task subtask create <task_guid> --summary "子任务标题" [-o json]
feishu-cli task subtask list <task_guid> [--page-size 20] [--page-token <token>] [-o json]
feishu-cli task set-ancestor <task_guid> [--ancestor-id <parent_guid>]   # 不传 --ancestor-id 解除父任务
```

`set-ancestor` 把已有任务挂到另一个任务下（与 `subtask create` 新建子任务不同）。

## 成员、提醒、评论、附件

```bash
feishu-cli task member add <task_guid> --members ou_xxx,ou_yyy [--role assignee|follower]   # 默认 assignee
feishu-cli task member remove <task_guid> --members ou_xxx [--role assignee|follower]
feishu-cli task reminder add <task_guid> --minutes 30        # 相对截止时间提前的分钟数，0 = 截止时提醒
feishu-cli task reminder remove <task_guid> --ids <reminder_id>[,<reminder_id>]   # ID 从 task get 获取
feishu-cli task comment add <task_guid> --content "进展说明" [--reply-to <comment_id>]
feishu-cli task comment list <task_guid> [-o json]
feishu-cli task upload-attachment --task-guid <task_guid> --file ./report.pdf [--resource-type task|task_delivery]
```

附件单文件 ≤ 50MB（`task:attachment:write`）。

## 分组（section）

```bash
feishu-cli task section list [--resource-type my_tasks|tasklist] [--resource-id <tasklist_guid>] [--page-all] [-o json]
feishu-cli task section tasks <section_guid> [--completed | --uncompleted] [--page-all] [-o json]
```

`--resource-type` 默认 `my_tasks`（"我的任务"里的分组，需 User Token）；`tasklist` 时 `--resource-id` 必填。

## 任务清单

任务清单是任务的分组容器，一个任务可以属于多个清单。

```bash
feishu-cli tasklist create --name "Sprint 计划" [-o json]
feishu-cli tasklist list [--page-size 100] [--page-token <token>] [-o json]
feishu-cli tasklist get <tasklist_guid> [-o json]
feishu-cli tasklist search [--query "Sprint"] [--creator ou_xxx] [-o json]     # 至少一个条件
feishu-cli tasklist tasks <tasklist_guid> [--completed] [-o json]
feishu-cli tasklist task-add <tasklist_guid> --task-ids <task_guid>[,<task_guid>]
feishu-cli tasklist task-remove <tasklist_guid> --task-ids <task_guid>
feishu-cli tasklist member add <tasklist_guid> --members ou_xxx [--role editor|viewer]   # 默认 editor
feishu-cli tasklist member remove <tasklist_guid> --members ou_xxx [--role editor|viewer]
feishu-cli tasklist delete <tasklist_guid>
```

## 权限要求

任一即可，完整列表用 `feishu-cli schema task.<resource>.<method> --format json` 查看。

| 操作 | scope |
|------|------|
| 读取任务（`task my/search/related` 需用户授权） | `task:task:read`、`task:task:write` |
| 创建/修改/删除任务、子任务 | `task:task:write`、`task:task:writeonly` |
| 任务成员 | `task:task:write`、`task:personnel:writeonly` |
| 读取 / 管理任务清单 | `task:tasklist:read` / `task:tasklist:write` |
| 分组（section） | `task:section:read`、`task:section:write` |
| 上传附件 | `task:attachment:write` |
