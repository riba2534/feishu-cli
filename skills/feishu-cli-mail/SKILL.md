---
name: feishu-cli-mail
description: >-
  飞书邮箱专用入口，覆盖收件箱分诊、邮件和线程读取、发送、回复、转发、草稿、过滤器、签名、
  CID 内联图片和模板。用户提到飞书邮件、邮箱、收件箱、未读邮件、草稿、过滤器、邮箱签名、
  邮件模板、回复、转发、发送预览/确认或发送 HTML/CID 邮件时必须使用本 Skill。
  即使用户只说“发送飞书邮件前先生成预览并等待我明确确认”、暂时不真正发送，也属于本
  Skill 的草稿/发送工作流，必须使用本 Skill。
  邮件命令必须使用 User Token；发送默认先保存草稿，只有明确确认后才真正发送。
argument-hint: <triage|message|thread|send|reply|forward|draft> [args]
user-invocable: true
allowed-tools: Bash(feishu-cli:*), Read, Write
---

# 飞书邮箱

读取 `references/workflows/mail/workflow.md` 后执行。
将该工作流中的 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按 `workflow.md`
所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 安全规则

1. 所有 mail 命令均需 User Token；先执行对应 scope 的 `auth check`。
2. 默认创建草稿。只有用户明确要求发送并确认收件人、主题和附件后，才使用 `--confirm-send`。
3. 回复和转发前先读取原邮件，避免选错 message ID 或 thread ID。
4. 不在日志或结果中回显邮件正文里的敏感信息。
