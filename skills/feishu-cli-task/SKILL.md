---
name: feishu-cli-task
description: >-
  飞书任务：创建、查看、更新、完成、删除任务，管理子任务、成员、提醒和任务清单。
  适用于 todo、待办、项目任务和清单协作场景。
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书任务技能

这个技能对齐官方 lark-task 的入口。

## 常用命令

```bash
feishu-cli task create --summary "完成代码审查"
feishu-cli task my
feishu-cli task list
feishu-cli task get <task_id>
feishu-cli task update <task_id> --summary "新标题"
feishu-cli task complete <task_id>
feishu-cli task reopen <task_id>
feishu-cli task delete <task_id>
feishu-cli task subtask create <task_guid> --summary "子任务标题"
feishu-cli task subtask list <task_guid>
feishu-cli task member add <task_guid> --members id1,id2 --role assignee
feishu-cli task member remove <task_guid> --members id1,id2 --role follower
feishu-cli task reminder add <task_guid> --minutes 30
feishu-cli task reminder remove <task_guid> --ids rid1,rid2
feishu-cli tasklist create --summary "项目清单"
feishu-cli tasklist get <tasklist_id>
feishu-cli tasklist list
feishu-cli tasklist delete <tasklist_id>
feishu-cli tasklist member add <tasklist_id> --members id1,id2
feishu-cli tasklist member remove <tasklist_id> --members id1,id2
feishu-cli tasklist task-add <tasklist_id> --task-ids tid1,tid2
```

## 提示

- 用户说 todo / 待办 时，优先考虑 task
- 用户说“我的任务”时，优先看 `task list`
