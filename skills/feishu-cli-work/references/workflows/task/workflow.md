# 任务与任务清单工作流

## Task

```bash
feishu-cli task create --summary "完成项目文档"
feishu-cli task get <task_guid>
feishu-cli task list
feishu-cli task search --uncompleted --assignee ou_xxx --due-before "2026-12-31"
feishu-cli task my --uncompleted
feishu-cli task update <task_guid> --summary "新标题"
feishu-cli task complete <task_guid>
feishu-cli task reopen <task_guid>
feishu-cli task subtask create <task_guid> --summary "子任务"
feishu-cli task member add <task_guid> --members ou_xxx --role assignee
feishu-cli task reminder add <task_guid> --minutes 30
feishu-cli task comment add <task_guid> --content "进展说明"
feishu-cli task upload-attachment --task-guid <task_guid> --file ./report.pdf
feishu-cli task set-ancestor <child_guid> --ancestor-id <parent_guid>   # 不传 --ancestor-id 则解除父任务
feishu-cli task related --include-completed=false --page-all             # 与我相关的任务（User Token）
feishu-cli task section list [--resource-type tasklist --resource-id <tasklist_guid>]
feishu-cli task section tasks <section_guid> --uncompleted
```

任务 ID 参数也接受任务 applink（`...?guid=<task_guid>`），界面上的任务编号（如 `t123456`）不是 GUID。
`task get` 输出成员、父任务与**提醒 ID**（`task reminder remove --ids` 需要）；全天截止日（`due_is_all_day=true`）
按服务端的 UTC 零点取日期，输出 `YYYY-MM-DD`，不会因本地时区差一天。

`task related` 的 `--page-token` 是任务 `updated_at` 的**微秒**时间戳游标（不是任务 ID），且列表是稀疏分页：
首页可能 0 条但 `has_more=true`，需要完整结果时用 `--page-all`（`--page-limit` 最大 40）。

`task my` 和 `task search` 必须使用 User Token。task get/list、子任务和评论读取优先 User Token 并可回落 App Token。
创建、修改、删除、完成/重开、子任务创建、成员、提醒、评论写入、附件上传以及清单写操作支持 `--as bot|user|auto`，
**默认 auto**：已登录即以本人身份操作（**行为变更**：此前默认 Bot，纯 Bot 读写用户的任务实测报 1470403）；
未配置 User Token 时回落 Bot，已配置但刷新失败 fail-closed。无人值守或操作 Bot 自建任务时显式 `--as bot`。

`task my` 默认返回**全部**任务（含已完成，与官方一致）；只看未完成用 `--uncompleted`，只看已完成用 `--completed`，两者互斥。
`task complete` 幂等：已完成的任务不会被重复改写完成时间（JSON 带 `already_completed=true`）。

`task search` 按创建人 / 负责人 / 关注人 / 完成状态 / 截止时间在服务端过滤，`--keyword` 为服务端关键词检索；
命中项会自动并发拉取详情。至少提供一个搜索条件。服务端限制：`--page-size` 最大 30，翻页 offset 上限 150
（`--page-all` 越过后优雅停止并提示缩小范围）。详见 `references/commands.md`。

## Tasklist

```bash
feishu-cli tasklist create --name "项目清单"
feishu-cli tasklist get <tasklist_guid>
feishu-cli tasklist list
feishu-cli tasklist tasks <tasklist_guid>
feishu-cli tasklist task-add <tasklist_guid> --task-ids <task_guid>[,<task_guid>]
feishu-cli tasklist task-remove <tasklist_guid> --task-ids <task_guid>
feishu-cli tasklist delete <tasklist_guid>
feishu-cli tasklist search --query "Sprint" [--creator ou_xxx]
```

命令名是 `task-add` 和 `task-remove`。`tasklist member` 仅支持 `add` / `remove`，没有
`list` 子命令。执行删除、移除成员或移除任务前确认 GUID。

完整参数表读取 `references/commands.md`，但以当前编译二进制的 `--help` 为最终依据。
