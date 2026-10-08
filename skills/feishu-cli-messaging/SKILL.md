---
name: feishu-cli-messaging
description: >-
  飞书即时消息、群聊与交互卡片：发送、回复、转发、加急、撤回、资源下载、聊天历史与话题、Reaction/Pin、成员管理和事件订阅。用于构造或预览 Card JSON 2.0、模板及卡片引用，也支持品牌宣传和主题风格卡片。仅处理 IM 消息及其资源；跨会话关键词搜索使用 feishu-cli-platform。明确不用于邮箱邮件、视频会议录制、妙记或逐字稿及其媒体下载，分别使用 feishu-cli-mail、feishu-cli-meetings。
compatibility: Requires feishu-cli v1.41.0+ and network access for Feishu API calls. Bundled scripts require Python 3.10+.
allowed-tools: Bash(feishu-cli:*) Bash(./feishu-cli:*) Bash(./bin/feishu-cli:*) Bash(jq:*) Bash(python3:*) Read Write
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
7. 完整 Card JSON 2.0 的发送候选运行 card workflow 的 `lint_card.py --strict`；
   `template_id/card_id` 引用按 msg workflow 校验引用结构与实际 ID，不送入完整 JSON 的 linter。
   接收者和卡片引用必须来自本次请求、已授权上下文或配置，不能把示例值当默认值。
8. 用户点名卡片风格时使用 card workflow 内置的 19 个预设；使用本地头图或图标素材时，
   lint 与实际的 `msg send` / `msg reply` 都传 `--upload-images`，不要固化跨租户 `img_key`。

遇到 Token、身份或 scope 报错（如 99991663/99991668/99991672/99991679）时，读取 `../feishu-cli-platform/references/workflows/auth/references/identity.md` 确认应使用的身份与预检方式，排错表见 `../feishu-cli-platform/references/workflows/auth/workflow.md`。
