---
name: feishu-cli-messaging
description: >-
  飞书即时消息统一入口，覆盖发送、回复、转发、合并转发、加急和资源下载，读取聊天历史、
  Reaction、Pin 和群成员管理，构造 V2 交互卡片，以及 WebSocket 事件订阅。用户要求发消息或通知、
  查看或导出群聊、管理群成员、制作告警/审批/报告/dashboard/营销宣传/节日贺卡/品牌展示/
  手绘或深色创意/带按钮或图表的飞书卡片、监听消息或审批实时事件、处理消息附件时必须使用本
  Skill。只要目标载体是飞书聊天或群消息，或者请求出现 msg/chat/card/event、post/interactive、
  oc_/om_、Reaction、Pin、加急或消息内本地图片，即使用户没明确说 CLI，也必须触发本 Skill。
  不要用于邮件读取/回复/草稿，也不要用于会议录制、妙记或逐字稿；这些分别使用
  feishu-cli-mail 和 feishu-cli-meetings。全局消息搜索使用 feishu-cli-platform。
argument-hint: <msg|chat|card|event> [args]
user-invocable: true
allowed-tools: Bash(feishu-cli:*), Bash(./feishu-cli:*), Bash(jq:*), Bash(python3:*), Read, Write
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
3. `msg send` 与 `msg reply` 共用内容快捷参数；本地图片、文件、Opus 音频、MP4 视频会先上传，
   任一文件缺失、格式非法或上传失败都会阻止提交消息。`--upload-images` 同时适用于两条命令。
4. 进入既有话题必须使用 `msg reply <om_xxx>`；`omt_xxx` 仅用于话题查询/转发，不能作为
   `msg send` 的接收者。普通消息群开启新话题时才加 `--reply-in-thread`。
5. 发送与回复重试都使用同一 `--idempotency-key`；媒体上传可能重做，但服务端幂等键防止
   可见消息重复。只有命令返回非空 `message_id` 才判定成功。
6. 外部群 232033 的排错读取 `references/workflows/chat/references/external-chat.md`。
7. 卡片草稿与发送候选分开校验；发送前必须运行 card workflow 的
   `lint_card.py --strict`，不得发送含占位符、示例 ID、假链接或未接通回调按钮的卡片。
8. 用户点名卡片风格时使用 card workflow 内置的 19 个预设；使用本地头图或图标素材时，
   lint 与实际的 `msg send` / `msg reply` 都传 `--upload-images`，不要固化跨租户 `img_key`。
