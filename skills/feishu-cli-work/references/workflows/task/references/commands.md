# 任务管理详细参考

飞书任务 V2 API。任务 ID 为 UUID 格式（如 `d300a75f-c56a-4be9-80d6-e47653f6xxxx`）。

## 任务 CRUD

### 创建任务

```bash
feishu-cli task create \
  --summary "任务标题" \
  [--description "详细描述"] \
  [--due "2024-02-01"] \
  [--origin-href "https://example.com"] \
  [--origin-platform "feishu-cli"]
```

截止时间格式：`YYYY-MM-DD HH:mm:ss` 或 `YYYY-MM-DD`

### 列出任务

```bash
feishu-cli task list [--completed | --uncompleted] [--page-size 20] [--page-token <token>]
```

### 获取任务详情

```bash
feishu-cli task get <task_id> [-o json]
```

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

- 底层调用任务搜索接口（`POST /open-apis/task/v2/tasks/search`），**需 User Token**（`task:task:read`）。
- 创建人 / 负责人 / 关注人 / 完成状态 / 截止时间在**服务端**过滤；`--keyword` 是服务端关键词检索（匹配标题等）。
- `--due-before` 传纯日期（如 `2026-12-31`）自动对齐到当天 23:59:59（含当天全部到期任务）。
- 搜索接口只返回任务 GUID，默认逐条补全详情（5 并发）；只需要 GUID/链接的脚本场景加 `--enrich=false` 跳过补全，零额外 API 往返。
- 搜索接口只返回任务 GUID，命中项会自动并发拉取详情以展示标题、截止时间等。
- `--keyword`、`--creator/--assignee/--follower`、`--completed/--uncompleted`、`--due-after/--due-before` 至少提供一个。
- `--creator/--assignee/--follower` 传 `open_id`，多个用逗号分隔。`--due-*` 接受 RFC3339 / `2026-01-02 15:04:05` / `2026-01-02`。
- 服务端限制：`--page-size` 最大 30（超出自动截断为 30）；翻页 offset 上限 150，`--page-all` 越过后会优雅停止并提示缩小范围。

### 更新任务

```bash
feishu-cli task update <task_id> \
  [--summary "新标题"] \
  [--description "新描述"] \
  [--due "2024-03-01"]
```

### 完成任务

```bash
feishu-cli task complete <task_id>
```

### 删除任务

```bash
feishu-cli task delete <task_id>
```

## 子任务管理

### 创建子任务

```bash
feishu-cli task subtask create <task_guid> --summary "子任务标题" [-o json]
```

### 列出子任务

```bash
feishu-cli task subtask list <task_guid> [--page-size 20] [--page-token <token>] [-o json]
```

## 成员管理

### 添加成员

```bash
feishu-cli task member add <task_guid> --members id1,id2 --role assignee
```

| 角色 | 说明 |
|------|------|
| `assignee` | 执行者（默认） |
| `follower` | 关注者 |

### 移除成员

```bash
feishu-cli task member remove <task_guid> --members id1,id2 --role assignee
```

## 提醒管理

### 添加提醒

```bash
feishu-cli task reminder add <task_guid> --minutes 30
```

`--minutes`：提前提醒的分钟数，`0` 表示在截止时间提醒。

### 移除提醒

```bash
feishu-cli task reminder remove <task_guid> --ids id1,id2
```

## 任务清单

任务清单是任务的分组容器，一个任务可以属于多个清单。

### 创建清单

```bash
feishu-cli tasklist create --name "Sprint 计划" [-o json]
```

### 列出清单

```bash
feishu-cli tasklist list [--page-size 20] [--page-token <token>] [-o json]
```

### 获取清单详情

```bash
feishu-cli tasklist get <tasklist_guid> [-o json]
```

### 删除清单

```bash
feishu-cli tasklist delete <tasklist_guid>
```

## 权限要求

| 权限 | 说明 |
|------|------|
| `task:task:read` | 读取任务（需单独申请） |
| `task:task:write` | 创建/修改/删除任务（需单独申请） |
| `task:tasklist:read` | 读取任务清单 |
| `task:tasklist:write` | 管理任务清单 |
