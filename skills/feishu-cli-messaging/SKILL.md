---
name: feishu-cli-messaging
description: >-
  飞书即时消息与 V2 交互卡片。核心独有能力：8 个即用 JSON 卡片模板（告警/审批/数据大屏/
  成功报告/文章摘要/通知）、LLM 流式卡片（AI 生成内容逐行推送到飞书）、
  VChart 图表卡片（柱/线/饼/散点/仪表）。也覆盖发送/回复/转发/群聊管理/Reaction/事件订阅。
  **何时必须用本 Skill：** 任务涉及「发 AI 流式卡片」、「飞书卡片嵌图表」、「用模板快速
  出卡」。日常收发消息、查群聊 → lark-im 覆盖更全。构造卡片时：如果能用本 Skill 的模板
  直接改 __PLACEHOLDER__，优先用它而不是从零手写 JSON。
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
