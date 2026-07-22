# 任务与任务清单工作流

## Task

```bash
feishu-cli task create --summary "完成项目文档"
feishu-cli task get <task_guid>
feishu-cli task list
feishu-cli task search --uncompleted --assignee ou_xxx --due-before "2026-12-31"
feishu-cli task my
feishu-cli task update <task_guid> --summary "新标题"
feishu-cli task complete <task_guid>
feishu-cli task reopen <task_guid>
feishu-cli task subtask create <task_guid> --summary "子任务"
feishu-cli task member add <task_guid> --members ou_xxx --role assignee
feishu-cli task reminder add <task_guid> --minutes 30
feishu-cli task comment add <task_guid> --content "进展说明"
feishu-cli task upload-attachment <task_guid> ./report.pdf
```

`task my` 和 `task search` 必须使用 User Token。task get/list、子任务和评论读取优先 User Token并可回落 App Token；
创建、修改、删除、成员、提醒和评论写入默认 Bot 身份，显式 User Token 才切换用户身份。

`task search` 按创建人 / 负责人 / 关注人 / 完成状态 / 截止时间在服务端过滤，`--keyword` 为服务端关键词检索；
命中项会自动并发拉取详情。至少提供一个搜索条件。服务端限制：`--page-size` 最大 30，翻页 offset 上限 150
（`--page-all` 越过后优雅停止并提示缩小范围）。详见 `references/commands.md`。

## Tasklist

```bash
feishu-cli tasklist create --name "项目清单"
feishu-cli tasklist get <tasklist_guid>
feishu-cli tasklist list
feishu-cli tasklist tasks <tasklist_guid>
feishu-cli tasklist task-add <tasklist_guid> <task_guid>
feishu-cli tasklist task-remove <tasklist_guid> <task_guid>
feishu-cli tasklist delete <tasklist_guid>
```

命令名是 `task-add` 和 `task-remove`。`tasklist member` 仅支持 `add` / `remove`，没有
`list` 子命令。执行删除、移除成员或移除任务前确认 GUID。

完整参数表读取 `references/commands.md`，但以当前编译二进制的 `--help` 为最终依据。
