# 任务与任务清单工作流

覆盖飞书任务 V2：任务 CRUD、完成/重开、子任务与父任务、成员、提醒、评论、附件、分组（section）、
"我的任务"/"与我相关"、任务搜索，以及任务清单（tasklist）。完整参数读取 `references/commands.md`，
以当前编译二进制的 `--help` 为最终依据。

## 决策规则

| 用户意图 | 命令 | 身份 |
|---|---|---|
| 分配给我的任务 | `task my`（默认返回全部，含已完成） | 必需 User |
| 我负责/关注/创建的全部相关任务 | `task related` | 必需 User |
| 按负责人/创建人/关注人/截止时间/关键词找任务 | `task search` | 必需 User |
| "我的任务"里的分组、清单里的分组 | `task section list` / `task section tasks` | User 优先回退 Bot；`my_tasks` 需 User |
| 找清单 | `tasklist search` / `tasklist list` | User 优先回退 Bot |
| 把任务挂到另一个任务下 / 解除 | `task set-ancestor` | `--as`，默认 auto |
| 任何写操作 | create/update/delete/complete/reopen、subtask create、member、reminder、comment add、upload-attachment、清单写操作 | `--as bot\|user\|auto`，默认 auto |

- 写命令**默认 auto**：已登录即以本人身份操作（**行为变更**：此前默认 Bot；实测纯 Bot 改用户的任务报 1470403）；
  未配置 User Token 时回落 Bot，已配置但刷新失败 fail-closed。无人值守或操作 Bot 自建的任务时显式 `--as bot`。
- `task get/list`、`subtask list`、`comment list`、`tasklist get/list/tasks/search`、`task section list/tasks` 无 `--as`：
  User Token 优先，未登录回落 App Token。

## Task

```bash
feishu-cli task create --summary "完成项目文档" --due "2026-12-31 18:00:00"
feishu-cli task get <task_guid>                     # 也接受任务 applink
feishu-cli task list [--completed | --uncompleted]
feishu-cli task my --uncompleted
feishu-cli task search --uncompleted --assignee ou_xxx --due-before "2026-12-31"
feishu-cli task related --include-completed=false --page-all
feishu-cli task update <task_guid> --summary "新标题"
feishu-cli task complete <task_guid>
feishu-cli task reopen <task_guid>
feishu-cli task delete <task_guid>
feishu-cli task subtask create <task_guid> --summary "子任务"
feishu-cli task subtask list <task_guid>
feishu-cli task set-ancestor <child_guid> --ancestor-id <parent_guid>   # 不传 --ancestor-id 则解除父任务
feishu-cli task member add <task_guid> --members ou_xxx --role assignee
feishu-cli task member remove <task_guid> --members ou_xxx --role follower
feishu-cli task reminder add <task_guid> --minutes 30
feishu-cli task reminder remove <task_guid> --ids <reminder_id>
feishu-cli task comment add <task_guid> --content "进展说明" [--reply-to <comment_id>]
feishu-cli task comment list <task_guid> [-o json]
feishu-cli task upload-attachment --task-guid <task_guid> --file ./report.pdf
feishu-cli task section list [--resource-type tasklist --resource-id <tasklist_guid>]
feishu-cli task section tasks <section_guid> --uncompleted
```

- **任务 ID**：位置参数里的任务 GUID、`--task-guid`、`--ancestor-id` 都接受任务 applink（`...?guid=<task_guid>`）；
  `tasklist task-add/task-remove --task-ids` 只接受 GUID。界面上的任务编号（如 `t123456`）不是 GUID，会报用法错误。
- **截止时间**：`--due` 接受 `YYYY-MM-DD HH:mm:ss` 或 `YYYY-MM-DD`；纯日期会设为**当天 00:00** 的具体时刻（CLI 不创建全天截止），
  "当天下班前截止"之类的需求请写明时刻。在客户端设置的全天截止（`due_is_all_day=true`）`task get` 按 UTC 零点取日期，输出 `YYYY-MM-DD`。
- `task get` 输出成员、父任务与**提醒 ID**（`task reminder remove --ids` 需要）；`--minutes 0` 表示截止时提醒。
- `task my` 默认返回**全部**任务（与官方一致）；`--completed` / `--uncompleted` 互斥（同时传 exit 2）。`task list`、`task section tasks` 同理。
- `task complete` 幂等：已完成的任务不会被重复改写完成时间（JSON 带 `already_completed=true`）。
- 删除任务、清单不需要 `--yes`，执行前自行向用户确认 GUID。

### 分页

- `task related` 是稀疏分页：实测 `--include-completed=false` 时首页 0 条但 `has_more=true`，`--page-all` 后才拿到结果。
  需要完整结果时用 `--page-all`（`--page-limit` 默认 20、最大 40）。`--page-token` 是任务 `updated_at` 的**微秒**时间戳，不是任务 ID。
- `task section list/tasks` 的 `--page-all` 上限 40 页。
- `task search`：服务端 `--page-size` 最大 30（超出自动截断）、翻页 offset 上限 150（`--page-all` 越过后停止并提示缩小范围）；
  至少提供一个搜索条件。搜索只返回 GUID，默认并发补全详情；只要 GUID/链接时加 `--enrich=false`。刚删除的任务可能仍被命中但详情为空。

## Tasklist

```bash
feishu-cli tasklist create --name "项目清单"
feishu-cli tasklist get <tasklist_guid>
feishu-cli tasklist list
feishu-cli tasklist search --query "Sprint" [--creator ou_xxx]
feishu-cli tasklist tasks <tasklist_guid> [--completed]
feishu-cli tasklist task-add <tasklist_guid> --task-ids <task_guid>[,<task_guid>]
feishu-cli tasklist task-remove <tasklist_guid> --task-ids <task_guid>
feishu-cli tasklist member add <tasklist_guid> --members ou_xxx --role editor
feishu-cli tasklist member remove <tasklist_guid> --members ou_xxx --role editor
feishu-cli tasklist delete <tasklist_guid>
```

- 命令名是 `task-add` 和 `task-remove`（不存在 `add-task` / `remove-task`）。
- `tasklist member` 仅支持 `add` / `remove`（角色 `editor` / `viewer`），没有 `list` 子命令。
- `tasklist search` 至少传 `--query` 或 `--creator` 之一。
- 实测 `tasklist list` 在部分 `--page-size`（含默认值）下稳定返回 1470500（服务端内部错误），换一个 `--page-size`（如 100）可绕过。

## 坑点与错误处理

| 现象 | 原因 | 处理 |
|---|---|---|
| `code: 1470403` Invoker is unauthorized | 以 Bot 身份读写用户的任务（Bot 不是任务成员） | 去掉 `--as bot`，用默认 auto/User 身份 |
| `"t123456" 是任务界面编号，不是任务 GUID` | 传了界面编号 | 用 `task search` 或任务 applink 拿 GUID |
| `task my` / `search` / `related` 报缺 User Token | 这些命令必需 User Token | `feishu-cli auth login` 或 `--user-access-token` |
| `--completed 与 --uncompleted 不能同时使用` | 两个过滤互斥 | 只传一个，都不传返回全部 |
