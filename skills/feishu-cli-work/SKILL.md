---
name: feishu-cli-work
description: >-
  仅用于日历、任务、审批、考勤和 OKR，不是所有飞书办公请求的通用入口。用户要求 freebusy、
  找共同空闲时间、预订会议室、接受/拒绝邀请、创建或回复日程，管理任务/清单，发起、撤回、
  通过、拒绝、转交或抄送审批，查询打卡/迟到/请假统计，查询 OKR 周期或更新进展时必须使用本 Skill。
  明确禁止用于用户/部门通讯录、邮箱、历史会议检索、会议录制或妙记；它们分别使用
  feishu-cli-platform、feishu-cli-mail 和 feishu-cli-meetings。会议通知消息使用
  feishu-cli-messaging。
argument-hint: <calendar|task|tasklist|approval|attendance|okr> [args]
user-invocable: true
allowed-tools: Bash(feishu-cli:*), Bash(jq:*), Read, Write
---

# 飞书工作管理

加载工作流后，将其中 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 路由

| 意图 | 读取文件 |
|---|---|
| 日历 CRUD、agenda、忙闲、智能时段、会议室、RSVP | `references/workflows/calendar/workflow.md` |
| 任务、子任务、成员、提醒、评论、附件、任务清单 | `references/workflows/task/workflow.md` |
| 审批定义、实例、任务、抄送和审批动作 | `references/workflows/approval/workflow.md` |
| 打卡任务和考勤统计 | `references/workflows/attendance/workflow.md` |
| OKR 周期、进展记录、创建 O/KR 与量化指标（api 透传） | `references/workflows/okr/workflow.md` |

## 执行规则

1. 创建、修改、删除、审批通过/拒绝等操作会影响他人，执行前展示目标和关键参数。
2. `task my`、审批任务动作和 `calendar rsvp` 必须使用 User Token；其他命令按各工作流说明选择身份。
3. 时间统一使用带时区的 RFC3339；不要把全天事件和具体时段混用。
4. 任务清单添加/移除任务使用 `task-add` / `task-remove`，不存在 `add-task` / `remove-task`。
