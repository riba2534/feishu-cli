---
name: feishu-cli-messaging
description: >-
  飞书即时消息统一入口，覆盖发送、回复、转发、合并转发、加急和资源下载，读取聊天历史、
  Reaction、Pin 和群成员管理，构造 V2 交互卡片，以及 WebSocket 事件订阅。用户要求发消息或通知、
  查看或导出群聊、管理群成员、制作告警/审批/报告/dashboard/带按钮或图表的卡片、监听消息或审批
  实时事件、处理消息附件时必须使用本 Skill。不要用于邮件读取/回复/草稿，也不要用于会议录制、
  妙记或逐字稿；这些分别使用 feishu-cli-mail 和 feishu-cli-meetings。全局消息搜索使用
  feishu-cli-platform。
argument-hint: <msg|chat|card|event> [args]
user-invocable: true
allowed-tools: Bash(feishu-cli:*), Bash(jq:*), Bash(python3:*), Read, Write
---

# 飞书即时消息

加载工作流后，将其中 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 路由

| 意图 | 读取文件 |
|---|---|
| 发送、回复、转发、加急、flag、资源下载 | `references/workflows/msg/workflow.md` |
| 历史消息、详情、Reaction、Pin、撤回、群和成员 | `references/workflows/chat/workflow.md` |
| 设计和生成 V2 interactive 卡片 JSON | `references/workflows/card/workflow.md` |
| 订阅和消费实时事件 | `references/workflows/event/workflow.md` |

发送交互卡片时先读取 card 构造 JSON，再读取 msg 发送。搜索历史消息关键词时读取
`../feishu-cli-platform/references/workflows/search/workflow.md`。

## 执行规则

1. 发送前确认接收者类型和 ID；不要把 email、open_id、chat_id 混用。
2. 群发、加急和删除消息有外部影响，先展示目标和数量。
3. 上传图片失败可能降级为警告并继续发送其他内容；以命令实际输出判断部分成功，不宣称原子失败。
4. 外部群 232033 的排错读取 `references/workflows/chat/references/external-chat.md`。
