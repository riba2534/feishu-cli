---
name: feishu-cli-mail
description: >-
  飞书邮箱专用入口，覆盖收件箱分诊、邮件和线程读取、发送、回复、转发、草稿、查询签名、
  CID 内联图片和模板。用户提到飞书邮件、邮箱、收件箱、未读邮件、草稿、邮箱签名、
  邮件模板、回复、转发、发送预览/确认或发送 HTML/CID 邮件时必须使用本 Skill。
  仅要求预览或等待发送确认也属于本 Skill 的草稿/发送工作流，必须先加载本 Skill 准备预览。
  只读命令可用 User/Bot；写入、签名和模板需要 User。聊天消息使用 feishu-cli-messaging。
compatibility: Requires feishu-cli v1.41.0+ and network access for Feishu API calls.
allowed-tools: Bash(feishu-cli:*) Bash(./feishu-cli:*) Bash(./bin/feishu-cli:*) Read Write
---

# 飞书邮箱

读取 `references/workflows/mail/workflow.md` 后执行。
将该工作流中的 `references/` 相对路径按 `workflow.md` 所在目录解析。

## 执行规则

1. `triage/message/messages/thread` 支持 `--as bot|user|auto`；Bot 必须显式指定邮箱，不能用 `me`。写类、签名和模板管理需 User Token。`auth check` 只预检本地 User Token，不作为 Bot 的前置条件。
2. 用户只要求草稿或尚未明确发送时保存草稿；已明确授权发送且收件人、主题、正文齐备时直接使用 `--confirm-send`，不重复索取确认。普通附件暂不支持，不要承诺发送未支持的附件。
3. 回复和转发前先读取原邮件，避免选错 message ID 或 thread ID。
4. 不在日志或结果中回显邮件正文里的敏感信息。

遇到 Token、身份或 scope 报错（如 99991663/99991668/99991672/99991679）时，读取 `../feishu-cli-platform/references/workflows/auth/references/identity.md` 确认应使用的身份与预检方式，排错表见 `../feishu-cli-platform/references/workflows/auth/workflow.md`。
