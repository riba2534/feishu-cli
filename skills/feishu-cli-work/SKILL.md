---
name: feishu-cli-work
description: >-
  飞书日历、任务、审批、考勤与 OKR：查忙闲与共同空闲、预订会议室、创建/修改/删除/回复日程（含参与人、重复日程范围、分享与转让组织者），管理任务、子任务、分组与清单，发起、撤回、通过、拒绝、转交、回退、加签、催办或抄送审批，查询打卡与考勤统计，查询 OKR 周期、创建或更新目标与关键结果、上报进展和评论。用户提到日程、会议室、待办任务、审批、打卡请假、OKR 时使用。不用于：通讯录查人和未封装接口的 schema/raw API（feishu-cli-platform）、邮件（feishu-cli-mail）、历史会议/录制/妙记（feishu-cli-meetings）、发送通知或监听审批实时事件（feishu-cli-messaging）。
compatibility: Requires feishu-cli v1.42.0+ and network access for Feishu API calls.
allowed-tools: Bash(feishu-cli:*) Bash(./feishu-cli:*) Bash(./bin/feishu-cli:*) Bash(jq:*) Bash(sleep:*) Read Write
---

# 飞书工作管理

加载工作流后，将其中 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 路由

| 意图 | 读取文件 |
|---|---|
| 日程查询（agenda / event-search）、日程增删改与重复日程、参与人与会议室、忙闲/智能时段/会议室查找、接受/拒绝邀请、日程分享与转让组织者 | `references/workflows/calendar/workflow.md` |
| 任务、子任务与父任务、成员、提醒、评论、附件、分组（section）、我的/相关任务、任务搜索、任务清单 | `references/workflows/task/workflow.md` |
| 审批定义、实例（发起/撤回/抄送/我发起的）、任务（待办查询、通过/拒绝/转交/退回/加签/催办） | `references/workflows/approval/workflow.md` |
| 打卡记录和考勤统计 | `references/workflows/attendance/workflow.md` |
| OKR 周期、目标/关键结果创建与更新、进展、评论；量化指标走 api 透传 | `references/workflows/okr/workflow.md` |

## 执行规则

1. 会影响他人或不可撤销的操作（邀请参与人、审批动作、OKR 写入、转让组织者、删除）执行前展示目标与关键参数，
   能 `--dry-run` 的先预览，得到用户确认再执行；非交互环境需要确认的命令不带 `--yes` 会以 exit 10 退出且不执行。
2. 身份：`task my|search|related`、**全部**审批命令、`calendar rsvp|event-reply`、`okr comment create` 必须使用 User Token；
   日历与任务的写命令 `--as` 默认 auto（已登录即本人身份，此前版本默认 Bot）；OKR 命令组默认 `--as bot`；其他按各工作流说明。
3. 重复日程的修改/删除先让用户确认范围（`--apply-to single|all|this-and-following`），不要替用户默认；`all` /
   `this-and-following` 需 `--yes`。`event-transfer` 不可撤销，重复日程还需 `--transfer-series`。
4. 看某段时间的安排用 `calendar agenda`（展开重复日程、过滤已取消），不要用 `list-events`；没有明确时间的排会先
   `calendar suggestion` 再对选定时段 `room-find`。
5. 时间格式以命令帮助为准：日历时间点优先带时区的 RFC3339，CLI 不能创建全天日程（`YYYY-MM-DD` 会变成当天 00:00 的时段）；
   `suggestion` / `room-find` 只收 RFC3339；考勤日期只用 `YYYY-MM-DD` / `YYYYMMDD`；任务 `--due` 纯日期是当天 00:00。
6. 审批 `task query` / `instance initiated` 与 `task related` 是稀疏分页：空页或不足一页不代表没有数据，以 `has_more` 为准，
   需要完整结果用 `--page-all`；审批的 `count` 不是总数。
7. 任务清单添加/移除任务使用 `task-add` / `task-remove`，不存在 `add-task` / `remove-task`。

删除、覆盖类命令返回退出码 10 时，向用户确认目标与影响后追加全局 `--yes` 重跑，不要自行添加。身份或 scope 报错（如 99991663/99991668/99991672/99991679）读取 `../feishu-cli-platform/references/workflows/auth/references/identity.md`；判断成败、编写脚本或处理确认门禁读取 `../feishu-cli-platform/references/workflows/auth/references/agent-contract.md`。
